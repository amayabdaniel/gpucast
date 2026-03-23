package metrics

import (
	"fmt"
	"math"
)

// WasteReason categorizes why GPU resources are being wasted.
type WasteReason string

const (
	WasteIdle          WasteReason = "idle"           // model loaded, no requests
	WasteOverprovisioned WasteReason = "overprovisioned" // GPU too big for model
	WasteLowBatch      WasteReason = "low_batch"      // batch size 1 when capacity exists
	WasteColdStart     WasteReason = "cold_start"     // GPU time spent loading model
	WasteFragmentation WasteReason = "fragmentation"  // KV cache fragmented, wasted VRAM
)

// WasteAnalysis is the result of analyzing GPU waste.
type WasteAnalysis struct {
	TotalWastePercent float64
	Findings          []WasteFinding
}

// WasteFinding is a single identified waste cause with actionable advice.
type WasteFinding struct {
	Reason      WasteReason
	Severity    string  // "critical", "warning", "info"
	WastePercent float64
	Description string
	Action      string
	SavedUSD    float64 // estimated monthly savings if fixed
}

// WasteInput contains the data needed for waste analysis.
type WasteInput struct {
	GPUUtilPercent    float64
	GPUVRAMUsedGB     float64
	GPUVRAMTotalGB    float64
	RequestsPerMin    float64
	AvgBatchSize      float64
	MaxBatchSize      float64
	CacheHitRate      float64
	ModelLoadTimeSec  float64
	NumPreemptions    float64
	GPUHourlyRate     float64
}

// AnalyzeWaste examines GPU metrics and identifies specific waste causes.
func AnalyzeWaste(input WasteInput) WasteAnalysis {
	analysis := WasteAnalysis{}

	// 1. Idle detection: GPU loaded but few/no requests
	if input.RequestsPerMin < 1 && input.GPUVRAMUsedGB > 0 {
		wastePercent := 100 - input.GPUUtilPercent
		monthlySaved := input.GPUHourlyRate * 24 * 30 * (wastePercent / 100)
		analysis.Findings = append(analysis.Findings, WasteFinding{
			Reason:       WasteIdle,
			Severity:     "critical",
			WastePercent: wastePercent,
			Description:  fmt.Sprintf("Model is loaded (%.1fGB VRAM) but receiving <1 req/min", input.GPUVRAMUsedGB),
			Action:       "Consider scale-to-zero or model unloading after idle timeout",
			SavedUSD:     math.Round(monthlySaved*100) / 100,
		})
	} else if input.GPUUtilPercent < 20 && input.RequestsPerMin > 0 {
		analysis.Findings = append(analysis.Findings, WasteFinding{
			Reason:       WasteIdle,
			Severity:     "warning",
			WastePercent: 80 - input.GPUUtilPercent,
			Description:  fmt.Sprintf("GPU utilization %.0f%% with %.0f req/min", input.GPUUtilPercent, input.RequestsPerMin),
			Action:       "Consolidate with other models using GPU time-slicing or MPS",
			SavedUSD:     math.Round(input.GPUHourlyRate*24*30*0.5*100) / 100, // ~50% savings estimate
		})
	}

	// 2. Overprovisioned: VRAM usage far below GPU capacity
	if input.GPUVRAMTotalGB > 0 {
		vramUtil := (input.GPUVRAMUsedGB / input.GPUVRAMTotalGB) * 100
		if vramUtil < 30 && input.GPUVRAMTotalGB >= 40 {
			// Find cheapest GPU that fits
			cheaperGPU, cheaperRate := findCheaperGPU(input.GPUVRAMUsedGB)
			if cheaperRate > 0 && cheaperRate < input.GPUHourlyRate {
				monthlySaved := (input.GPUHourlyRate - cheaperRate) * 24 * 30
				analysis.Findings = append(analysis.Findings, WasteFinding{
					Reason:       WasteOverprovisioned,
					Severity:     "warning",
					WastePercent: 100 - vramUtil,
					Description:  fmt.Sprintf("Using %.0fGB GPU but only %.1fGB VRAM needed (%.0f%% utilized)", input.GPUVRAMTotalGB, input.GPUVRAMUsedGB, vramUtil),
					Action:       fmt.Sprintf("Downgrade to %s and save $%.0f/month", cheaperGPU, monthlySaved),
					SavedUSD:     math.Round(monthlySaved*100) / 100,
				})
			}
		}
	}

	// 3. Low batch size: not utilizing batch capacity
	if input.AvgBatchSize > 0 && input.MaxBatchSize > 0 {
		batchUtil := (input.AvgBatchSize / input.MaxBatchSize) * 100
		if batchUtil < 25 && input.RequestsPerMin > 5 {
			analysis.Findings = append(analysis.Findings, WasteFinding{
				Reason:       WasteLowBatch,
				Severity:     "info",
				WastePercent: 100 - batchUtil,
				Description:  fmt.Sprintf("Avg batch size %.1f out of max %.0f (%.0f%% batch utilization)", input.AvgBatchSize, input.MaxBatchSize, batchUtil),
				Action:       "Increase max_num_seqs or waiting time to improve batching",
			})
		}
	}

	// 4. Cold start waste
	if input.ModelLoadTimeSec > 30 {
		analysis.Findings = append(analysis.Findings, WasteFinding{
			Reason:       WasteColdStart,
			Severity:     "info",
			WastePercent: 0,
			Description:  fmt.Sprintf("Model load time: %.0fs — each scale-up event wastes this GPU time", input.ModelLoadTimeSec),
			Action:       "Use warm pod pool or pre-loaded model cache to reduce cold starts",
		})
	}

	// 5. KV cache fragmentation
	if input.NumPreemptions > 10 {
		analysis.Findings = append(analysis.Findings, WasteFinding{
			Reason:       WasteFragmentation,
			Severity:     "warning",
			WastePercent: 0,
			Description:  fmt.Sprintf("%.0f preemptions detected — KV cache is being evicted under memory pressure", input.NumPreemptions),
			Action:       "Increase GPU VRAM, reduce max_num_seqs, or lower max context length",
		})
	}

	// Calculate total waste
	totalWaste := 0.0
	for _, f := range analysis.Findings {
		if f.WastePercent > totalWaste {
			totalWaste = f.WastePercent
		}
	}
	analysis.TotalWastePercent = totalWaste

	return analysis
}

func findCheaperGPU(vramNeededGB float64) (string, float64) {
	gpus := []struct {
		name string
		vram float64
		cost float64
	}{
		{"T4", 16, 0.35},
		{"L4", 24, 0.80},
		{"A10G", 24, 1.01},
		{"A100-40GB", 40, 3.40},
	}

	for _, gpu := range gpus {
		if gpu.vram >= vramNeededGB*1.2 { // 20% headroom
			return gpu.name, gpu.cost
		}
	}
	return "", 0
}

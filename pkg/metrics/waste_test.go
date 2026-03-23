package metrics

import "testing"

func TestAnalyzeWaste_IdleGPU(t *testing.T) {
	input := WasteInput{
		GPUUtilPercent: 2,
		GPUVRAMUsedGB:  12,
		GPUVRAMTotalGB: 24,
		RequestsPerMin: 0.1,
		GPUHourlyRate:  0.80,
	}

	result := AnalyzeWaste(input)
	if len(result.Findings) == 0 {
		t.Fatal("expected waste findings for idle GPU")
	}

	found := false
	for _, f := range result.Findings {
		if f.Reason == WasteIdle && f.Severity == "critical" {
			found = true
			if f.SavedUSD <= 0 {
				t.Error("expected savings estimate for idle GPU")
			}
		}
	}
	if !found {
		t.Error("expected critical idle finding")
	}
}

func TestAnalyzeWaste_Overprovisioned(t *testing.T) {
	input := WasteInput{
		GPUUtilPercent: 30,
		GPUVRAMUsedGB:  5,
		GPUVRAMTotalGB: 80,
		RequestsPerMin: 10,
		GPUHourlyRate:  4.10,
	}

	result := AnalyzeWaste(input)
	found := false
	for _, f := range result.Findings {
		if f.Reason == WasteOverprovisioned {
			found = true
			if f.SavedUSD <= 0 {
				t.Error("expected savings for overprovisioned GPU")
			}
			t.Logf("Action: %s, Savings: $%.0f/mo", f.Action, f.SavedUSD)
		}
	}
	if !found {
		t.Error("expected overprovisioned finding for 5GB on 80GB GPU")
	}
}

func TestAnalyzeWaste_LowBatchSize(t *testing.T) {
	input := WasteInput{
		GPUUtilPercent: 50,
		GPUVRAMUsedGB:  20,
		GPUVRAMTotalGB: 24,
		RequestsPerMin: 30,
		AvgBatchSize:   1.2,
		MaxBatchSize:   16,
		GPUHourlyRate:  0.80,
	}

	result := AnalyzeWaste(input)
	found := false
	for _, f := range result.Findings {
		if f.Reason == WasteLowBatch {
			found = true
		}
	}
	if !found {
		t.Error("expected low batch finding")
	}
}

func TestAnalyzeWaste_ColdStart(t *testing.T) {
	input := WasteInput{
		GPUUtilPercent:   80,
		GPUVRAMUsedGB:    20,
		GPUVRAMTotalGB:   24,
		RequestsPerMin:   50,
		ModelLoadTimeSec: 60,
		GPUHourlyRate:    0.80,
	}

	result := AnalyzeWaste(input)
	found := false
	for _, f := range result.Findings {
		if f.Reason == WasteColdStart {
			found = true
		}
	}
	if !found {
		t.Error("expected cold start finding for 60s load time")
	}
}

func TestAnalyzeWaste_Fragmentation(t *testing.T) {
	input := WasteInput{
		GPUUtilPercent:  70,
		GPUVRAMUsedGB:   20,
		GPUVRAMTotalGB:  24,
		RequestsPerMin:  40,
		NumPreemptions:  50,
		GPUHourlyRate:   0.80,
	}

	result := AnalyzeWaste(input)
	found := false
	for _, f := range result.Findings {
		if f.Reason == WasteFragmentation {
			found = true
		}
	}
	if !found {
		t.Error("expected fragmentation finding for 50 preemptions")
	}
}

func TestAnalyzeWaste_HealthyGPU(t *testing.T) {
	input := WasteInput{
		GPUUtilPercent:  75,
		GPUVRAMUsedGB:   18,
		GPUVRAMTotalGB:  24,
		RequestsPerMin:  100,
		AvgBatchSize:    8,
		MaxBatchSize:    16,
		ModelLoadTimeSec: 15,
		NumPreemptions:  2,
		GPUHourlyRate:   0.80,
	}

	result := AnalyzeWaste(input)
	if len(result.Findings) > 0 {
		t.Errorf("expected no waste for healthy GPU, got %d findings", len(result.Findings))
		for _, f := range result.Findings {
			t.Logf("  finding: %s: %s", f.Reason, f.Description)
		}
	}
}

func TestAnalyzeWaste_TotalWastePercent(t *testing.T) {
	input := WasteInput{
		GPUUtilPercent: 5,
		GPUVRAMUsedGB:  5,
		GPUVRAMTotalGB: 80,
		RequestsPerMin: 0.5,
		GPUHourlyRate:  8.00,
	}

	result := AnalyzeWaste(input)
	if result.TotalWastePercent < 50 {
		t.Errorf("expected high waste for 5%% util on H100, got %.0f%%", result.TotalWastePercent)
	}
}

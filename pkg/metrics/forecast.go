package metrics

import (
	"math"
	"sync"
	"time"
)

// CostForecaster tracks spending velocity and predicts end-of-period costs.
type CostForecaster struct {
	mu       sync.RWMutex
	samples  []costSample
	maxAge   time.Duration
	window   int
}

type costSample struct {
	timestamp time.Time
	costUSD   float64
}

// ForecastResult contains the prediction and supporting data.
type ForecastResult struct {
	CurrentSpendUSD     float64
	SpendRatePerHour    float64
	ProjectedDailyUSD   float64
	ProjectedMonthlyUSD float64
	DaysUntilBudget     float64
	Confidence          string // "high", "medium", "low"
}

// NewCostForecaster creates a forecaster with the given sample window.
func NewCostForecaster(maxAge time.Duration, maxSamples int) *CostForecaster {
	return &CostForecaster{
		maxAge: maxAge,
		window: maxSamples,
	}
}

// Record adds a cost observation.
func (f *CostForecaster) Record(costUSD float64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.samples = append(f.samples, costSample{
		timestamp: time.Now(),
		costUSD:   costUSD,
	})

	// Trim old samples
	f.trimLocked()
}

// Forecast returns the projected costs based on recent spend velocity.
func (f *CostForecaster) Forecast(budgetUSD float64) ForecastResult {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if len(f.samples) < 2 {
		return ForecastResult{Confidence: "low"}
	}

	// Calculate spend rate from samples
	first := f.samples[0]
	last := f.samples[len(f.samples)-1]

	elapsed := last.timestamp.Sub(first.timestamp)
	if elapsed <= 0 {
		return ForecastResult{
			CurrentSpendUSD: last.costUSD,
			Confidence:      "low",
		}
	}

	costDelta := last.costUSD - first.costUSD
	if costDelta < 0 {
		costDelta = last.costUSD // counter reset
	}

	hoursElapsed := elapsed.Hours()
	spendPerHour := costDelta / hoursElapsed

	result := ForecastResult{
		CurrentSpendUSD:     last.costUSD,
		SpendRatePerHour:    spendPerHour,
		ProjectedDailyUSD:   spendPerHour * 24,
		ProjectedMonthlyUSD: spendPerHour * 24 * 30,
	}

	if budgetUSD > 0 && spendPerHour > 0 {
		remaining := budgetUSD - last.costUSD
		if remaining > 0 {
			hoursLeft := remaining / spendPerHour
			result.DaysUntilBudget = hoursLeft / 24
		}
	}

	// Confidence based on sample count and time span
	switch {
	case len(f.samples) >= 20 && hoursElapsed >= 1:
		result.Confidence = "high"
	case len(f.samples) >= 5 && hoursElapsed >= 0.25:
		result.Confidence = "medium"
	default:
		result.Confidence = "low"
	}

	// Round values
	result.SpendRatePerHour = math.Round(result.SpendRatePerHour*1000) / 1000
	result.ProjectedDailyUSD = math.Round(result.ProjectedDailyUSD*100) / 100
	result.ProjectedMonthlyUSD = math.Round(result.ProjectedMonthlyUSD*100) / 100
	result.DaysUntilBudget = math.Round(result.DaysUntilBudget*10) / 10

	return result
}

// SampleCount returns the current number of samples.
func (f *CostForecaster) SampleCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.samples)
}

func (f *CostForecaster) trimLocked() {
	cutoff := time.Now().Add(-f.maxAge)

	// Remove old samples
	i := 0
	for i < len(f.samples) && f.samples[i].timestamp.Before(cutoff) {
		i++
	}
	if i > 0 {
		f.samples = f.samples[i:]
	}

	// Cap to max window
	if len(f.samples) > f.window {
		f.samples = f.samples[len(f.samples)-f.window:]
	}
}

package metrics

import (
	"testing"
	"time"
)

func TestCostForecaster_BasicForecast(t *testing.T) {
	f := NewCostForecaster(1*time.Hour, 100)

	// Simulate $1/hr spend rate over 10 samples, 6 min apart
	for i := 0; i < 10; i++ {
		f.mu.Lock()
		f.samples = append(f.samples, costSample{
			timestamp: time.Now().Add(time.Duration(i) * 6 * time.Minute),
			costUSD:   float64(i) * 0.1, // $0.10 per 6 min = $1/hr
		})
		f.mu.Unlock()
	}

	result := f.Forecast(100)

	if result.SpendRatePerHour < 0.9 || result.SpendRatePerHour > 1.1 {
		t.Errorf("expected ~$1/hr spend rate, got $%.3f/hr", result.SpendRatePerHour)
	}

	if result.ProjectedDailyUSD < 20 || result.ProjectedDailyUSD > 28 {
		t.Errorf("expected ~$24/day projected, got $%.2f", result.ProjectedDailyUSD)
	}

	if result.ProjectedMonthlyUSD < 600 || result.ProjectedMonthlyUSD > 840 {
		t.Errorf("expected ~$720/mo projected, got $%.2f", result.ProjectedMonthlyUSD)
	}
}

func TestCostForecaster_BudgetCountdown(t *testing.T) {
	f := NewCostForecaster(1*time.Hour, 100)

	// $10 spent so far, $1/hr rate
	f.mu.Lock()
	f.samples = append(f.samples,
		costSample{timestamp: time.Now().Add(-1 * time.Hour), costUSD: 9.0},
		costSample{timestamp: time.Now(), costUSD: 10.0},
	)
	f.mu.Unlock()

	result := f.Forecast(100) // $100 budget

	// $90 remaining at $1/hr = 90 hours = 3.75 days
	if result.DaysUntilBudget < 3.0 || result.DaysUntilBudget > 4.5 {
		t.Errorf("expected ~3.75 days until budget, got %.1f", result.DaysUntilBudget)
	}
}

func TestCostForecaster_InsufficientSamples(t *testing.T) {
	f := NewCostForecaster(1*time.Hour, 100)

	f.Record(1.0)
	result := f.Forecast(100)

	if result.Confidence != "low" {
		t.Errorf("expected low confidence with 1 sample, got %s", result.Confidence)
	}
}

func TestCostForecaster_HighConfidence(t *testing.T) {
	f := NewCostForecaster(2*time.Hour, 100)

	f.mu.Lock()
	for i := 0; i < 25; i++ {
		f.samples = append(f.samples, costSample{
			timestamp: time.Now().Add(time.Duration(i) * 5 * time.Minute),
			costUSD:   float64(i) * 0.5,
		})
	}
	f.mu.Unlock()

	result := f.Forecast(1000)
	if result.Confidence != "high" {
		t.Errorf("expected high confidence with 25 samples over 2hrs, got %s", result.Confidence)
	}
}

func TestCostForecaster_ZeroBudget(t *testing.T) {
	f := NewCostForecaster(1*time.Hour, 100)

	f.mu.Lock()
	f.samples = append(f.samples,
		costSample{timestamp: time.Now().Add(-30 * time.Minute), costUSD: 0},
		costSample{timestamp: time.Now(), costUSD: 5},
	)
	f.mu.Unlock()

	result := f.Forecast(0) // no budget set
	if result.DaysUntilBudget != 0 {
		t.Errorf("expected 0 days until budget when no budget set, got %.1f", result.DaysUntilBudget)
	}
}

func TestCostForecaster_SampleCount(t *testing.T) {
	f := NewCostForecaster(1*time.Hour, 100)

	if f.SampleCount() != 0 {
		t.Errorf("expected 0 samples initially, got %d", f.SampleCount())
	}

	f.Record(1.0)
	f.Record(2.0)
	if f.SampleCount() != 2 {
		t.Errorf("expected 2 samples, got %d", f.SampleCount())
	}
}

func TestCostForecaster_WindowCap(t *testing.T) {
	f := NewCostForecaster(24*time.Hour, 5) // max 5 samples

	for i := 0; i < 10; i++ {
		f.Record(float64(i))
	}

	if f.SampleCount() > 5 {
		t.Errorf("expected max 5 samples, got %d", f.SampleCount())
	}
}

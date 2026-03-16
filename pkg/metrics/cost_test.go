package metrics

import (
	"math"
	"testing"
)

func TestCostCalculator_BasicCalculation(t *testing.T) {
	calc := &CostCalculator{GPUHourlyRate: 3.60} // $3.60/hr = $0.001/sec

	result := calc.Calculate(1.0, 100, 50)

	if result.GPUSeconds != 1.0 {
		t.Errorf("expected 1.0 GPU seconds, got %f", result.GPUSeconds)
	}
	expectedCost := (1.0 / 3600.0) * 3.60 // = $0.001
	if math.Abs(result.CostUSD-expectedCost) > 0.0001 {
		t.Errorf("expected cost ~$%.6f, got $%.6f", expectedCost, result.CostUSD)
	}
	if result.TotalTokens != 150 {
		t.Errorf("expected 150 total tokens, got %d", result.TotalTokens)
	}
	if result.TokensPerDollar < 100000 {
		t.Errorf("expected tokens/dollar > 100k, got %.0f", result.TokensPerDollar)
	}
}

func TestCostCalculator_ZeroGPUSeconds(t *testing.T) {
	calc := &CostCalculator{GPUHourlyRate: 3.60}

	result := calc.Calculate(0, 0, 0)

	if result.CostUSD != 0 {
		t.Errorf("expected $0, got $%f", result.CostUSD)
	}
	if result.TokensPerDollar != 0 {
		t.Errorf("expected 0 tokens/dollar for zero cost, got %f", result.TokensPerDollar)
	}
}

func TestCostCalculator_ExpensiveGPU(t *testing.T) {
	calc := &CostCalculator{GPUHourlyRate: 40.00} // H100 $40/hr

	result := calc.Calculate(3.0, 1000, 500)

	expectedCost := (3.0 / 3600.0) * 40.0 // ~$0.0333
	if math.Abs(result.CostUSD-expectedCost) > 0.001 {
		t.Errorf("expected cost ~$%.4f, got $%.4f", expectedCost, result.CostUSD)
	}
}

func TestCostCalculator_LongRequest(t *testing.T) {
	calc := &CostCalculator{GPUHourlyRate: 1.00} // cheap GPU

	result := calc.Calculate(60.0, 10000, 5000) // 60 seconds

	expectedCost := (60.0 / 3600.0) * 1.0 // ~$0.0167
	if math.Abs(result.CostUSD-expectedCost) > 0.001 {
		t.Errorf("expected cost ~$%.4f, got $%.4f", expectedCost, result.CostUSD)
	}
	if result.TotalTokens != 15000 {
		t.Errorf("expected 15000 tokens, got %d", result.TotalTokens)
	}
}

func TestCostCalculator_TokenEfficiency(t *testing.T) {
	calc := &CostCalculator{GPUHourlyRate: 3.60}

	fast := calc.Calculate(0.5, 500, 200)  // fast request
	slow := calc.Calculate(5.0, 500, 200)  // same tokens, 10x GPU time

	if fast.TokensPerDollar <= slow.TokensPerDollar {
		t.Errorf("faster request should have higher tokens/dollar: fast=%.0f, slow=%.0f",
			fast.TokensPerDollar, slow.TokensPerDollar)
	}
}

func TestCostCalculator_MetricsRegister(t *testing.T) {
	// Verify all metrics can be registered without panic
	reg := NewTestRegistry()
	RegisterAll(reg)
}

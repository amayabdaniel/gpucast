package metrics

// CostCalculator computes the estimated cost of an inference request
// by correlating GPU time with the hourly rate.
type CostCalculator struct {
	// GPUHourlyRate is the cost per GPU-hour in USD.
	GPUHourlyRate float64
}

// RequestCost represents the computed cost breakdown of a single inference request.
type RequestCost struct {
	GPUSeconds     float64
	CostUSD        float64
	PromptTokens   int
	CompletionTokens int
	TotalTokens    int
	TokensPerDollar float64
}

// Calculate computes the cost of an inference request given GPU usage and token counts.
func (c *CostCalculator) Calculate(gpuSeconds float64, promptTokens, completionTokens int) RequestCost {
	costUSD := (gpuSeconds / 3600.0) * c.GPUHourlyRate
	totalTokens := promptTokens + completionTokens

	var tokensPerDollar float64
	if costUSD > 0 {
		tokensPerDollar = float64(totalTokens) / costUSD
	}

	return RequestCost{
		GPUSeconds:       gpuSeconds,
		CostUSD:          costUSD,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		TokensPerDollar:  tokensPerDollar,
	}
}

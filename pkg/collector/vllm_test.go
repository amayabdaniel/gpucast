package collector

import (
	"math"
	"testing"
)

const sampleVLLMMetrics = `# HELP vllm:num_requests_running Number of requests currently running
# TYPE vllm:num_requests_running gauge
vllm:num_requests_running 3
# HELP vllm:num_requests_waiting Number of requests waiting
# TYPE vllm:num_requests_waiting gauge
vllm:num_requests_waiting 1
# HELP vllm:num_requests_total Total number of requests
# TYPE vllm:num_requests_total counter
vllm:num_requests_total 1500
# HELP vllm:prompt_tokens_total Total prompt tokens
# TYPE vllm:prompt_tokens_total counter
vllm:prompt_tokens_total 450000
# HELP vllm:generation_tokens_total Total generation tokens
# TYPE vllm:generation_tokens_total counter
vllm:generation_tokens_total 225000
# HELP vllm:gpu_cache_usage_perc GPU KV cache usage
# TYPE vllm:gpu_cache_usage_perc gauge
vllm:gpu_cache_usage_perc 0.72
# HELP vllm:num_preemptions_total Total preemptions
# TYPE vllm:num_preemptions_total counter
vllm:num_preemptions_total 5
# HELP vllm:time_to_first_token_seconds Time to first token
# TYPE vllm:time_to_first_token_seconds summary
vllm:time_to_first_token_seconds{quantile="0.5"} 0.15
vllm:time_to_first_token_seconds{quantile="0.95"} 0.42
vllm:time_to_first_token_seconds{quantile="0.99"} 0.88
# HELP vllm:time_per_output_token_seconds Time per output token
# TYPE vllm:time_per_output_token_seconds summary
vllm:time_per_output_token_seconds{quantile="0.5"} 0.012
vllm:time_per_output_token_seconds{quantile="0.95"} 0.025
# HELP vllm:e2e_request_latency_seconds End to end latency
# TYPE vllm:e2e_request_latency_seconds summary
vllm:e2e_request_latency_seconds{quantile="0.5"} 1.95
vllm:e2e_request_latency_seconds{quantile="0.95"} 4.20
`

func TestVLLMCollector_ParseRequestMetrics(t *testing.T) {
	c := NewVLLMCollector("http://fake:8000/metrics", 3.60, "test-model")
	m, err := c.Parse(sampleVLLMMetrics)
	if err != nil {
		t.Fatal(err)
	}

	if m.RequestsTotal != 1500 {
		t.Errorf("expected 1500 requests, got %f", m.RequestsTotal)
	}
	if m.RequestsRunning != 3 {
		t.Errorf("expected 3 running, got %f", m.RequestsRunning)
	}
	if m.RequestsWaiting != 1 {
		t.Errorf("expected 1 waiting, got %f", m.RequestsWaiting)
	}
}

func TestVLLMCollector_ParseTokenMetrics(t *testing.T) {
	c := NewVLLMCollector("http://fake:8000/metrics", 3.60, "test-model")
	m, err := c.Parse(sampleVLLMMetrics)
	if err != nil {
		t.Fatal(err)
	}

	if m.PromptTokensTotal != 450000 {
		t.Errorf("expected 450000 prompt tokens, got %f", m.PromptTokensTotal)
	}
	if m.GenerationTokensTotal != 225000 {
		t.Errorf("expected 225000 gen tokens, got %f", m.GenerationTokensTotal)
	}
}

func TestVLLMCollector_ParseLatencyMetrics(t *testing.T) {
	c := NewVLLMCollector("http://fake:8000/metrics", 3.60, "test-model")
	m, err := c.Parse(sampleVLLMMetrics)
	if err != nil {
		t.Fatal(err)
	}

	if m.TTFT_P50 != 0.15 {
		t.Errorf("expected TTFT p50 = 0.15, got %f", m.TTFT_P50)
	}
	if m.TTFT_P95 != 0.42 {
		t.Errorf("expected TTFT p95 = 0.42, got %f", m.TTFT_P95)
	}
	if m.TTFT_P99 != 0.88 {
		t.Errorf("expected TTFT p99 = 0.88, got %f", m.TTFT_P99)
	}
	if m.TPOT_P50 != 0.012 {
		t.Errorf("expected TPOT p50 = 0.012, got %f", m.TPOT_P50)
	}
	if m.E2E_P50 != 1.95 {
		t.Errorf("expected E2E p50 = 1.95, got %f", m.E2E_P50)
	}
}

func TestVLLMCollector_ParseGPUMetrics(t *testing.T) {
	c := NewVLLMCollector("http://fake:8000/metrics", 3.60, "test-model")
	m, err := c.Parse(sampleVLLMMetrics)
	if err != nil {
		t.Fatal(err)
	}

	if m.GPUCacheUsagePercent != 72.0 {
		t.Errorf("expected 72%% cache usage, got %f", m.GPUCacheUsagePercent)
	}
	if m.NumPreemptions != 5 {
		t.Errorf("expected 5 preemptions, got %f", m.NumPreemptions)
	}
}

func TestVLLMCollector_CostEstimation(t *testing.T) {
	c := NewVLLMCollector("http://fake:8000/metrics", 3.60, "test-model")
	m, err := c.Parse(sampleVLLMMetrics)
	if err != nil {
		t.Fatal(err)
	}

	// 1500 requests, 225000 gen tokens → 150 tokens/req avg
	// GPU seconds per req ≈ 0.15 (TTFT) + 150 * 0.012 (TPOT) = 1.95s
	// Total GPU seconds ≈ 1500 * 1.95 = 2925s
	// Cost = (2925 / 3600) * 3.60 ≈ $2.925

	if m.EstimatedGPUSeconds < 2900 || m.EstimatedGPUSeconds > 2950 {
		t.Errorf("expected ~2925 GPU seconds, got %f", m.EstimatedGPUSeconds)
	}

	if math.Abs(m.EstimatedCostUSD-2.925) > 0.1 {
		t.Errorf("expected ~$2.93 cost, got $%.2f", m.EstimatedCostUSD)
	}
}

func TestVLLMCollector_EmptyMetrics(t *testing.T) {
	c := NewVLLMCollector("http://fake:8000/metrics", 3.60, "test-model")
	m, err := c.Parse("")
	if err != nil {
		t.Fatal(err)
	}

	if m.RequestsTotal != 0 {
		t.Errorf("expected 0 requests from empty input, got %f", m.RequestsTotal)
	}
	if m.EstimatedCostUSD != 0 {
		t.Errorf("expected $0 cost from empty input, got $%f", m.EstimatedCostUSD)
	}
}

func TestVLLMCollector_CommentsAndBlankLines(t *testing.T) {
	input := `# This is a comment
# Another comment

vllm:num_requests_total 42

# More comments
`
	c := NewVLLMCollector("http://fake:8000/metrics", 1.00, "test")
	m, err := c.Parse(input)
	if err != nil {
		t.Fatal(err)
	}

	if m.RequestsTotal != 42 {
		t.Errorf("expected 42 requests, got %f", m.RequestsTotal)
	}
}

// TestParseValue_IgnoresOptionalTimestamp locks in the take-last-field
// fix. Prometheus exposition allows a trailing Unix-ms timestamp; the
// previous impl returned it as the value whenever vLLM emitted one,
// making cost metrics jump to ~1.7e12.
func TestParseValue_IgnoresOptionalTimestamp(t *testing.T) {
	cases := map[string]float64{
		"vllm:num_requests_running 3":                          3,
		"vllm:num_requests_running 3 1700000000000":            3,
		`vllm:gpu_cache_usage_perc{model="m"} 0.62`:            0.62,
		`vllm:gpu_cache_usage_perc{model="m"} 0.62 1700000000`: 0.62,
		"garbage":                                               0,
		"# HELP not a value":                                    0,
	}
	for line, want := range cases {
		if got := parseValue(line); got != want {
			t.Errorf("parseValue(%q) = %v, want %v", line, got, want)
		}
	}
}

// TestParseValue_RejectsNonFinite locks in the NaN/±Inf fix. vLLM can
// emit non-finite values on uninitialized gauges and divide-by-zero
// counters; ParseFloat accepts all three silently. Any one of them
// would poison main.go's delta-tracking `+=` accumulator (NaN is
// contagious under addition) and make encoding/json refuse to marshal
// the metrics response.
func TestParseValue_RejectsNonFinite(t *testing.T) {
	// Note: a non-finite value followed by a finite timestamp
	// ("metric NaN 1700000000") is inherently ambiguous under the
	// left-to-right scanner. Exporters emitting NaN generally don't
	// include timestamps; we accept the tradeoff.
	for _, line := range []string{
		"vllm:num_requests_running NaN",
		"vllm:num_requests_running +Inf",
		"vllm:num_requests_running -Inf",
		`vllm:gpu_cache_usage_perc{model="m"} NaN`,
		"vllm:num_preemptions_total Inf",
	} {
		got := parseValue(line)
		if got != 0 {
			t.Errorf("parseValue(%q) = %v; want 0 (non-finite must not propagate)", line, got)
		}
		if math.IsNaN(got) || math.IsInf(got, 0) {
			t.Errorf("parseValue(%q) returned non-finite %v — the whole point of the guard", line, got)
		}
	}
}

// TestParseValue_AccumulatorSurvivesNaNLine mirrors the invariant
// downstream code depends on: summing values across a mix of good and
// non-finite lines produces a finite total. Before the fix a single
// NaN in a scrape poisoned the running total forever.
func TestParseValue_AccumulatorSurvivesNaNLine(t *testing.T) {
	var total float64
	for _, line := range []string{
		"vllm:num_requests_running 10",
		"vllm:num_requests_running NaN",
		"vllm:num_requests_running 20",
	} {
		total += parseValue(line)
	}
	if total != 30 {
		t.Errorf("accumulator should sum to 30 across finite lines; got %v", total)
	}
	if math.IsNaN(total) || math.IsInf(total, 0) {
		t.Fatalf("total became non-finite (%v) — NaN escaped the parser", total)
	}
}

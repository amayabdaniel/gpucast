package collector

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// VLLMCollector scrapes a vLLM Prometheus endpoint and extracts
// inference-specific metrics for cost correlation.
type VLLMCollector struct {
	endpoint   string
	client     *http.Client
	gpuHourly  float64
	modelName  string
}

// VLLMMetrics holds parsed metrics from a single vLLM scrape.
type VLLMMetrics struct {
	// Request metrics
	RequestsTotal     float64
	RequestsRunning   float64
	RequestsWaiting   float64

	// Token metrics
	PromptTokensTotal     float64
	GenerationTokensTotal float64

	// Latency metrics (seconds)
	TTFT_P50  float64
	TTFT_P95  float64
	TTFT_P99  float64
	TPOT_P50  float64 // time per output token
	TPOT_P95  float64
	E2E_P50   float64 // end to end latency
	E2E_P95   float64

	// GPU metrics
	GPUCacheUsagePercent float64
	NumPreemptions       float64

	// Computed
	EstimatedGPUSeconds float64
	EstimatedCostUSD    float64
}

// NewVLLMCollector creates a collector targeting a vLLM metrics endpoint.
func NewVLLMCollector(endpoint string, gpuHourlyRate float64, modelName string) *VLLMCollector {
	return &VLLMCollector{
		endpoint:  endpoint,
		gpuHourly: gpuHourlyRate,
		modelName: modelName,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Scrape fetches and parses vLLM Prometheus metrics.
func (c *VLLMCollector) Scrape() (*VLLMMetrics, error) {
	resp, err := c.client.Get(c.endpoint)
	if err != nil {
		return nil, fmt.Errorf("scraping vLLM metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vLLM metrics returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading vLLM metrics: %w", err)
	}

	return c.Parse(string(body))
}

// Parse extracts VLLMMetrics from raw Prometheus exposition text.
func (c *VLLMCollector) Parse(raw string) (*VLLMMetrics, error) {
	m := &VLLMMetrics{}

	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		switch {
		case matchMetric(line, "vllm:num_requests_running"):
			m.RequestsRunning = parseValue(line)
		case matchMetric(line, "vllm:num_requests_waiting"):
			m.RequestsWaiting = parseValue(line)
		case matchMetric(line, "vllm:num_requests_total"):
			m.RequestsTotal = parseValue(line)
		case matchMetric(line, "vllm:prompt_tokens_total"):
			m.PromptTokensTotal = parseValue(line)
		case matchMetric(line, "vllm:generation_tokens_total"):
			m.GenerationTokensTotal = parseValue(line)
		case matchMetric(line, "vllm:gpu_cache_usage_perc"):
			m.GPUCacheUsagePercent = parseValue(line) * 100
		case matchMetric(line, "vllm:num_preemptions_total"):
			m.NumPreemptions = parseValue(line)
		case matchQuantile(line, "vllm:time_to_first_token_seconds", "0.5"):
			m.TTFT_P50 = parseValue(line)
		case matchQuantile(line, "vllm:time_to_first_token_seconds", "0.95"):
			m.TTFT_P95 = parseValue(line)
		case matchQuantile(line, "vllm:time_to_first_token_seconds", "0.99"):
			m.TTFT_P99 = parseValue(line)
		case matchQuantile(line, "vllm:time_per_output_token_seconds", "0.5"):
			m.TPOT_P50 = parseValue(line)
		case matchQuantile(line, "vllm:time_per_output_token_seconds", "0.95"):
			m.TPOT_P95 = parseValue(line)
		case matchQuantile(line, "vllm:e2e_request_latency_seconds", "0.5"):
			m.E2E_P50 = parseValue(line)
		case matchQuantile(line, "vllm:e2e_request_latency_seconds", "0.95"):
			m.E2E_P95 = parseValue(line)
		}
	}

	// Estimate GPU seconds: TTFT + (generation_tokens * TPOT) per request
	if m.RequestsTotal > 0 && m.TPOT_P50 > 0 {
		avgGenTokensPerReq := m.GenerationTokensTotal / m.RequestsTotal
		m.EstimatedGPUSeconds = m.RequestsTotal * (m.TTFT_P50 + avgGenTokensPerReq*m.TPOT_P50)
	}

	// Compute cost
	if m.EstimatedGPUSeconds > 0 {
		m.EstimatedCostUSD = (m.EstimatedGPUSeconds / 3600.0) * c.gpuHourly
	}

	return m, nil
}

func matchMetric(line, name string) bool {
	return strings.HasPrefix(line, name+" ") || strings.HasPrefix(line, name+"{")
}

func matchQuantile(line, name, quantile string) bool {
	return strings.Contains(line, name) && strings.Contains(line, `quantile="`+quantile+`"`)
}

// parseValue extracts the numeric value from one Prometheus exposition
// line. The exposition format is:
//
//	metric_name{labels} value [optional_timestamp_ms]
//
// Two subtleties the old take-last-field impl got wrong:
//
//   - Optional trailing timestamp. Some exporters (including vLLM
//     under certain settings) append the sample time as a third
//     field. Taking parts[len-1] silently returned that timestamp as
//     "the value" — cost metrics jumped to ~1.7e12 for anyone whose
//     vLLM had timestamps enabled. Walk left-to-right and return the
//     first token that parses as a *finite* float; metric name +
//     labels never parse as a float, so this is unambiguous.
//
//   - Non-finite values. strconv.ParseFloat accepts NaN, +Inf, -Inf
//     without error — all three are spec-legal Prometheus values
//     (uninitialized gauges, divide-by-zero counters). Any of them
//     poisons every downstream `+=` accumulator in main.go's delta
//     tracker AND makes encoding/json refuse to marshal the metrics
//     response. Treat them as value-missing and keep scanning.
//
// Same fix landed in gpudab-server's internal/source/gpucast.go
// (parsePrometheusValue) and previously in pkg/native/parser_pure.go
// (now deleted). This is the third repo where the same collector
// pattern needed the same fix.
func parseValue(line string) float64 {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return 0
	}
	for _, tok := range parts[1:] {
		v, err := strconv.ParseFloat(tok, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		return v
	}
	return 0
}

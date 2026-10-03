package metrics

import "github.com/prometheus/client_golang/prometheus"

// Inference cost and performance metrics for GPU workloads.
// These are the metrics that don't exist anywhere else — the gap
// between Kubecost (infra cost) and LangSmith (app traces).

var (
	// InferenceCostUSD tracks the estimated cost in USD per inference request,
	// attributed to model and tenant.
	//
	// Gauge semantics: reports the current-scrape lifetime cost, so
	// alerting on this metric alone is fragile — vLLM's counters reset
	// on process restart, and this gauge collapses to the since-restart
	// value. Prefer alerting on inference_cost_usd_total (below), which
	// is delta-accumulated and survives restarts. This gauge is kept
	// for backwards compatibility with dashboards that plot it directly.
	InferenceCostUSD = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "gpucast",
		Name:      "inference_cost_usd",
		Help:      "Current-scrape estimated cost in USD, by model and tenant. Fragile across vLLM restarts — see inference_cost_usd_total.",
	}, []string{"model", "tenant", "namespace"})

	// InferenceCostUSDTotal accumulates cost across scrapes using per-
	// scrape deltas, so counter resets on vLLM restart do not silently
	// erase prior spend the way the Gauge does. This is the metric to
	// alert on, put into budget calculations, or rate() in Grafana.
	InferenceCostUSDTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gpucast",
		Name:      "inference_cost_usd_total",
		Help:      "Cumulative inference cost in USD across all scrapes, delta-accumulated so it survives vLLM restarts.",
	}, []string{"model", "tenant", "namespace"})

	// GPUSecondsPerRequestMean carries the per-scrape MEAN GPU-seconds
	// per request. A prior version declared a HistogramVec and called
	// .Observe(EstimatedGPUSeconds / RequestsTotal) once per scrape —
	// so what landed in the histogram was a sequence of per-scrape
	// means, not individual request samples; histogram_quantile(0.95,
	// ...) on such a histogram is the 95th percentile of means, not
	// of requests, and is approximately the mean. The honest shape:
	// gpucast has no per-request GPU-seconds signal (vLLM /metrics
	// doesn't expose it), only the per-scrape aggregate derivable
	// from EstimatedGPUSeconds / RequestsTotal. Emit the mean under a
	// name that says 'mean' so panels and alerts stop claiming
	// distribution semantics that aren't there.
	GPUSecondsPerRequestMean = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "gpucast",
		Name:      "gpu_seconds_per_request_mean",
		Help:      "Mean GPU-seconds per inference request this scrape (EstimatedGPUSeconds / RequestsTotal — NOT a distribution; no per-request sample is available from vLLM /metrics).",
	}, []string{"model", "tenant"})

	// TokensPerGPUDollar measures token throughput efficiency — higher is better.
	TokensPerGPUDollar = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "gpucast",
		Name:      "tokens_per_gpu_dollar",
		Help:      "Tokens generated per dollar of GPU cost. Higher = more efficient.",
	}, []string{"model"})

	// `wasted_gpu_seconds_total` lived here as a CounterVec that was
	// registered but never written: no .Add call in main.go, in the
	// collector, or anywhere else in gpucast's tree. The alert
	// `GPUWasteHigh` queried rate(wasted_gpu_seconds_total[15m]) > 100
	// — rate(0) is 0, 0 > 100 is false under every condition, so the
	// alert was mathematically incapable of firing. The dashboard
	// panel `Wasted GPU Seconds (by reason)` was permanently empty,
	// which reads as "no waste" rather than "not measured" (same
	// signal-vs-silence shape operators have been bitten by in other
	// repos this week). Deleted rather than left as scaffolding;
	// wiring would need gpucast to actually detect idle / fragmentation
	// / cold-start periods, which the current vLLM /metrics scrape has
	// no signal for. Honest re-introduction path is to add it back the
	// same commit that wires a real waste detector.

	// InferenceRequestsTotal counts requests per model/tenant.
	InferenceRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gpucast",
		Name:      "inference_requests_total",
		Help:      "Total inference requests by model and tenant.",
	}, []string{"model", "tenant", "status"})

	// TokensProcessedTotal counts tokens across input and output.
	TokensProcessedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gpucast",
		Name:      "tokens_processed_total",
		Help:      "Total tokens processed (prompt + completion).",
	}, []string{"model", "tenant", "direction"})

	// TTFT percentile gauges — one per vLLM-reported quantile. A prior
	// version declared a `time_to_first_token_seconds` HistogramVec and
	// called .Observe(m.TTFT_P50) ONCE per scrape, feeding a single
	// pre-aggregated P50 scalar into the histogram as if it were an
	// individual request sample. Taking histogram_quantile(0.95, ...)
	// on a histogram populated with only P50 values returned
	// approximately the P50, so the alert and the three dashboard
	// panels (p50/p95/p99) all showed roughly the same number while
	// claiming three different percentiles. gpucast scrapes vLLM
	// /metrics which already carries all three percentiles — emit them
	// as separate gauges so each panel reads what its label claims.
	// Name each gauge for the percentile it carries, not for a
	// distribution it doesn't have.
	TTFTP50Seconds = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "gpucast",
		Name:      "ttft_p50_seconds",
		Help:      "Time to first token, p50 (median), from vLLM's own aggregates.",
	}, []string{"model"})
	TTFTP95Seconds = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "gpucast",
		Name:      "ttft_p95_seconds",
		Help:      "Time to first token, p95, from vLLM's own aggregates.",
	}, []string{"model"})
	TTFTP99Seconds = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "gpucast",
		Name:      "ttft_p99_seconds",
		Help:      "Time to first token, p99, from vLLM's own aggregates.",
	}, []string{"model"})

	// Per-tenant cost lived here as `tenant_budget_used_usd` and was
	// never populated — vLLM /metrics is process-level and carries no
	// tenant identity, so gpucast standalone has nothing to attribute
	// spend by. Tenant is only knowable from modelgate's audit stream
	// (proxy.AuditEvent.Tenant), which means per-tenant cost belongs at
	// the audit-consumer layer (gpudab), not here. Removed rather than
	// left permanently zero — a dashboard panel reading a zero series
	// reads as "no spend" instead of "not measured."

	// VLLMKVCacheUsagePercent tracks vLLM's KV-cache occupancy ratio
	// (how much of the per-model KV-cache memory is currently in use
	// across active sequences). This is NOT GPU compute utilization —
	// nvidia-smi utilization-% comes from the hardware's SM counters
	// and gpucast does not scrape them. vLLM's /metrics only exposes
	// the KV-cache number; mislabelling it as GPU utilization is the
	// pattern that caused the inverted-waste-signal bug in gpudab's
	// dashboard (low KV cache = efficient batching, was being read as
	// "GPU idle, candidate for deprovisioning"). Named for exactly
	// what it measures so downstream consumers can't accidentally
	// make the opposite claim.
	//
	// gpu_id="0" + model is a shape kept for compat with the prior
	// metric; vLLM's KV cache is per-model-instance and the gpu_id
	// label is effectively deterministic until gpucast grows a
	// per-GPU scraper.
	VLLMKVCacheUsagePercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "gpucast",
		Name:      "vllm_kv_cache_usage_percent",
		Help:      "vLLM KV-cache occupancy percentage (NOT GPU compute utilization — gpucast does not scrape SM counters).",
	}, []string{"gpu_id", "model"})
)

// RegisterAll registers all gpucast metrics with the given registry.
func RegisterAll(reg prometheus.Registerer) {
	reg.MustRegister(
		InferenceCostUSD,
		InferenceCostUSDTotal,
		GPUSecondsPerRequestMean,
		TokensPerGPUDollar,
		InferenceRequestsTotal,
		TokensProcessedTotal,
		TTFTP50Seconds,
		TTFTP95Seconds,
		TTFTP99Seconds,
		VLLMKVCacheUsagePercent,
	)
}

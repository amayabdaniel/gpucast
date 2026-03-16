package metrics

import "github.com/prometheus/client_golang/prometheus"

// Inference cost and performance metrics for GPU workloads.
// These are the metrics that don't exist anywhere else — the gap
// between Kubecost (infra cost) and LangSmith (app traces).

var (
	// InferenceCostUSD tracks the estimated cost in USD per inference request,
	// attributed to model and tenant.
	InferenceCostUSD = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "gpucast",
		Name:      "inference_cost_usd",
		Help:      "Estimated cost in USD per inference request, by model and tenant.",
	}, []string{"model", "tenant", "namespace"})

	// GPUSecondsPerRequest tracks GPU-seconds consumed per request at various percentiles.
	GPUSecondsPerRequest = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "gpucast",
		Name:      "gpu_seconds_per_request",
		Help:      "GPU-seconds consumed per inference request.",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0, 10.0},
	}, []string{"model", "tenant"})

	// TokensPerGPUDollar measures token throughput efficiency — higher is better.
	TokensPerGPUDollar = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "gpucast",
		Name:      "tokens_per_gpu_dollar",
		Help:      "Tokens generated per dollar of GPU cost. Higher = more efficient.",
	}, []string{"model"})

	// WastedGPUSeconds tracks GPU time not used for inference.
	WastedGPUSeconds = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gpucast",
		Name:      "wasted_gpu_seconds_total",
		Help:      "GPU-seconds wasted due to idle, fragmentation, or cold starts.",
	}, []string{"reason", "model"})

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

	// TimeToFirstTokenSeconds tracks TTFT latency.
	TimeToFirstTokenSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "gpucast",
		Name:      "time_to_first_token_seconds",
		Help:      "Time to first token (TTFT) in seconds.",
		Buckets:   []float64{0.05, 0.1, 0.2, 0.5, 1.0, 2.0, 5.0},
	}, []string{"model"})

	// TenantBudgetUsedUSD tracks cumulative spend per tenant.
	TenantBudgetUsedUSD = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "gpucast",
		Name:      "tenant_budget_used_usd",
		Help:      "Cumulative GPU inference spend per tenant in USD.",
	}, []string{"tenant", "namespace"})

	// GPUUtilizationPercent tracks current GPU utilization.
	GPUUtilizationPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "gpucast",
		Name:      "gpu_utilization_percent",
		Help:      "Current GPU compute utilization percentage.",
	}, []string{"gpu_id", "model"})
)

// RegisterAll registers all gpucast metrics with the given registry.
func RegisterAll(reg prometheus.Registerer) {
	reg.MustRegister(
		InferenceCostUSD,
		GPUSecondsPerRequest,
		TokensPerGPUDollar,
		WastedGPUSeconds,
		InferenceRequestsTotal,
		TokensProcessedTotal,
		TimeToFirstTokenSeconds,
		TenantBudgetUsedUSD,
		GPUUtilizationPercent,
	)
}

# gpucast

Kubernetes-native inference cost tracking.

Cost per request. Per model. For self-hosted GPU inference.

Per-tenant attribution is not sourced from vLLM (its `/metrics` is process-level and carries no tenant identity); it belongs at the audit-consumer layer — e.g., gpudab reading the modelgate audit stream, where every request has a tenant field.

## The problem

Teams spend $5K-50K/month on GPU inference and can't answer "what does this model cost per request?" Kubecost tracks node costs. LiteLLM estimates token costs. Nobody connects: request → GPU time → dollars → tenant.

## How it works

```
  vLLM /metrics ──────▶ gpucast ──────▶ Prometheus ──────▶ Grafana
  (tokens, latency,      (correlate      (store)           (dashboards
   GPU cache, queue)       + cost calc)                     + alerts)
```

gpucast scrapes your vLLM Prometheus endpoint, correlates request metrics with GPU costs, and exports inference-specific metrics that Kubecost and standard tools don't provide.

## Quick start

```bash
# Standalone
go install github.com/amayabdaniel/gpucast@latest
gpucast --vllm-endpoint=http://vllm-svc:8000/metrics --gpu-hourly-rate=0.80 --model=qwen3-8b

# Helm (with Prometheus Operator)
helm install gpucast deploy/helm/gpucast/ -n monitoring --create-namespace
```

## Metrics exported

| Metric | Type | What it tracks |
|---|---|---|
| `gpucast_inference_cost_usd` | Gauge | Estimated cost per request in USD, by model and tenant |
| `gpucast_gpu_seconds_per_request` | Histogram | GPU-seconds consumed per request (p50/p95/p99) |
| `gpucast_tokens_per_gpu_dollar` | Gauge | Token throughput efficiency — higher = cheaper |
| `gpucast_inference_requests_total` | Counter | Request count by model, tenant, status |
| `gpucast_tokens_processed_total` | Counter | Tokens by model, tenant, direction (prompt/completion) |
| `gpucast_time_to_first_token_seconds` | Histogram | TTFT latency (p50/p95/p99) |
| `gpucast_gpu_utilization_percent` | Gauge | GPU compute utilization |

## Grafana dashboard

Ships with a 7-panel dashboard:

| Panel | Visualization |
|---|---|
| Inference Cost per Model | Time series (USD/request) |
| GPU Seconds per Request (p95) | Time series |
| Tokens per GPU Dollar | Stat with thresholds (red/yellow/green) |
| GPU Utilization | Gauge (0-100%) |
| Requests per Model | Bar chart (req/s) |
| Time to First Token (p50/p95/p99) | Time series |
| Tokens Processed (prompt vs completion) | Time series |

Auto-provisioned via ConfigMap when `grafanaDashboard.enabled: true`.

## Alert rules

Ships with PrometheusRule alerts:

| Alert | Condition | Severity |
|---|---|---|
| InferenceCostSpike | Cost > $0.10/request for 5m | Warning |
| HighTimeToFirstToken | TTFT p95 > 2s for 5m | Warning |
| SlowInferenceRequests | GPU sec/request p95 > 5s for 5m | Warning |
| GPUUnderutilized | GPU util < 20% for 15m | Info |
| LowTokenEfficiency | < 5000 tokens per GPU dollar for 10m | Warning |

## Security

- Pod runs as non-root (uid 65532), seccomp RuntimeDefault, drop all capabilities
- Read-only root filesystem
- NetworkPolicy: only Prometheus ingress, only inference endpoint egress
- Endpoint URL validation (scheme, path, traversal checks)
- GPU hourly rate bounds checking

## Related projects

- [inferctl](https://github.com/amayabdaniel/inferctl) — deploy the models gpucast monitors
- [modelgate](https://github.com/amayabdaniel/modelgate) — secure the inference API gpucast tracks

## Tests

```bash
make test    # 19 tests
make build   # builds to bin/gpucast
```

## License

Apache 2.0

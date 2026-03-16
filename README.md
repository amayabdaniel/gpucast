# gpucast

Kubernetes-native inference cost tracking.

Cost per request. Per model. Per tenant. For self-hosted GPU inference.

## The problem

Teams spend $5K-50K/month on GPU inference and can't answer "what does this model cost per request?" Kubecost tracks node costs. LiteLLM estimates token costs. Nobody connects: request → GPU time → batch efficiency → dollars → tenant.

## What gpucast does

A Prometheus exporter + Grafana dashboard pack that correlates:

1. **Demand plane** — requests, tokens, tenants (from gateway OTel spans)
2. **Execution plane** — GPU runtime, batching, queueing (from vLLM/Triton metrics + DCGM exporter)
3. **Cost plane** — amortized GPU $/hr, spot/reserved pricing, cluster overhead

Exports metrics like:

```
inference_cost_usd{model="llama3-70b", tenant="support-team"} 0.0043
gpu_seconds_per_request_p95{model="llama3-70b"} 1.82
wasted_gpu_seconds{reason="idle"} 342.5
tokens_per_gpu_dollar{model="qwen3-8b"} 48200
```

Ships with:
- Grafana dashboards (cost per model, cost per tenant, GPU efficiency)
- Alert rules (cost anomaly, utilization drop, budget threshold)
- Helm chart for one-command install

## Status

Early development. See `projectz/potential-projectz.md` for full plan.

## License

Apache 2.0

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/amayabdaniel/gpucast/pkg/collector"
	"github.com/amayabdaniel/gpucast/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	addr := flag.String("listen", ":9400", "address to listen on for metrics")
	vllmEndpoint := flag.String("vllm-endpoint", "", "vLLM metrics endpoint (e.g., http://vllm-svc:8000/metrics)")
	gpuRate := flag.Float64("gpu-hourly-rate", 0.80, "GPU hourly cost in USD for cost estimation")
	modelName := flag.String("model", "default", "model name for metric labels")
	scrapeInterval := flag.Duration("scrape-interval", 15*time.Second, "how often to scrape vLLM metrics")
	flag.Parse()

	reg := prometheus.NewRegistry()
	metrics.RegisterAll(reg)

	// Start vLLM collector if endpoint is configured
	if *vllmEndpoint != "" {
		if err := collector.ValidateEndpoint(*vllmEndpoint); err != nil {
			log.Fatalf("gpucast: invalid vllm endpoint: %v", err)
		}
		if err := collector.ValidateGPUHourlyRate(*gpuRate); err != nil {
			log.Fatalf("gpucast: invalid gpu rate: %v", err)
		}

		c := collector.NewVLLMCollector(*vllmEndpoint, *gpuRate, *modelName)
		go runCollectorLoop(c, *modelName, *scrapeInterval)
		log.Printf("gpucast: scraping vLLM at %s every %s (GPU rate: $%.2f/hr)", *vllmEndpoint, *scrapeInterval, *gpuRate)
	} else {
		log.Println("gpucast: no --vllm-endpoint specified, running in metrics-only mode")
		log.Println("gpucast: use --vllm-endpoint=http://vllm-svc:8000/metrics to enable collection")
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// Explicit *http.Server so we get ReadHeaderTimeout — the default
	// http.ListenAndServe uses a zero-valued Server which never times
	// out header reads, letting a slowloris-style attacker tie up
	// every listener slot by trickling bytes forever. WriteTimeout
	// covers the response side; IdleTimeout bounds keep-alive
	// sessions. Values are generous enough for a scraper that pulls
	// large exposition texts under load, tight enough to reject
	// pathological clients.
	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("gpucast: serving metrics on %s/metrics", *addr)
	log.Fatal(srv.ListenAndServe())
}

func runCollectorLoop(c *collector.VLLMCollector, modelName string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// prev holds the previous scrape's raw VLLMMetrics. Delta arithmetic
	// lives in collector.ComputeDelta — extracted from this loop so it
	// can be exercised under go test rather than only via a live vLLM.
	// The zero-value first-scrape produces Delta == current, matching
	// the prior inline behavior.
	var prev collector.VLLMMetrics

	for range ticker.C {
		m, err := c.Scrape()
		if err != nil {
			log.Printf("gpucast: scrape error: %v", err)
			continue
		}

		d := collector.ComputeDelta(prev, *m)
		prev = *m

		// Gauges — set directly. InferenceCostUSD is per-scrape lifetime
		// and fragile across restarts by design; alerting/budgeting
		// callers should read inference_cost_usd_total below instead.
		metrics.InferenceCostUSD.WithLabelValues(modelName, "all", "default").Set(m.EstimatedCostUSD)
		// VLLMKVCacheUsagePercent is populated from vLLM's KV-cache
		// occupancy (m.GPUCacheUsagePercent — the vLLM-side field name
		// is historical, the value is KV-cache usage, not GPU compute).
		// Named explicitly on the Prometheus side so downstream can't
		// misread it as compute utilization — see the metric godoc.
		metrics.VLLMKVCacheUsagePercent.WithLabelValues("0", modelName).Set(m.GPUCacheUsagePercent)

		// Counters — add deltas only
		if d.Requests > 0 {
			metrics.InferenceRequestsTotal.WithLabelValues(modelName, "all", "ok").Add(d.Requests)
		}
		if d.PromptTokens > 0 {
			metrics.TokensProcessedTotal.WithLabelValues(modelName, "all", "prompt").Add(d.PromptTokens)
		}
		if d.GenerationTokens > 0 {
			metrics.TokensProcessedTotal.WithLabelValues(modelName, "all", "completion").Add(d.GenerationTokens)
		}
		if d.CostUSD > 0 {
			metrics.InferenceCostUSDTotal.WithLabelValues(modelName, "all", "default").Add(d.CostUSD)
		}

		if m.EstimatedCostUSD > 0 {
			totalTokens := m.PromptTokensTotal + m.GenerationTokensTotal
			metrics.TokensPerGPUDollar.WithLabelValues(modelName).Set(totalTokens / m.EstimatedCostUSD)
		}

		// Emit vLLM's own TTFT percentile aggregates as separate gauges
		// — see the TTFT*Seconds metric godocs for why this isn't a
		// histogram anymore. Only writing values vLLM reported (>0 as
		// the "did we actually scrape this quantile" guard).
		if m.TTFT_P50 > 0 {
			metrics.TTFTP50Seconds.WithLabelValues(modelName).Set(m.TTFT_P50)
		}
		if m.TTFT_P95 > 0 {
			metrics.TTFTP95Seconds.WithLabelValues(modelName).Set(m.TTFT_P95)
		}
		if m.TTFT_P99 > 0 {
			metrics.TTFTP99Seconds.WithLabelValues(modelName).Set(m.TTFT_P99)
		}

		// Per-scrape MEAN GPU-seconds per request. Set as a gauge, not
		// observed into a histogram, because what gpucast has is the
		// aggregate (EstimatedGPUSeconds / RequestsTotal) — see the
		// metric godoc for why observing a stream of means into a
		// histogram and taking quantiles over it was misleading.
		if m.EstimatedGPUSeconds > 0 && m.RequestsTotal > 0 {
			avgGPUSec := m.EstimatedGPUSeconds / m.RequestsTotal
			metrics.GPUSecondsPerRequestMean.WithLabelValues(modelName, "all").Set(avgGPUSec)
		}
	}
}

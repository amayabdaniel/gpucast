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

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
	http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	log.Printf("gpucast: serving metrics on %s/metrics", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
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
		metrics.GPUUtilizationPercent.WithLabelValues("0", modelName).Set(m.GPUCacheUsagePercent)

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

		if m.TTFT_P50 > 0 {
			metrics.TimeToFirstTokenSeconds.WithLabelValues(modelName).Observe(m.TTFT_P50)
		}

		if m.EstimatedGPUSeconds > 0 && m.RequestsTotal > 0 {
			avgGPUSec := m.EstimatedGPUSeconds / m.RequestsTotal
			metrics.GPUSecondsPerRequest.WithLabelValues(modelName, "all").Observe(avgGPUSec)
		}
	}
}

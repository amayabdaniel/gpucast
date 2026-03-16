package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/amayabdaniel/gpucast/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	addr := flag.String("listen", ":9400", "address to listen on for metrics")
	flag.Parse()

	reg := prometheus.NewRegistry()
	metrics.RegisterAll(reg)

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	log.Printf("gpucast: serving metrics on %s/metrics", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

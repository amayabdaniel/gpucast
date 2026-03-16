package metrics

import "github.com/prometheus/client_golang/prometheus"

// NewTestRegistry creates a fresh registry for testing.
func NewTestRegistry() *prometheus.Registry {
	return prometheus.NewRegistry()
}

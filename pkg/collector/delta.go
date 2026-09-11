package collector

// Delta is what changed between two consecutive vLLM scrapes. All
// fields are non-negative; a scrape where vLLM restarted (counter went
// backwards) is handled by treating the current lifetime value as the
// delta — matching Prometheus rate() semantics for a counter reset.
//
// Extracted from the inline loop in main.go so the delta arithmetic is
// exercisable under `go test`. Money math that lives only inside a
// goroutine can't be verified against known inputs — the log-app 100x
// bugs found by another session had the same shape.
type Delta struct {
	Requests         float64
	PromptTokens     float64
	GenerationTokens float64
	// CostUSD is the change in EstimatedCostUSD across the two scrapes,
	// also reset-guarded. Previously main.go set InferenceCostUSD as a
	// Gauge to the per-scrape lifetime cost — which silently collapsed
	// to the since-restart value on every vLLM restart, disagreeing
	// with the properly-delta-tracked request/token counters. Emitting
	// this delta into a proper Counter keeps the two consistent.
	CostUSD float64
}

// ComputeDelta returns the positive-only difference between two
// consecutive VLLMMetrics scrapes. prev may be a zero-value struct on
// the first scrape after boot; in that case Delta equals current
// (every value is fresh). A counter reset (any current field strictly
// less than the prev field) is detected per-field and the delta for
// that field becomes the current value — same behavior as Prometheus
// rate() and the historical inline logic in main.go, now separated so
// it can be tested.
func ComputeDelta(prev, current VLLMMetrics) Delta {
	return Delta{
		Requests:         deltaOrCurrent(prev.RequestsTotal, current.RequestsTotal),
		PromptTokens:     deltaOrCurrent(prev.PromptTokensTotal, current.PromptTokensTotal),
		GenerationTokens: deltaOrCurrent(prev.GenerationTokensTotal, current.GenerationTokensTotal),
		CostUSD:          deltaOrCurrent(prev.EstimatedCostUSD, current.EstimatedCostUSD),
	}
}

// deltaOrCurrent returns current-prev when non-negative, otherwise
// current. Non-negative means "vLLM's counter grew normally"; negative
// means "counter reset since last scrape" (process restart).
func deltaOrCurrent(prev, current float64) float64 {
	d := current - prev
	if d < 0 {
		return current
	}
	return d
}

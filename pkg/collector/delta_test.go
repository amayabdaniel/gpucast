package collector

import (
	"math"
	"testing"
)

// TestComputeDelta_FirstScrapeReturnsCurrentValues locks in the boot
// contract: prev is a zero-value VLLMMetrics on the very first scrape,
// and the delta must equal current — otherwise the first scrape would
// silently drop everything (a zero baseline followed by a "delta" of
// zero would mean "no requests happened yet" even after minutes of
// production traffic).
func TestComputeDelta_FirstScrapeReturnsCurrentValues(t *testing.T) {
	current := VLLMMetrics{
		RequestsTotal:         100,
		PromptTokensTotal:     5000,
		GenerationTokensTotal: 2500,
		EstimatedCostUSD:      0.15,
	}
	d := ComputeDelta(VLLMMetrics{}, current)

	if d.Requests != 100 {
		t.Errorf("first-scrape Requests: want 100, got %v", d.Requests)
	}
	if d.PromptTokens != 5000 || d.GenerationTokens != 2500 {
		t.Errorf("first-scrape tokens: want 5000/2500, got %v/%v", d.PromptTokens, d.GenerationTokens)
	}
	if d.CostUSD != 0.15 {
		t.Errorf("first-scrape cost: want 0.15, got %v", d.CostUSD)
	}
}

// TestComputeDelta_SecondScrape locks in the normal path — monotonic
// growth produces exact positive deltas. This is the arithmetic that
// feeds inference_cost_usd_total: a bug here becomes a billing bug.
func TestComputeDelta_SecondScrape(t *testing.T) {
	prev := VLLMMetrics{
		RequestsTotal:         100,
		PromptTokensTotal:     5000,
		GenerationTokensTotal: 2500,
		EstimatedCostUSD:      0.15,
	}
	current := VLLMMetrics{
		RequestsTotal:         150,
		PromptTokensTotal:     7250,
		GenerationTokensTotal: 3800,
		EstimatedCostUSD:      0.234,
	}
	d := ComputeDelta(prev, current)

	if d.Requests != 50 {
		t.Errorf("Requests delta: want 50, got %v", d.Requests)
	}
	if d.PromptTokens != 2250 {
		t.Errorf("PromptTokens delta: want 2250, got %v", d.PromptTokens)
	}
	if d.GenerationTokens != 1300 {
		t.Errorf("GenerationTokens delta: want 1300, got %v", d.GenerationTokens)
	}
	if math.Abs(d.CostUSD-0.084) > 1e-9 {
		t.Errorf("CostUSD delta: want 0.084, got %v", d.CostUSD)
	}
}

// TestComputeDelta_VLLMRestart is the specific bug this extraction
// fixes. When vLLM's process restarts, every counter goes back to a
// small value (typically the counts accumulated since restart). The
// old cost gauge silently collapsed on this: the operator saw their
// month-to-date spend jump from $500 to $0.02 with no explanation.
// The delta must treat a decrease as "current is the delta since
// restart" — same as Prometheus rate() behavior for counter resets.
func TestComputeDelta_VLLMRestart(t *testing.T) {
	prev := VLLMMetrics{
		RequestsTotal:         10000,
		PromptTokensTotal:     500000,
		GenerationTokensTotal: 250000,
		EstimatedCostUSD:      12.50,
	}
	// vLLM restarted between scrapes; counters reset then ramped a bit.
	current := VLLMMetrics{
		RequestsTotal:         5,
		PromptTokensTotal:     250,
		GenerationTokensTotal: 120,
		EstimatedCostUSD:      0.008,
	}
	d := ComputeDelta(prev, current)

	if d.Requests != 5 {
		t.Errorf("post-restart Requests: want current=5 (not negative), got %v", d.Requests)
	}
	if d.PromptTokens != 250 || d.GenerationTokens != 120 {
		t.Errorf("post-restart tokens: want 250/120, got %v/%v", d.PromptTokens, d.GenerationTokens)
	}
	if math.Abs(d.CostUSD-0.008) > 1e-9 {
		t.Errorf("post-restart cost: want 0.008 (delta clamped to current), got %v", d.CostUSD)
	}
	// Explicit non-negative invariant — the whole point of the guard.
	if d.CostUSD < 0 {
		t.Errorf("CostUSD delta must never be negative, got %v", d.CostUSD)
	}
}

// TestComputeDelta_NoChange asserts an idempotent scrape (nothing
// happened between them) yields all-zero deltas — importantly, cost
// must NOT be misreported as full lifetime. If ComputeDelta returned
// current-when-equal (a subtle off-by-one), every idle scrape would
// double-charge into inference_cost_usd_total.
func TestComputeDelta_NoChange(t *testing.T) {
	m := VLLMMetrics{
		RequestsTotal:    100,
		EstimatedCostUSD: 0.15,
	}
	d := ComputeDelta(m, m)
	if d.Requests != 0 || d.PromptTokens != 0 || d.GenerationTokens != 0 || d.CostUSD != 0 {
		t.Errorf("no-change scrape must produce zero delta, got %+v", d)
	}
}

// TestComputeDelta_PartialResetOneField locks in per-field
// independence: a counter that reset without other counters resetting
// still gets the safe clamp. Not observed in practice (vLLM resets all
// at once on restart), but the arithmetic contract holds regardless.
func TestComputeDelta_PartialResetOneField(t *testing.T) {
	prev := VLLMMetrics{
		RequestsTotal:         100,
		PromptTokensTotal:     5000,
		GenerationTokensTotal: 2500,
		EstimatedCostUSD:      0.15,
	}
	// Only GenerationTokens reset; others grew normally.
	current := VLLMMetrics{
		RequestsTotal:         150,
		PromptTokensTotal:     7250,
		GenerationTokensTotal: 100,  // reset
		EstimatedCostUSD:      0.234,
	}
	d := ComputeDelta(prev, current)

	if d.Requests != 50 {
		t.Errorf("unaffected Requests delta: want 50, got %v", d.Requests)
	}
	if d.GenerationTokens != 100 {
		t.Errorf("reset GenerationTokens must clamp to current=100, got %v", d.GenerationTokens)
	}
	if math.Abs(d.CostUSD-0.084) > 1e-9 {
		t.Errorf("unaffected CostUSD delta: want 0.084, got %v", d.CostUSD)
	}
}

// TestComputeDelta_KnownPricingArithmetic is the concrete money
// verification — plug in numbers a human would compute by hand for a
// realistic vLLM scrape and assert the CostUSD delta matches. This is
// what the peer's "money math with tests against known inputs" ask
// looks like end-to-end: an operator observing the same delta cost
// hitting inference_cost_usd_total.
func TestComputeDelta_KnownPricingArithmetic(t *testing.T) {
	// Baseline scrape: 1000 requests, $0.80/hr GPU, model has been
	// running long enough that EstimatedCostUSD = $1.234.
	prev := VLLMMetrics{RequestsTotal: 1000, EstimatedCostUSD: 1.234}
	// After the next 15-second scrape interval, 47 more requests
	// completed and cost rose by exactly 3.1 cents.
	current := VLLMMetrics{RequestsTotal: 1047, EstimatedCostUSD: 1.265}
	d := ComputeDelta(prev, current)

	if d.Requests != 47 {
		t.Errorf("want 47 request delta, got %v", d.Requests)
	}
	if math.Abs(d.CostUSD-0.031) > 1e-9 {
		t.Errorf("want $0.031 cost delta, got $%.6f", d.CostUSD)
	}
	// The invariant the Counter depends on: cost delta scales with
	// request delta in a way the operator can predict — the per-scrape
	// average is $0.031 / 47 ≈ $0.00066 per request. Verifiable.
	perReq := d.CostUSD / d.Requests
	if math.Abs(perReq-0.000659574) > 1e-6 {
		t.Errorf("derived per-request cost: want ~$0.000660, got $%.9f", perReq)
	}
}

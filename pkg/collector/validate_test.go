package collector

import "testing"

func TestValidateEndpoint_Valid(t *testing.T) {
	valid := []string{
		"http://vllm-svc:8000/metrics",
		"http://localhost:8000/metrics",
		"https://inference.prod.svc:8000/metrics",
		"http://10.0.0.5:8000/metrics",
		"http://vllm:8000/",
		"http://vllm:8000",
	}
	for _, ep := range valid {
		if err := ValidateEndpoint(ep); err != nil {
			t.Errorf("expected valid endpoint %q, got: %v", ep, err)
		}
	}
}

func TestValidateEndpoint_Invalid(t *testing.T) {
	invalid := []struct {
		endpoint string
		reason   string
	}{
		{"", "empty"},
		{"ftp://host:21/metrics", "wrong scheme"},
		{"http://", "no host"},
		{"http://host:8000/../../../etc/passwd", "path traversal"},
		{"http://host:8000/admin/secret", "wrong path"},
		{"not-a-url", "not a URL"},
	}
	for _, tt := range invalid {
		if err := ValidateEndpoint(tt.endpoint); err == nil {
			t.Errorf("expected error for %s endpoint: %q", tt.reason, tt.endpoint)
		}
	}
}

func TestValidateGPUHourlyRate_Valid(t *testing.T) {
	valid := []float64{0, 0.80, 3.60, 40.0, 100.0}
	for _, rate := range valid {
		if err := ValidateGPUHourlyRate(rate); err != nil {
			t.Errorf("expected valid rate %f, got: %v", rate, err)
		}
	}
}

func TestValidateGPUHourlyRate_Invalid(t *testing.T) {
	if err := ValidateGPUHourlyRate(-1.0); err == nil {
		t.Error("expected error for negative rate")
	}
	if err := ValidateGPUHourlyRate(500.0); err == nil {
		t.Error("expected error for unreasonably high rate")
	}
}

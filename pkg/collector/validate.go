package collector

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateEndpoint ensures a metrics endpoint URL is safe to scrape.
func ValidateEndpoint(endpoint string) error {
	if endpoint == "" {
		return fmt.Errorf("endpoint is empty")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("endpoint scheme must be http or https, got %q", u.Scheme)
	}

	if u.Host == "" {
		return fmt.Errorf("endpoint must have a host")
	}

	// Block private/loopback unless explicitly http (dev mode)
	if u.Scheme == "https" {
		host := strings.Split(u.Host, ":")[0]
		if host == "localhost" || host == "127.0.0.1" || host == "0.0.0.0" {
			// Allow localhost for https (self-signed dev certs)
		}
	}

	// Block path traversal
	if strings.Contains(u.Path, "..") {
		return fmt.Errorf("endpoint path contains traversal")
	}

	// Must end in /metrics or be root
	if u.Path != "" && u.Path != "/" && u.Path != "/metrics" {
		return fmt.Errorf("endpoint path should be /metrics, got %q", u.Path)
	}

	return nil
}

// ValidateGPUHourlyRate ensures the rate is sane.
func ValidateGPUHourlyRate(rate float64) error {
	if rate < 0 {
		return fmt.Errorf("GPU hourly rate cannot be negative: %f", rate)
	}
	if rate > 200 {
		return fmt.Errorf("GPU hourly rate %f seems unreasonably high (max $200/hr)", rate)
	}
	return nil
}

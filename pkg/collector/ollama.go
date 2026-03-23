package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaCollector scrapes the Ollama API for model and usage information.
// Ollama doesn't expose Prometheus metrics, so we poll its REST API instead.
type OllamaCollector struct {
	endpoint  string
	client    *http.Client
	gpuHourly float64
}

// OllamaMetrics holds parsed metrics from Ollama API calls.
type OllamaMetrics struct {
	// Models currently loaded
	RunningModels []OllamaRunningModel

	// Aggregate stats
	TotalModelsLoaded int
	TotalVRAMUsedBytes int64
	TotalSizeBytes     int64
}

// OllamaRunningModel represents a model currently loaded in Ollama.
type OllamaRunningModel struct {
	Name       string
	SizeBytes  int64
	VRAMBytes  int64
	ExpiresAt  time.Time
	SizeVRAM   int64
}

// ollamaProcessResponse is the JSON shape returned by GET /api/ps
type ollamaProcessResponse struct {
	Models []struct {
		Name      string `json:"name"`
		Model     string `json:"model"`
		Size      int64  `json:"size"`
		SizeVRAM  int64  `json:"size_vram"`
		ExpiresAt string `json:"expires_at"`
	} `json:"models"`
}

// ollamaTagsResponse is the JSON shape returned by GET /api/tags
type ollamaTagsResponse struct {
	Models []struct {
		Name       string `json:"name"`
		Model      string `json:"model"`
		Size       int64  `json:"size"`
		ModifiedAt string `json:"modified_at"`
	} `json:"models"`
}

// NewOllamaCollector creates a collector for the Ollama REST API.
func NewOllamaCollector(endpoint string, gpuHourlyRate float64) *OllamaCollector {
	return &OllamaCollector{
		endpoint:  endpoint,
		gpuHourly: gpuHourlyRate,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Scrape fetches running models from Ollama's /api/ps endpoint.
func (c *OllamaCollector) Scrape() (*OllamaMetrics, error) {
	m := &OllamaMetrics{}

	// Get running models
	resp, err := c.client.Get(c.endpoint + "/api/ps")
	if err != nil {
		return nil, fmt.Errorf("fetching /api/ps: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("/api/ps returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading /api/ps: %w", err)
	}

	var psResp ollamaProcessResponse
	if err := json.Unmarshal(body, &psResp); err != nil {
		return nil, fmt.Errorf("parsing /api/ps: %w", err)
	}

	for _, model := range psResp.Models {
		expires, _ := time.Parse(time.RFC3339, model.ExpiresAt)
		m.RunningModels = append(m.RunningModels, OllamaRunningModel{
			Name:      model.Name,
			SizeBytes: model.Size,
			VRAMBytes: model.SizeVRAM,
			ExpiresAt: expires,
			SizeVRAM:  model.SizeVRAM,
		})
		m.TotalVRAMUsedBytes += model.SizeVRAM
		m.TotalSizeBytes += model.Size
	}

	m.TotalModelsLoaded = len(psResp.Models)

	return m, nil
}

// ParseProcessResponse parses an Ollama /api/ps JSON response for testing.
func (c *OllamaCollector) ParseProcessResponse(data []byte) (*OllamaMetrics, error) {
	var psResp ollamaProcessResponse
	if err := json.Unmarshal(data, &psResp); err != nil {
		return nil, err
	}

	m := &OllamaMetrics{}
	for _, model := range psResp.Models {
		expires, _ := time.Parse(time.RFC3339, model.ExpiresAt)
		m.RunningModels = append(m.RunningModels, OllamaRunningModel{
			Name:      model.Name,
			SizeBytes: model.Size,
			VRAMBytes: model.SizeVRAM,
			ExpiresAt: expires,
			SizeVRAM:  model.SizeVRAM,
		})
		m.TotalVRAMUsedBytes += model.SizeVRAM
		m.TotalSizeBytes += model.Size
	}
	m.TotalModelsLoaded = len(psResp.Models)

	return m, nil
}

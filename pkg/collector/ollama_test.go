package collector

import (
	"testing"
)

const sampleOllamaPS = `{
  "models": [
    {
      "name": "qwen3:8b",
      "model": "qwen3:8b",
      "size": 4920000000,
      "size_vram": 4920000000,
      "expires_at": "2026-03-23T12:00:00Z"
    },
    {
      "name": "nomic-embed-text:latest",
      "model": "nomic-embed-text:latest",
      "size": 274000000,
      "size_vram": 274000000,
      "expires_at": "2026-03-23T12:05:00Z"
    }
  ]
}`

const sampleOllamaEmpty = `{"models": []}`

func TestOllamaCollector_ParseProcessResponse(t *testing.T) {
	c := NewOllamaCollector("http://fake:11434", 0.80)

	m, err := c.ParseProcessResponse([]byte(sampleOllamaPS))
	if err != nil {
		t.Fatal(err)
	}

	if m.TotalModelsLoaded != 2 {
		t.Errorf("expected 2 models loaded, got %d", m.TotalModelsLoaded)
	}

	if len(m.RunningModels) != 2 {
		t.Fatalf("expected 2 running models, got %d", len(m.RunningModels))
	}

	if m.RunningModels[0].Name != "qwen3:8b" {
		t.Errorf("expected qwen3:8b, got %s", m.RunningModels[0].Name)
	}

	if m.RunningModels[0].VRAMBytes != 4920000000 {
		t.Errorf("expected 4.92GB VRAM, got %d", m.RunningModels[0].VRAMBytes)
	}

	if m.RunningModels[1].Name != "nomic-embed-text:latest" {
		t.Errorf("expected nomic-embed-text, got %s", m.RunningModels[1].Name)
	}
}

func TestOllamaCollector_TotalVRAM(t *testing.T) {
	c := NewOllamaCollector("http://fake:11434", 0.80)

	m, err := c.ParseProcessResponse([]byte(sampleOllamaPS))
	if err != nil {
		t.Fatal(err)
	}

	expectedVRAM := int64(4920000000 + 274000000)
	if m.TotalVRAMUsedBytes != expectedVRAM {
		t.Errorf("expected total VRAM %d, got %d", expectedVRAM, m.TotalVRAMUsedBytes)
	}

	expectedSize := int64(4920000000 + 274000000)
	if m.TotalSizeBytes != expectedSize {
		t.Errorf("expected total size %d, got %d", expectedSize, m.TotalSizeBytes)
	}
}

func TestOllamaCollector_EmptyResponse(t *testing.T) {
	c := NewOllamaCollector("http://fake:11434", 0.80)

	m, err := c.ParseProcessResponse([]byte(sampleOllamaEmpty))
	if err != nil {
		t.Fatal(err)
	}

	if m.TotalModelsLoaded != 0 {
		t.Errorf("expected 0 models, got %d", m.TotalModelsLoaded)
	}
	if len(m.RunningModels) != 0 {
		t.Errorf("expected empty running models, got %d", len(m.RunningModels))
	}
}

func TestOllamaCollector_InvalidJSON(t *testing.T) {
	c := NewOllamaCollector("http://fake:11434", 0.80)

	_, err := c.ParseProcessResponse([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestOllamaCollector_ExpiresAtParsing(t *testing.T) {
	c := NewOllamaCollector("http://fake:11434", 0.80)

	m, err := c.ParseProcessResponse([]byte(sampleOllamaPS))
	if err != nil {
		t.Fatal(err)
	}

	if m.RunningModels[0].ExpiresAt.IsZero() {
		t.Error("expected non-zero expires_at")
	}
	if m.RunningModels[0].ExpiresAt.Year() != 2026 {
		t.Errorf("expected year 2026, got %d", m.RunningModels[0].ExpiresAt.Year())
	}
}

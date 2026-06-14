package ingestion_test

import (
	"strings"
	"testing"

	"github.com/balpal4495/loar/internal/ingestion"
)

// parse is exported via a thin test-helper wrapper to avoid exporting it from
// the production package. We test the Ingestor via IngestReader with a
// mock store.

func TestIngestReaderNDJSON(t *testing.T) {
	ndjson := `{"content":"Romano reports transfer."}
{"content":"Player X signs."}
`
	ms := newMockStore()
	ing := ingestion.New(ms, "proj-1")
	count, err := ing.IngestReader(t.Context(), strings.NewReader(ndjson), "test")
	if err != nil {
		t.Fatalf("IngestReader NDJSON: %v", err)
	}
	if count != 2 {
		t.Errorf("count: want 2, got %d", count)
	}
	if len(ms.observations) != 2 {
		t.Errorf("stored observations: want 2, got %d", len(ms.observations))
	}
	if ms.observations[0].Content != "Romano reports transfer." {
		t.Errorf("content: want 'Romano reports transfer.', got %q", ms.observations[0].Content)
	}
}

func TestIngestReaderJSONArray(t *testing.T) {
	arr := `[{"text":"First item"},{"text":"Second item"}]`
	ms := newMockStore()
	ing := ingestion.New(ms, "proj-1")
	count, err := ing.IngestReader(t.Context(), strings.NewReader(arr), "test")
	if err != nil {
		t.Fatalf("IngestReader JSON array: %v", err)
	}
	if count != 2 {
		t.Errorf("count: want 2, got %d", count)
	}
}

func TestIngestReaderNoContentField(t *testing.T) {
	// Records without a known content field should fall back to raw JSON.
	ndjson := `{"foo":"bar","baz":1}`
	ms := newMockStore()
	ing := ingestion.New(ms, "proj-1")
	count, err := ing.IngestReader(t.Context(), strings.NewReader(ndjson), "test")
	if err != nil {
		t.Fatalf("IngestReader fallback: %v", err)
	}
	if count != 1 {
		t.Errorf("count: want 1, got %d", count)
	}
	if ms.observations[0].Content == "" {
		t.Error("Content should not be empty when falling back to raw JSON")
	}
}

func TestIngestReaderEmpty(t *testing.T) {
	ms := newMockStore()
	ing := ingestion.New(ms, "proj-1")
	count, err := ing.IngestReader(t.Context(), strings.NewReader(""), "test")
	if err != nil {
		t.Fatalf("IngestReader empty: %v", err)
	}
	if count != 0 {
		t.Errorf("count: want 0, got %d", count)
	}
}

func TestIngestReaderSetsProjectID(t *testing.T) {
	ndjson := `{"content":"hello"}`
	ms := newMockStore()
	ing := ingestion.New(ms, "my-project-id")
	if _, err := ing.IngestReader(t.Context(), strings.NewReader(ndjson), "test"); err != nil {
		t.Fatal(err)
	}
	if ms.observations[0].ProjectID != "my-project-id" {
		t.Errorf("ProjectID: want my-project-id, got %s", ms.observations[0].ProjectID)
	}
}

func TestIngestReaderSetsObservedAt(t *testing.T) {
	ndjson := `{"content":"hello"}`
	ms := newMockStore()
	ing := ingestion.New(ms, "proj-1")
	if _, err := ing.IngestReader(t.Context(), strings.NewReader(ndjson), "test"); err != nil {
		t.Fatal(err)
	}
	if ms.observations[0].Temporal.ObservedAt == nil {
		t.Error("ObservedAt should be set during ingestion")
	}
}

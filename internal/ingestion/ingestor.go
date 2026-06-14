// Package ingestion handles importing data into Loar from files, streams,
// and URLs.
//
// Each input record is normalised and stored as an Observation.
// Supported formats: NDJSON (one JSON object per line) and JSON arrays.
package ingestion

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/balpal4495/loar/internal/domain"
	"github.com/balpal4495/loar/internal/store"
)

// Ingestor reads records from various sources and stores them as observations.
type Ingestor struct {
	store     store.Store
	projectID string
}

// New creates a new Ingestor for the given project.
func New(s store.Store, projectID string) *Ingestor {
	return &Ingestor{store: s, projectID: projectID}
}

// IngestFile reads a file from disk and ingests its contents.
// Supported formats: NDJSON, JSON array.
func (ing *Ingestor) IngestFile(ctx context.Context, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("ingestion: open file %q: %w", path, err)
	}
	defer f.Close()
	return ing.IngestReader(ctx, f, path)
}

// IngestURL downloads content from the given URL and ingests it.
func (ing *Ingestor) IngestURL(ctx context.Context, rawURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, fmt.Errorf("ingestion: build request %q: %w", rawURL, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("ingestion: fetch %q: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("ingestion: fetch %q: HTTP %d", rawURL, resp.StatusCode)
	}
	return ing.IngestReader(ctx, resp.Body, rawURL)
}

// IngestReader reads records from r and stores each as an observation.
// source is stored in the observation's SourceID for traceability.
func (ing *Ingestor) IngestReader(ctx context.Context, r io.Reader, source string) (int, error) {
	records, err := parse(r)
	if err != nil {
		return 0, fmt.Errorf("ingestion: parse %q: %w", source, err)
	}

	now := time.Now().UTC()
	count := 0
	for _, rec := range records {
		content := extractContent(rec)
		if content == "" {
			content = rawJSON(rec)
		}

		obs := &domain.Observation{
			ProjectID: ing.projectID,
			Content:   content,
			SourceID:  source,
			Temporal: domain.Temporal{
				ObservedAt: &now,
			},
			Metadata: rec,
		}

		if err := ing.store.CreateObservation(ctx, obs); err != nil {
			return count, fmt.Errorf("ingestion: store observation: %w", err)
		}
		count++
	}
	return count, nil
}

// parse reads r and returns a slice of JSON objects.
// It handles both NDJSON (one object per line) and JSON arrays.
func parse(r io.Reader) ([]map[string]any, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(data))

	if strings.HasPrefix(trimmed, "[") {
		var arr []map[string]any
		if err := json.Unmarshal([]byte(trimmed), &arr); err != nil {
			return nil, err
		}
		return arr, nil
	}

	var records []map[string]any
	scanner := bufio.NewScanner(strings.NewReader(trimmed))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			return nil, err
		}
		records = append(records, obj)
	}
	return records, scanner.Err()
}

// extractContent tries common content fields from a record.
func extractContent(rec map[string]any) string {
	for _, key := range []string{"content", "text", "body", "message", "description", "title"} {
		if v, ok := rec[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// rawJSON serialises the record back to a compact JSON string for use as
// content when no known content field is found.
func rawJSON(rec map[string]any) string {
	b, _ := json.Marshal(rec)
	return string(b)
}

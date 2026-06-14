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
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/balpal4495/loar/internal/domain"
	"github.com/balpal4495/loar/internal/entity"
	"github.com/balpal4495/loar/internal/store"
)

// Ingestor reads records from various sources and stores them as observations.
type Ingestor struct {
	store       store.Store
	projectID   string
	Incremental bool // when true, records whose source_id already exists are skipped
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

// IngestDir ingests all supported files in dir.
// When recursive is true, subdirectories are walked depth-first.
// Supported file extensions: .json, .ndjson, .jsonl
func (ing *Ingestor) IngestDir(ctx context.Context, dir string, recursive bool) (int, []error) {
	var total int
	var errs []error

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, fmt.Errorf("skipped %q: %w", path, err))
			return nil
		}
		if d.IsDir() {
			if path != dir && !recursive {
				return fs.SkipDir
			}
			return nil
		}
		if !isSupportedExt(path) {
			return nil
		}
		n, err := ing.IngestFile(ctx, path)
		if err != nil {
			errs = append(errs, fmt.Errorf("skipped %q: %w", path, err))
			return nil
		}
		total += n
		return nil
	}

	if err := filepath.WalkDir(dir, walkFn); err != nil {
		errs = append(errs, fmt.Errorf("walk dir %q: %w", dir, err))
	}
	return total, errs
}

// isSupportedExt reports whether the file extension is a supported ingest format.
func isSupportedExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json", ".ndjson", ".jsonl":
		return true
	}
	return false
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
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, fmt.Errorf("ingestion: read %q: %w", source, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return 0, nil // empty file — skip silently
	}

	records, err := parseBytes(data)
	if err != nil {
		return 0, fmt.Errorf("ingestion: parse %q: %w", source, err)
	}

	now := time.Now().UTC()
	count := 0
	for _, rec := range records {
		content := extractContent(rec)
		if content == "" {
			content = buildContent(rec)
		}

		// Use the record's own "id" field as source_id when present —
		// this preserves chronicle entry UUIDs rather than the filename.
		sourceID := source
		if idVal, ok := rec["id"]; ok {
			if idStr, ok := idVal.(string); ok && idStr != "" {
				sourceID = idStr
			}
		}

		// Incremental mode: skip records already present in the store.
		if ing.Incremental {
			exists, err := ing.store.ExistsObservationBySourceID(ctx, ing.projectID, sourceID)
			if err == nil && exists {
				continue
			}
		}

		obs := &domain.Observation{
			ProjectID: ing.projectID,
			Content:   content,
			SourceID:  sourceID,
			Temporal: domain.Temporal{
				ObservedAt: &now,
				OccurredAt: extractTime(rec),
			},
			Metadata: rec,
		}

		if err := ing.store.CreateObservation(ctx, obs); err != nil {
			return count, fmt.Errorf("ingestion: store observation: %w", err)
		}
		// Extract entities from content and link them to this observation.
		_ = entity.ExtractAndLink(ctx, ing.store, ing.projectID, obs)
		count++
	}
	return count, nil
}

// parseBytes parses b as either a JSON array or NDJSON.
// If initial parsing fails, it attempts automatic repair before giving up.
func parseBytes(data []byte) ([]map[string]any, error) {
	records, err := doParse(data)
	if err == nil {
		return records, nil
	}
	// Attempt repair and retry.
	repaired := repairJSON(data)
	records, repairErr := doParse(repaired)
	if repairErr == nil {
		return records, nil
	}
	// Return the original error — repair didn't help.
	return nil, err
}

// doParse is the raw parse attempt with no repair logic.
// It handles three formats:
//  1. JSON array  [ {...}, {...} ]
//  2. Single JSON object  { ... }  (including multi-line)
//  3. NDJSON — one complete JSON object per line
func doParse(data []byte) ([]map[string]any, error) {
	trimmed := strings.TrimSpace(string(data))

	// JSON array.
	if strings.HasPrefix(trimmed, "[") {
		var arr []map[string]any
		if err := json.Unmarshal([]byte(trimmed), &arr); err != nil {
			return nil, err
		}
		return arr, nil
	}

	// Single JSON object (catches multi-line objects like chronicle files).
	if strings.HasPrefix(trimmed, "{") {
		var obj map[string]any
		if err := json.Unmarshal([]byte(trimmed), &obj); err == nil {
			return []map[string]any{obj}, nil
		}
		// Falls through to NDJSON attempt, then repair.
	}

	// NDJSON — one complete object per line.
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

// trailingCommaRe matches a comma immediately before a closing } or ].
var trailingCommaRe = regexp.MustCompile(`,\s*([}\]])`)

// repairJSON attempts to fix common JSON corruption:
//   - Trailing commas before } or ]
//   - Truncated input (missing closing braces/brackets)
func repairJSON(data []byte) []byte {
	s := strings.TrimSpace(string(data))

	// Fix trailing commas.
	s = trailingCommaRe.ReplaceAllString(s, "$1")

	// Close any unclosed braces/brackets caused by truncation.
	s = closeOpenStructures(s)

	return []byte(s)
}

// closeOpenStructures scans s and appends any missing closing } or ] to
// repair truncated JSON.
func closeOpenStructures(s string) string {
	var stack []byte
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch c {
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	// Append closers in reverse order.
	for i := len(stack) - 1; i >= 0; i-- {
		s += string(stack[i])
	}
	return s
}

// extractContent tries a short list of conventional "primary content" field
// names. Returns empty string if none match.
func extractContent(rec map[string]any) string {
	for _, key := range []string{"content", "text", "body", "message"} {
		if v, ok := rec[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// buildContent constructs a human-readable summary from all string-valued
// top-level fields, formatted as "key: value" lines. This ensures any JSON
// structure — regardless of field names — produces searchable content rather
// than an opaque JSON blob.
func buildContent(rec map[string]any) string {
	// Ordered priority fields rendered first when present.
	priority := []string{
		"key_insight", "insight",
		"decision",
		"outcome",
		"design",
		"description", "title", "summary", "name",
	}

	seen := make(map[string]bool)
	seenValues := make(map[string]bool)
	var parts []string

	appendField := func(k string, v any) {
		if seen[k] {
			return
		}
		seen[k] = true
		switch val := v.(type) {
		case string:
			if val != "" && !seenValues[val] {
				seenValues[val] = true
				parts = append(parts, k+": "+val)
			}
		case []any:
			var items []string
			for _, item := range val {
				if s, ok := item.(string); ok && s != "" {
					items = append(items, s)
				}
			}
			if len(items) > 0 {
				joined := strings.Join(items, ", ")
				if !seenValues[joined] {
					seenValues[joined] = true
					parts = append(parts, k+": "+joined)
				}
			}
		}
	}

	for _, k := range priority {
		if v, ok := rec[k]; ok {
			appendField(k, v)
		}
	}
	for k, v := range rec {
		appendField(k, v)
	}

	return strings.Join(parts, "\n")
}

// extractTime looks for common timestamp field names and returns a parsed
// *time.Time, or nil if none is found.
func extractTime(rec map[string]any) *time.Time {
	for _, key := range []string{"timestamp", "occurred_at", "occurred", "date", "created_at", "time"} {
		v, ok := rec[key]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02", "2006-01-02T15:04:05"} {
			if t, err := time.Parse(layout, s); err == nil {
				t = t.UTC()
				return &t
			}
		}
	}
	return nil
}

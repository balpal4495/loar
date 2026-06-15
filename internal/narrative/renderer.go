// Package narrative provides NLG-based rendering of Loar context packages
// via an embedded Node.js script. If Node is not available the caller should
// degrade gracefully.
package narrative

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/balpal4495/loar/internal/domain"
)

//go:embed loar-narrative.js
var script []byte

// ErrNodeNotFound is returned when the node binary cannot be found on PATH.
var ErrNodeNotFound = errors.New("node not found on PATH — install Node.js for enhanced narrative output")

// Render serialises pkg as JSON and pipes it through the embedded Node.js
// narrative script, returning the generated prose.
//
// Returns ErrNodeNotFound if node is not available so the caller can degrade
// gracefully. Any other error indicates a script failure.
func Render(pkg *domain.ContextPackage, obs []domain.Observation) (string, error) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return "", ErrNodeNotFound
	}

	// Write the embedded script to a temp file.
	tmp, err := os.CreateTemp("", "loar-narrative-*.js")
	if err != nil {
		return "", fmt.Errorf("narrative: create temp script: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(script); err != nil {
		tmp.Close()
		return "", fmt.Errorf("narrative: write temp script: %w", err)
	}
	tmp.Close()

	// Build the JSON input (same shape as loar explain output).
	input, err := buildInput(pkg, obs)
	if err != nil {
		return "", fmt.Errorf("narrative: marshal input: %w", err)
	}

	cmd := exec.Command(nodePath, tmp.Name())
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("narrative: script error: %s", stderr.String())
	}
	return stdout.String(), nil
}

// buildInput serialises the context package into the JSON shape expected by
// loar-narrative.js — identical to the loar explain output schema.
func buildInput(pkg *domain.ContextPackage, obs []domain.Observation) ([]byte, error) {
	type obsJSON struct {
		Content    string  `json:"content"`
		OccurredAt *string `json:"occurred_at,omitempty"`
		SourceID   string  `json:"source_id,omitempty"`
	}
	type entityJSON struct {
		Name          string `json:"name"`
		CanonicalName string `json:"canonical_name"`
		Type          string `json:"type"`
	}
	type contradictionJSON struct {
		Summary string `json:"summary"`
	}
	type dateRangeJSON struct {
		Earliest string `json:"earliest,omitempty"`
		Latest   string `json:"latest,omitempty"`
	}
	type input struct {
		Query          string              `json:"query"`
		Entities       []entityJSON        `json:"entities"`
		Observations   []obsJSON           `json:"observations"`
		Contradictions []contradictionJSON `json:"contradictions,omitempty"`
		Confidence     float64             `json:"confidence"`
		DateRange      dateRangeJSON       `json:"date_range,omitempty"`
	}

	obsOut := make([]obsJSON, 0, len(obs))
	for _, o := range obs {
		var ts *string
		t := obsTime(o)
		if t != nil {
			s := t.Format(time.RFC3339)
			ts = &s
		}
		obsOut = append(obsOut, obsJSON{
			Content:    o.Content,
			OccurredAt: ts,
			SourceID:   o.SourceID,
		})
	}

	entOut := make([]entityJSON, 0, len(pkg.Entities))
	for _, e := range pkg.Entities {
		entOut = append(entOut, entityJSON{Name: e.CanonicalName, CanonicalName: e.CanonicalName, Type: e.Type})
	}

	conOut := make([]contradictionJSON, 0, len(pkg.Contradictions))
	for _, c := range pkg.Contradictions {
		conOut = append(conOut, contradictionJSON{Summary: c.Summary})
	}

	earliest, latest := dateRange(pkg.Timeline)

	return json.Marshal(input{
		Query:          pkg.Query,
		Entities:       entOut,
		Observations:   obsOut,
		Contradictions: conOut,
		Confidence:     pkg.Confidence,
		DateRange:      dateRangeJSON{Earliest: earliest, Latest: latest},
	})
}

func obsTime(o domain.Observation) *time.Time {
	if o.Temporal.OccurredAt != nil {
		return o.Temporal.OccurredAt
	}
	return o.Temporal.ObservedAt
}

func dateRange(events []domain.TimelineEvent) (earliest, latest string) {
	for _, ev := range events {
		var ds string
		if ev.Temporal.OccurredAt != nil {
			ds = ev.Temporal.OccurredAt.Format("2006-01-02")
		} else if ev.Temporal.ObservedAt != nil {
			ds = ev.Temporal.ObservedAt.Format("2006-01-02")
		}
		if ds == "" {
			continue
		}
		if earliest == "" || ds < earliest {
			earliest = ds
		}
		if ds > latest {
			latest = ds
		}
	}
	return
}

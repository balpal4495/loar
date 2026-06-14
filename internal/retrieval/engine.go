// Package retrieval implements Loar's retrieval engine.
//
// Retrieval pipeline:
//
//	Question
//	↓ Intent Detection
//	↓ Entity Resolution
//	↓ Relationship Traversal
//	↓ Evidence Gathering
//	↓ Context Package
package retrieval

import (
	"context"
	"fmt"
	"strings"

	"github.com/balpal4495/loar/internal/domain"
	"github.com/balpal4495/loar/internal/store"
)

// Engine executes retrieval queries against the store and produces
// ContextPackages.
type Engine struct {
	store store.Store
}

// New creates a new Engine backed by the given store.
func New(s store.Store) *Engine {
	return &Engine{store: s}
}

// Query runs the full retrieval pipeline for the given natural-language query
// within the named project.
func (e *Engine) Query(ctx context.Context, projectID, query string) (*domain.ContextPackage, error) {
	intent := DetectIntent(query)

	entities, err := e.resolveEntities(ctx, projectID, query)
	if err != nil {
		return nil, fmt.Errorf("retrieval: entity resolution: %w", err)
	}

	observations, err := e.gatherObservations(ctx, projectID, query, entities)
	if err != nil {
		return nil, fmt.Errorf("retrieval: evidence gathering: %w", err)
	}

	relationships, err := e.traverseRelationships(ctx, projectID, entities)
	if err != nil {
		return nil, fmt.Errorf("retrieval: relationship traversal: %w", err)
	}

	pkg := &domain.ContextPackage{
		Query:         query,
		Entities:      entities,
		Observations:  observations,
		Relationships: relationships,
		Confidence:    computeConfidence(observations),
	}

	pkg.Evidence = buildEvidence(observations)
	pkg.Contradictions = findContradictions(observations)
	pkg.Timeline = buildTimeline(observations)
	pkg.Summary = buildSummary(intent, query, entities, observations)

	return pkg, nil
}

// resolveEntities extracts candidate entity names from the query and looks
// them up in the store using exact, alias, and case-insensitive matching.
func (e *Engine) resolveEntities(ctx context.Context, projectID, query string) ([]domain.Entity, error) {
	words := tokenise(query)

	seen := map[string]bool{}
	var entities []domain.Entity

	for _, candidate := range words {
		if len(candidate) < 2 {
			continue
		}
		ent, err := e.store.FindEntityByName(ctx, projectID, candidate)
		if err != nil {
			continue
		}
		if seen[ent.ID] {
			continue
		}
		seen[ent.ID] = true
		entities = append(entities, *ent)
	}
	return entities, nil
}

// gatherObservations collects relevant observations via keyword search and,
// where entities were resolved, via entity links.
func (e *Engine) gatherObservations(ctx context.Context, projectID, query string, entities []domain.Entity) ([]domain.Observation, error) {
	seen := map[string]bool{}
	var obs []domain.Observation

	keywordResults, err := e.store.SearchObservations(ctx, projectID, query)
	if err != nil {
		return nil, err
	}
	for _, o := range keywordResults {
		if !seen[o.ID] {
			seen[o.ID] = true
			obs = append(obs, *o)
		}
	}

	for _, ent := range entities {
		entityObs, err := e.store.ObservationsForEntity(ctx, ent.ID)
		if err != nil {
			continue
		}
		for _, o := range entityObs {
			if !seen[o.ID] {
				seen[o.ID] = true
				obs = append(obs, *o)
			}
		}
	}

	return obs, nil
}

// traverseRelationships collects all relationships involving the resolved entities.
func (e *Engine) traverseRelationships(ctx context.Context, projectID string, entities []domain.Entity) ([]domain.Relationship, error) {
	seen := map[string]bool{}
	var rels []domain.Relationship

	for _, ent := range entities {
		entRels, err := e.store.FindRelationships(ctx, projectID, ent.ID)
		if err != nil {
			continue
		}
		for _, r := range entRels {
			if !seen[r.ID] {
				seen[r.ID] = true
				rels = append(rels, *r)
			}
		}
	}
	return rels, nil
}

// tokenise splits a query into candidate words, trimming punctuation.
func tokenise(query string) []string {
	replacer := strings.NewReplacer(
		"?", " ", "!", " ", ".", " ", ",", " ", "\"", " ",
		"'", " ", ";", " ", ":", " ",
	)
	cleaned := replacer.Replace(query)
	return strings.Fields(cleaned)
}

// computeConfidence returns a basic confidence score based on evidence volume.
func computeConfidence(obs []domain.Observation) float64 {
	switch {
	case len(obs) == 0:
		return 0
	case len(obs) < 3:
		return 0.4
	case len(obs) < 10:
		return 0.7
	default:
		return 0.9
	}
}

// buildEvidence converts observations into Evidence summaries.
func buildEvidence(obs []domain.Observation) []domain.Evidence {
	ev := make([]domain.Evidence, 0, len(obs))
	for _, o := range obs {
		summary := truncate(o.Content, 120)
		ev = append(ev, domain.Evidence{
			ObservationID: o.ID,
			Summary:       summary,
			Confidence:    1.0,
		})
	}
	return ev
}

// findContradictions performs a naive contradiction scan: observations whose
// content contains "not" or "never" alongside observations that do not.
// A full contradiction analysis would require semantic reasoning.
func findContradictions(obs []domain.Observation) []domain.Contradiction {
	var positive, negative []domain.Observation
	for _, o := range obs {
		lower := strings.ToLower(o.Content)
		if strings.Contains(lower, " not ") || strings.Contains(lower, " never ") {
			negative = append(negative, o)
		} else {
			positive = append(positive, o)
		}
	}

	var contradictions []domain.Contradiction
	for _, n := range negative {
		for _, p := range positive {
		// Only flag when both observations share a common keyword (>5 chars).
			if shareKeyword(n.Content, p.Content) {
				contradictions = append(contradictions, domain.Contradiction{
					ObservationAID: p.ID,
					ObservationBID: n.ID,
					Summary:        "Potential contradiction detected between observations.",
				})
				break
			}
		}
	}
	return contradictions
}

// shareKeyword reports whether two strings share a word longer than 5 characters.
func shareKeyword(a, b string) bool {
	wordsA := strings.Fields(strings.ToLower(a))
	wordsB := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(b)) {
		if len(w) > 5 {
			wordsB[w] = true
		}
	}
	for _, w := range wordsA {
		if len(w) > 5 && wordsB[w] {
			return true
		}
	}
	return false
}

// buildTimeline converts observations with temporal data into timeline events.
func buildTimeline(obs []domain.Observation) []domain.TimelineEvent {
	var events []domain.TimelineEvent
	for _, o := range obs {
		if o.Temporal.OccurredAt == nil && o.Temporal.ObservedAt == nil {
			continue
		}
		summary := truncate(o.Content, 80)
		events = append(events, domain.TimelineEvent{
			Temporal: o.Temporal,
			Summary:  summary,
		})
	}
	return events
}

// buildSummary constructs a human-readable summary for the context package.
func buildSummary(intent Intent, query string, entities []domain.Entity, obs []domain.Observation) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Intent: %s\n", intent))
	if len(entities) > 0 {
		names := make([]string, 0, len(entities))
		for _, e := range entities {
			names = append(names, e.CanonicalName)
		}
		sb.WriteString(fmt.Sprintf("Entities: %s\n", strings.Join(names, ", ")))
	}
	sb.WriteString(fmt.Sprintf("Evidence: %d observation(s) found.\n", len(obs)))
	if len(obs) == 0 {
		sb.WriteString("No evidence found for this query.")
	}
	return sb.String()
}

// truncate shortens s to at most maxRunes Unicode code points, appending "…"
// when truncation occurs. Using rune conversion ensures multi-byte UTF-8
// characters are not split.
func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

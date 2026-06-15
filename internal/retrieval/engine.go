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
	"math"
	"sort"
	"strings"
	"time"

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

	// 1. Resolve seed entities from the query text.
	seedEntities, err := e.resolveEntities(ctx, projectID, query)
	if err != nil {
		return nil, fmt.Errorf("retrieval: entity resolution: %w", err)
	}

	// 2. Expand the seed set up to 2 hops through the relationship graph.
	//    This surfaces connected entities (e.g. Arsenal → scouts → Romano)
	//    without requiring the query to name them explicitly.
	seedIDs := make([]string, len(seedEntities))
	for i, ent := range seedEntities {
		seedIDs[i] = ent.ID
	}
	expandedEntities, relationships, err := e.store.TraverseFromEntities(ctx, projectID, seedIDs, 2)
	if err != nil {
		return nil, fmt.Errorf("retrieval: graph traversal: %w", err)
	}

	// Merge seeds + expanded into a deduplicated entity set for observation gathering.
	entityMap := make(map[string]domain.Entity, len(seedEntities)+len(expandedEntities))
	for _, ent := range seedEntities {
		entityMap[ent.ID] = ent
	}
	for _, ent := range expandedEntities {
		entityMap[ent.ID] = ent
	}
	allEntities := make([]domain.Entity, 0, len(entityMap))
	for _, ent := range entityMap {
		allEntities = append(allEntities, ent)
	}

	// 3. Gather observations for the full entity set.
	observations, err := e.gatherObservations(ctx, projectID, query, allEntities)
	if err != nil {
		return nil, fmt.Errorf("retrieval: evidence gathering: %w", err)
	}

	// For timeline queries, present observations in chronological order.
	if intent == IntentTimeline {
		sort.Slice(observations, func(i, j int) bool {
			ti := observations[i].Temporal.OccurredAt
			tj := observations[j].Temporal.OccurredAt
			if ti == nil {
				return false
			}
			if tj == nil {
				return true
			}
			return ti.Before(*tj)
		})
	}

	pkg := &domain.ContextPackage{
		Query:         query,
		Entities:      allEntities,
		Observations:  observations,
		Relationships: relationships,
		Confidence:    computeConfidence(observations),
	}

	pkg.Evidence = buildEvidence(observations)
	pkg.Contradictions = findContradictions(observations, allEntities, intent)
	pkg.Timeline = buildTimeline(observations)
	pkg.Summary = buildSummary(intent, query, allEntities, observations)

	return pkg, nil
}

// resolveEntities extracts candidate entity names from the query and looks
// them up in the store using exact, alias, and case-insensitive matching.
// It tries n-grams (up to 3 tokens) first so multi-word names like "Phase 4"
// resolve before their individual tokens.
func (e *Engine) resolveEntities(ctx context.Context, projectID, query string) ([]domain.Entity, error) {
	words := tokenise(query)

	seen := map[string]bool{}
	var entities []domain.Entity

	// Build candidates: trigrams, bigrams, then single tokens (longest first).
	var candidates []string
	for i := range words {
		if i+2 < len(words) {
			candidates = append(candidates, strings.Join(words[i:i+3], " "))
		}
		if i+1 < len(words) {
			candidates = append(candidates, strings.Join(words[i:i+2], " "))
		}
		candidates = append(candidates, words[i])
	}

	for _, candidate := range candidates {
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

// gatherObservations collects relevant observations. When entities are
// resolved, entity-linked observations are returned exclusively (precise mode).
// Only when no entities resolve does it fall back to keyword search.
func (e *Engine) gatherObservations(ctx context.Context, projectID, query string, entities []domain.Entity) ([]domain.Observation, error) {
	seen := map[string]bool{}
	var obs []domain.Observation

	if len(entities) > 0 {
		// Entity-scoped retrieval: only return observations linked to resolved entities.
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
		if len(obs) > 0 {
			return obs, nil
		}
		// Entities resolved but no links yet — fall through to keyword search.
	}

	// Keyword fallback.
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

	return obs, nil
}

// traverseRelationships is kept for any future callers outside Query.
// Query now uses store.TraverseFromEntities directly for multi-hop support.
func (e *Engine) traverseRelationships(ctx context.Context, projectID string, entities []domain.Entity) ([]domain.Relationship, error) {
	if len(entities) == 0 {
		return nil, nil
	}
	ids := make([]string, len(entities))
	for i, ent := range entities {
		ids[i] = ent.ID
	}
	_, rels, err := e.store.TraverseFromEntities(ctx, projectID, ids, 1)
	return rels, err
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

// buildEvidence converts observations into Evidence summaries with
// recency-weighted confidence scores. Recent observations score closer to
// 1.0; older ones decay toward 0.5 over 365 days.
func buildEvidence(obs []domain.Observation) []domain.Evidence {
	now := time.Now()
	ev := make([]domain.Evidence, 0, len(obs))
	for _, o := range obs {
		ev = append(ev, domain.Evidence{
			ObservationID: o.ID,
			Summary:       truncate(o.Content, 120),
			Confidence:    recencyScore(o.Temporal, now),
		})
	}
	return ev
}

// recencyScore returns a confidence value between 0.5 and 1.0 based on how
// recent the observation is. It uses OccurredAt preferentially, falling back
// to ObservedAt. The decay formula is: 0.5 + 0.5 × e^(−days/365).
func recencyScore(t domain.Temporal, now time.Time) float64 {
	ref := t.OccurredAt
	if ref == nil {
		ref = t.ObservedAt
	}
	if ref == nil {
		return 0.5 // no temporal data — neutral score
	}
	days := now.Sub(*ref).Hours() / 24
	if days < 0 {
		return 1.0 // future-dated observation
	}
	return 0.5 + 0.5*math.Exp(-days/365)
}

// findContradictions looks for observations that make opposing claims.
// It requires both observations to mention at least one resolved entity name
// (preventing false positives from incidental shared keywords), caps results
// at 3 for general queries and 5 for causal/evidence queries where surfacing
// tensions is more important, and includes the actual conflicting excerpts.
func findContradictions(obs []domain.Observation, entities []domain.Entity, intent Intent) []domain.Contradiction {
	maxContradictions := 3
	if intent == IntentCausal || intent == IntentEvidence {
		maxContradictions = 5
	}

	// Build a set of entity names to anchor contradiction detection.
	entityNames := make([]string, 0, len(entities))
	for _, e := range entities {
		entityNames = append(entityNames, strings.ToLower(e.CanonicalName))
	}

	mentionsEntity := func(content string) bool {
		if len(entityNames) == 0 {
			return true // no entities resolved — fall back to keyword-only check
		}
		lower := strings.ToLower(content)
		for _, name := range entityNames {
			if strings.Contains(lower, name) {
				return true
			}
		}
		return false
	}

	var contradictions []domain.Contradiction
	for i := 0; i < len(obs) && len(contradictions) < maxContradictions; i++ {
		for j := i + 1; j < len(obs) && len(contradictions) < maxContradictions; j++ {
			a, b := obs[i], obs[j]
			// Both must mention a resolved entity.
			if !mentionsEntity(a.Content) || !mentionsEntity(b.Content) {
				continue
			}
			if !shareKeyword(a.Content, b.Content) {
				continue
			}
			aLower := strings.ToLower(a.Content)
			bLower := strings.ToLower(b.Content)
			aIsNeg := strings.Contains(aLower, " not ") || strings.Contains(aLower, " never ") || strings.Contains(aLower, "cannot")
			bIsNeg := strings.Contains(bLower, " not ") || strings.Contains(bLower, " never ") || strings.Contains(bLower, "cannot")
			if aIsNeg == bIsNeg {
				continue
			}
			contradictions = append(contradictions, domain.Contradiction{
				ObservationAID: a.ID,
				ObservationBID: b.ID,
				Summary: fmt.Sprintf("%s  ↔  %s",
					a.Content,
					b.Content),
			})
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

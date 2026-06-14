// Package entity provides automatic entity extraction from observation content.
//
// Extraction is pattern-based: it identifies named concepts from text without
// requiring an LLM. Extracted entities are upserted into the store and linked
// to the observation they were found in.
package entity

import (
	"context"
	"regexp"
	"strings"

	"github.com/balpal4495/loar/internal/domain"
	"github.com/balpal4495/loar/internal/store"
)

// ExtractAndLink extracts entities from obs.Content, upserts them into the
// store, creates observation_entities links, and writes entity-entity
// co_occurs relationships for every pair of entities found in the same
// observation. It returns the number of observation_entity links created.
// Errors are non-fatal — a failure on one entity does not abort ingestion.
func ExtractAndLink(ctx context.Context, s store.Store, projectID string, obs *domain.Observation) int {
	candidates := Extract(obs.Content)
	linked := 0
	var resolved []string // entity IDs successfully upserted this observation

	for _, c := range candidates {
		// Upsert: find existing entity or create a new one.
		ent, err := s.FindEntityByName(ctx, projectID, c.Name)
		if err != nil {
			// Not found — create it.
			ent = &domain.Entity{
				Type:          c.Type,
				CanonicalName: c.Name,
				Aliases:       []string{},
				Metadata:      map[string]any{},
			}
			if err := s.CreateEntity(ctx, projectID, ent); err != nil {
				continue
			}
			// Re-fetch to get the generated ID.
			ent, err = s.FindEntityByName(ctx, projectID, c.Name)
			if err != nil {
				continue
			}
		}
		resolved = append(resolved, ent.ID)
		// Link entity to observation (ignore duplicate link errors).
		if err := s.Link(ctx, obs.ID, ent.ID, c.Role); err == nil {
			linked++
		}
	}

	// Create pairwise co_occurs relationships between all entities
	// extracted from the same observation. These are idempotent — the store
	// uses ON CONFLICT DO NOTHING so re-ingestion is safe.
	for i := 0; i < len(resolved); i++ {
		for j := i + 1; j < len(resolved); j++ {
			_ = s.CreateRelationship(ctx, projectID, &domain.Relationship{
				SourceID:   resolved[i],
				TargetID:   resolved[j],
				Type:       "co_occurs",
				Confidence: 1.0,
			})
		}
	}

	return linked
}

// Candidate is an entity candidate extracted from text.
type Candidate struct {
	Name string
	Type string
	Role string // role in the observation context
}

// phaseRe matches "Phase N" or "phase N" where N is a digit.
var phaseRe = regexp.MustCompile(`(?i)\bphase\s+(\d+)\b`)

// commitRe matches short commit hashes in square brackets: [1aab2424].
var commitRe = regexp.MustCompile(`\[([0-9a-f]{7,12})\]`)

// filePathRe matches file paths like app/components/Foo.tsx or src/utils/bar.ts.
var filePathRe = regexp.MustCompile(`\b[\w./\-]+\.(tsx?|jsx?|go|py|sql|toml|ya?ml|json)\b`)

// pascalRe matches PascalCase words of 4+ chars (component/type names).
// Excludes all-caps acronyms and common English words.
var pascalRe = regexp.MustCompile(`\b[A-Z][a-z]+(?:[A-Z][a-z]+)+\b`)

// camelHookRe matches React hook names: useFoo, useBar.
var camelHookRe = regexp.MustCompile(`\buse[A-Z][A-Za-z]+\b`)

// Extract returns all entity candidates found in text.
func Extract(text string) []Candidate {
	seen := map[string]bool{}
	var candidates []Candidate

	add := func(name, typ, role string) {
		key := strings.ToLower(name)
		if seen[key] || len(name) < 3 {
			return
		}
		seen[key] = true
		candidates = append(candidates, Candidate{Name: name, Type: typ, Role: role})
	}

	// Phase references: "Phase 4" → entity type "phase"
	for _, m := range phaseRe.FindAllStringSubmatch(text, -1) {
		add("Phase "+m[1], "phase", "mentioned")
	}

	// Commit hashes: [1aab2424] → entity type "commit"
	for _, m := range commitRe.FindAllStringSubmatch(text, -1) {
		add(m[1], "commit", "referenced")
	}

	// File paths: app/components/PortfolioTable.tsx → entity type "file"
	for _, m := range filePathRe.FindAllString(text, -1) {
		// Use the base filename as the canonical name, keep path as alias context.
		add(m, "file", "affected")
	}

	// PascalCase component/type names: PortfolioTable, DiscoverTable
	for _, m := range pascalRe.FindAllString(text, -1) {
		if !isCommonWord(m) {
			add(m, "component", "mentioned")
		}
	}

	// React hooks: usePortfolio, useEnrichedStocks
	for _, m := range camelHookRe.FindAllString(text, -1) {
		add(m, "hook", "mentioned")
	}

	return candidates
}

// commonWords is a set of PascalCase words that are common English and should
// not be treated as entity names.
var commonWords = map[string]bool{
	"Phase": true, "True": true, "False": true, "None": true,
	"Error": true, "String": true, "Boolean": true, "Number": true,
	"Object": true, "Array": true, "Promise": true, "Function": true,
	"Type": true, "Interface": true, "Class": true,
}

func isCommonWord(s string) bool {
	return commonWords[s]
}

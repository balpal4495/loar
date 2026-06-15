package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/balpal4495/loar/internal/domain"
	"github.com/google/uuid"
)

// CreateProject inserts a new project. If p.ID is empty a new UUID is assigned.
func (db *DB) CreateProject(ctx context.Context, p *domain.Project) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	_, err := db.pool.Exec(ctx,
		`INSERT INTO projects (id, name, description) VALUES ($1, $2, $3)
		 ON CONFLICT (name) DO NOTHING`,
		p.ID, p.Name, p.Description,
	)
	return err
}

// GetProject retrieves a project by ID.
func (db *DB) GetProject(ctx context.Context, id string) (*domain.Project, error) {
	row := db.pool.QueryRow(ctx,
		`SELECT id, name, description FROM projects WHERE id = $1`, id)
	var p domain.Project
	if err := row.Scan(&p.ID, &p.Name, &p.Description); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetProjectByName retrieves a project by its unique name.
func (db *DB) GetProjectByName(ctx context.Context, name string) (*domain.Project, error) {
	row := db.pool.QueryRow(ctx,
		`SELECT id, name, description FROM projects WHERE name = $1`, name)
	var p domain.Project
	if err := row.Scan(&p.ID, &p.Name, &p.Description); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListProjects returns all projects.
func (db *DB) ListProjects(ctx context.Context) ([]*domain.Project, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT id, name, description FROM projects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []*domain.Project
	for rows.Next() {
		var p domain.Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Description); err != nil {
			return nil, err
		}
		projects = append(projects, &p)
	}
	return projects, rows.Err()
}

// CreateEntity inserts a new entity. If e.ID is empty a new UUID is assigned.
func (db *DB) CreateEntity(ctx context.Context, projectID string, e *domain.Entity) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	meta, err := json.Marshal(e.Metadata)
	if err != nil {
		return err
	}
	_, err = db.pool.Exec(ctx,
		`INSERT INTO entities (id, project_id, type, canonical_name, aliases, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		e.ID, projectID, e.Type, e.CanonicalName, e.Aliases, meta,
	)
	if err != nil {
		return err
	}
	// Mirror the entity into the Apache AGE property graph so it is
	// immediately available for Cypher traversal.
	params := fmt.Sprintf(`{"id": %q, "project_id": %q, "canonical_name": %q, "type": %q}`,
		e.ID, projectID, e.CanonicalName, e.Type)
	_, _ = db.pool.Exec(ctx,
		`SELECT * FROM cypher('loar_graph', $$
		   MERGE (n:Entity {id: $id})
		   SET n.project_id = $project_id, n.canonical_name = $canonical_name, n.type = $type
		 $$, $1::agtype) AS (v agtype)`, params)
	return nil
}

// GetEntity retrieves an entity by ID.
func (db *DB) GetEntity(ctx context.Context, id string) (*domain.Entity, error) {
	row := db.pool.QueryRow(ctx,
		`SELECT id, type, canonical_name, aliases, metadata FROM entities WHERE id = $1`, id)
	return scanEntity(row)
}

// FindEntityByName searches for an entity by canonical name or alias within a project.
func (db *DB) FindEntityByName(ctx context.Context, projectID, name string) (*domain.Entity, error) {
	row := db.pool.QueryRow(ctx,
		`SELECT id, type, canonical_name, aliases, metadata
		 FROM entities
		 WHERE project_id = $1
		   AND (canonical_name ILIKE $2 OR $2 = ANY(aliases))
		 LIMIT 1`,
		projectID, name,
	)
	return scanEntity(row)
}

// ListEntities returns all entities for a project.
func (db *DB) ListEntities(ctx context.Context, projectID string) ([]*domain.Entity, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT id, type, canonical_name, aliases, metadata
		 FROM entities WHERE project_id = $1 ORDER BY canonical_name`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entities []*domain.Entity
	for rows.Next() {
		e, err := scanEntityRow(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, e)
	}
	return entities, rows.Err()
}

// CreateObservation inserts a new observation. If o.ID is empty a new UUID is assigned.
func (db *DB) CreateObservation(ctx context.Context, o *domain.Observation) error {
	if o.ID == "" {
		o.ID = uuid.NewString()
	}
	meta, err := json.Marshal(o.Metadata)
	if err != nil {
		return err
	}
	_, err = db.pool.Exec(ctx,
		`INSERT INTO observations
		 (id, project_id, source_id, content, occurred_at, observed_at, resolved_at, learned_at, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		o.ID, o.ProjectID, o.SourceID, o.Content,
		o.Temporal.OccurredAt, o.Temporal.ObservedAt,
		o.Temporal.ResolvedAt, o.Temporal.LearnedAt,
		meta,
	)
	return err
}

// GetObservation retrieves an observation by ID.
func (db *DB) GetObservation(ctx context.Context, id string) (*domain.Observation, error) {
	row := db.pool.QueryRow(ctx,
		`SELECT id, project_id, source_id, content,
		        occurred_at, observed_at, resolved_at, learned_at, metadata
		 FROM observations WHERE id = $1`, id)
	return scanObservation(row)
}

// ExistsObservationBySourceID returns true if the project already has an
// observation with the given source_id. Used to skip records on incremental
// ingest runs.
func (db *DB) ExistsObservationBySourceID(ctx context.Context, projectID, sourceID string) (bool, error) {
	var exists bool
	err := db.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM observations WHERE project_id = $1 AND source_id = $2)`,
		projectID, sourceID,
	).Scan(&exists)
	return exists, err
}

// ListObservations returns all observations for a project ordered by created_at.
func (db *DB) ListObservations(ctx context.Context, projectID string) ([]*domain.Observation, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT id, project_id, source_id, content,
		        occurred_at, observed_at, resolved_at, learned_at, metadata
		 FROM observations WHERE project_id = $1 ORDER BY created_at`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var obs []*domain.Observation
	for rows.Next() {
		o, err := scanObservationRows(rows)
		if err != nil {
			return nil, err
		}
		obs = append(obs, o)
	}
	return obs, rows.Err()
}

// SearchObservations performs full-text search against observation content.
// It splits the query into individual keywords and uses PostgreSQL's
// to_tsvector/to_tsquery for ranked, language-aware matching, falling back
// to ILIKE per-keyword when no FTS results are found.
func (db *DB) SearchObservations(ctx context.Context, projectID, query string) ([]*domain.Observation, error) {
	// Build a tsquery by AND-ing all keywords together.
	keywords := splitKeywords(query)
	if len(keywords) == 0 {
		return nil, nil
	}

	// Try full-text search first.
	tsquery := strings.Join(keywords, " & ")
	rows, err := db.pool.Query(ctx,
		`SELECT id, project_id, source_id, content,
		        occurred_at, observed_at, resolved_at, learned_at, metadata
		 FROM observations
		 WHERE project_id = $1
		   AND to_tsvector('english', content) @@ to_tsquery('english', $2)
		 ORDER BY ts_rank(to_tsvector('english', content), to_tsquery('english', $2)) DESC`,
		projectID, tsquery,
	)
	if err == nil {
		defer rows.Close()
		var obs []*domain.Observation
		for rows.Next() {
			o, err := scanObservationRows(rows)
			if err != nil {
				return nil, err
			}
			obs = append(obs, o)
		}
		if rowErr := rows.Err(); rowErr != nil {
			return nil, rowErr
		}
		if len(obs) > 0 {
			return obs, nil
		}
	}

	// Fallback: OR-based ILIKE across all keywords.
	seen := map[string]bool{}
	var results []*domain.Observation
	for _, kw := range keywords {
		kwRows, err := db.pool.Query(ctx,
			`SELECT id, project_id, source_id, content,
			        occurred_at, observed_at, resolved_at, learned_at, metadata
			 FROM observations
			 WHERE project_id = $1 AND content ILIKE '%' || $2 || '%'
			 ORDER BY created_at`,
			projectID, kw,
		)
		if err != nil {
			continue
		}
		for kwRows.Next() {
			o, err := scanObservationRows(kwRows)
			if err != nil {
				kwRows.Close()
				return nil, err
			}
			if !seen[o.ID] {
				seen[o.ID] = true
				results = append(results, o)
			}
		}
		kwRows.Close()
	}
	return results, nil
}

// splitKeywords tokenises a natural-language query into searchable keywords.
// Stop words and short tokens are removed.
func splitKeywords(query string) []string {
	stopWords := map[string]bool{
		"a": true, "an": true, "the": true, "is": true, "are": true,
		"was": true, "were": true, "what": true, "how": true, "why": true,
		"when": true, "did": true, "do": true, "does": true, "in": true,
		"of": true, "to": true, "for": true, "and": true, "or": true,
		"about": true, "made": true, "been": true, "has": true, "have": true,
	}
	replacer := strings.NewReplacer("?", "", "!", "", ".", "", ",", "", "'", "", "\"", "")
	var keywords []string
	for _, word := range strings.Fields(replacer.Replace(strings.ToLower(query))) {
		if len(word) >= 3 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}
	return keywords
}

// CreateRelationship inserts a new relationship. If r.ID is empty a new UUID is assigned.
// Duplicate (project_id, source_id, target_id, relationship_type) tuples are silently ignored.
func (db *DB) CreateRelationship(ctx context.Context, projectID string, r *domain.Relationship) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	_, err := db.pool.Exec(ctx,
		`INSERT INTO relationships
                 (id, project_id, source_id, target_id, relationship_type, confidence)
                 VALUES ($1, $2, $3, $4, $5, $6)
                 ON CONFLICT (project_id, source_id, target_id, relationship_type) DO NOTHING`,
		r.ID, projectID, r.SourceID, r.TargetID, r.Type, r.Confidence,
	)
	if err != nil {
		return err
	}
	// Mirror the edge into the Apache AGE property graph.
	// MATCH on Entity nodes that were written by CreateEntity; if either
	// endpoint is missing the Cypher is a no-op (returns 0 rows, no error).
	params := fmt.Sprintf(`{"id": %q, "source_id": %q, "target_id": %q, "rel_type": %q, "confidence": %v}`,
		r.ID, r.SourceID, r.TargetID, r.Type, r.Confidence)
	_, _ = db.pool.Exec(ctx,
		`SELECT * FROM cypher('loar_graph', $$
		   MATCH (a:Entity {id: $source_id}), (b:Entity {id: $target_id})
		   MERGE (a)-[r:RELATED {id: $id}]->(b)
		   SET r.type = $rel_type, r.confidence = $confidence
		 $$, $1::agtype) AS (v agtype)`, params)
	return nil
}

// ListRelationships returns all relationships for a project.
func (db *DB) ListRelationships(ctx context.Context, projectID string) ([]*domain.Relationship, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT id, source_id, target_id, relationship_type, confidence
		 FROM relationships WHERE project_id = $1`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rels []*domain.Relationship
	for rows.Next() {
		var r domain.Relationship
		if err := rows.Scan(&r.ID, &r.SourceID, &r.TargetID, &r.Type, &r.Confidence); err != nil {
			return nil, err
		}
		rels = append(rels, &r)
	}
	return rels, rows.Err()
}

// FindRelationships returns relationships where the given entity is either
// source or target.
func (db *DB) FindRelationships(ctx context.Context, projectID, entityID string) ([]*domain.Relationship, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT id, source_id, target_id, relationship_type, confidence
		 FROM relationships
		 WHERE project_id = $1 AND (source_id = $2 OR target_id = $2)`,
		projectID, entityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rels []*domain.Relationship
	for rows.Next() {
		var r domain.Relationship
		if err := rows.Scan(&r.ID, &r.SourceID, &r.TargetID, &r.Type, &r.Confidence); err != nil {
			return nil, err
		}
		rels = append(rels, &r)
	}
	return rels, rows.Err()
}

// Link associates an observation with an entity.
func (db *DB) Link(ctx context.Context, observationID, entityID, role string) error {
	_, err := db.pool.Exec(ctx,
		`INSERT INTO observation_entities (observation_id, entity_id, role)
		 VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		observationID, entityID, role,
	)
	return err
}

// EntitiesForObservation returns all entities linked to an observation.
func (db *DB) EntitiesForObservation(ctx context.Context, observationID string) ([]*domain.Entity, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT e.id, e.type, e.canonical_name, e.aliases, e.metadata
		 FROM entities e
		 JOIN observation_entities oe ON oe.entity_id = e.id
		 WHERE oe.observation_id = $1`,
		observationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entities []*domain.Entity
	for rows.Next() {
		e, err := scanEntityRow(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, e)
	}
	return entities, rows.Err()
}

// ObservationsForEntity returns all observations linked to an entity.
func (db *DB) ObservationsForEntity(ctx context.Context, entityID string) ([]*domain.Observation, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT o.id, o.project_id, o.source_id, o.content,
		        o.occurred_at, o.observed_at, o.resolved_at, o.learned_at, o.metadata
		 FROM observations o
		 JOIN observation_entities oe ON oe.observation_id = o.id
		 WHERE oe.entity_id = $1
		 ORDER BY o.created_at`,
		entityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var obs []*domain.Observation
	for rows.Next() {
		o, err := scanObservationRows(rows)
		if err != nil {
			return nil, err
		}
		obs = append(obs, o)
	}
	return obs, rows.Err()
}

// --- helpers ---

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEntity(row rowScanner) (*domain.Entity, error) {
	var e domain.Entity
	var metaRaw []byte
	if err := row.Scan(&e.ID, &e.Type, &e.CanonicalName, &e.Aliases, &metaRaw); err != nil {
		return nil, err
	}
	if len(metaRaw) > 0 {
		if err := json.Unmarshal(metaRaw, &e.Metadata); err != nil {
			return nil, err
		}
	}
	return &e, nil
}

func scanEntityRow(row rowScanner) (*domain.Entity, error) {
	return scanEntity(row)
}

func scanObservation(row rowScanner) (*domain.Observation, error) {
	var o domain.Observation
	var metaRaw []byte
	var occurredAt, observedAt, resolvedAt, learnedAt *time.Time
	if err := row.Scan(
		&o.ID, &o.ProjectID, &o.SourceID, &o.Content,
		&occurredAt, &observedAt, &resolvedAt, &learnedAt,
		&metaRaw,
	); err != nil {
		return nil, err
	}
	o.Temporal = domain.Temporal{
		OccurredAt: occurredAt,
		ObservedAt: observedAt,
		ResolvedAt: resolvedAt,
		LearnedAt:  learnedAt,
	}
	if len(metaRaw) > 0 {
		if err := json.Unmarshal(metaRaw, &o.Metadata); err != nil {
			return nil, err
		}
	}
	return &o, nil
}

func scanObservationRows(row rowScanner) (*domain.Observation, error) {
	return scanObservation(row)
}

// --- entity_confidence ---

// WriteEntityConfidence inserts a new confidence snapshot for an entity.
// Rows are never updated — the time-series is append-only so trend is queryable.
func (db *DB) WriteEntityConfidence(ctx context.Context, ec *domain.EntityConfidence) error {
	if ec.ID == "" {
		ec.ID = uuid.NewString()
	}
	if ec.ComputedAt.IsZero() {
		ec.ComputedAt = time.Now()
	}
	_, err := db.pool.Exec(ctx,
		`INSERT INTO entity_confidence
		 (id, entity_id, project_id, score, observation_count, resolved_count, accuracy_rate, computed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		ec.ID, ec.EntityID, ec.ProjectID, ec.Score,
		ec.ObservationCount, ec.ResolvedCount, ec.AccuracyRate, ec.ComputedAt,
	)
	return err
}

// LatestEntityConfidence returns the most recent confidence snapshot for an entity.
func (db *DB) LatestEntityConfidence(ctx context.Context, projectID, entityID string) (*domain.EntityConfidence, error) {
	row := db.pool.QueryRow(ctx,
		`SELECT id, entity_id, project_id, score, observation_count, resolved_count, accuracy_rate, computed_at
		 FROM entity_confidence
		 WHERE project_id = $1 AND entity_id = $2
		 ORDER BY computed_at DESC LIMIT 1`,
		projectID, entityID,
	)
	return scanEntityConfidence(row)
}

// EntityConfidenceHistory returns confidence snapshots for an entity in
// reverse chronological order, capped at limit rows.
func (db *DB) EntityConfidenceHistory(ctx context.Context, projectID, entityID string, limit int) ([]*domain.EntityConfidence, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT id, entity_id, project_id, score, observation_count, resolved_count, accuracy_rate, computed_at
		 FROM entity_confidence
		 WHERE project_id = $1 AND entity_id = $2
		 ORDER BY computed_at DESC LIMIT $3`,
		projectID, entityID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*domain.EntityConfidence
	for rows.Next() {
		ec, err := scanEntityConfidence(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, ec)
	}
	return results, rows.Err()
}

func scanEntityConfidence(row rowScanner) (*domain.EntityConfidence, error) {
	var ec domain.EntityConfidence
	var computedAt time.Time
	if err := row.Scan(
		&ec.ID, &ec.EntityID, &ec.ProjectID, &ec.Score,
		&ec.ObservationCount, &ec.ResolvedCount, &ec.AccuracyRate, &computedAt,
	); err != nil {
		return nil, err
	}
	ec.ComputedAt = computedAt
	return &ec, nil
}

// --- graph traversal ---

// TraverseFromEntities expands seed entity IDs up to depth hops using
// Apache AGE Cypher on the loar_graph property graph. The Cypher MATCH
// traversal is more expressive than a recursive CTE and handles arbitrary
// relationship types natively.
//
// Returned entities are the expanded neighbours (depth > 0). The full entity
// records are fetched from the relational store after the graph query so
// callers get rich domain objects, not raw agtype values.
func (db *DB) TraverseFromEntities(ctx context.Context, projectID string, entityIDs []string, depth int) ([]domain.Entity, []domain.Relationship, error) {
	if len(entityIDs) == 0 || depth == 0 {
		return nil, nil, nil
	}

	// Build the agtype params. seed_ids is a Cypher list of quoted strings.
	quoted := make([]string, len(entityIDs))
	for i, id := range entityIDs {
		quoted[i] = fmt.Sprintf("%q", id)
	}
	params := fmt.Sprintf(`{"project_id": %q, "seed_ids": [%s]}`,
		projectID, strings.Join(quoted, ", "))

	// Use UNWIND to iterate seed IDs and perform a variable-length path match.
	// depth is an integer Go variable — fmt.Sprintf is safe here.
	sql := fmt.Sprintf(`
		SELECT * FROM cypher('loar_graph', $$
		    UNWIND $seed_ids AS seed_id
		    MATCH (seed:Entity {id: seed_id, project_id: $project_id})-[*1..%d]-(n:Entity {project_id: $project_id})
		    RETURN DISTINCT n.id
		$$, $1::agtype) AS (entity_id agtype)`, depth)

	rows, err := db.pool.Query(ctx, sql, params)
	if err != nil {
		return nil, nil, fmt.Errorf("traverse AGE: %w", err)
	}
	defer rows.Close()

	var expandedIDs []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, nil, err
		}
		// AGE returns string properties as JSON strings: "abc" — strip the quotes.
		id := strings.Trim(raw, `"`)
		if id != "" && id != "null" {
			expandedIDs = append(expandedIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	if len(expandedIDs) == 0 {
		return nil, nil, nil
	}

	// Fetch full entity records from the relational store.
	entityRows, err := db.pool.Query(ctx,
		`SELECT id, type, canonical_name, aliases, metadata
		 FROM entities
		 WHERE project_id = $1 AND id = ANY($2::text[])`,
		projectID, expandedIDs,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("traverse fetch entities: %w", err)
	}
	defer entityRows.Close()

	var expanded []domain.Entity
	for entityRows.Next() {
		e, err := scanEntityRow(entityRows)
		if err != nil {
			return nil, nil, err
		}
		expanded = append(expanded, *e)
	}
	if err := entityRows.Err(); err != nil {
		return nil, nil, err
	}

	// Collect all relationships in the induced subgraph (seeds ∪ expanded).
	allIDs := append(entityIDs, expandedIDs...)
	relRows, err := db.pool.Query(ctx,
		`SELECT id, source_id, target_id, relationship_type, confidence
		 FROM relationships
		 WHERE project_id = $1
		   AND source_id = ANY($2::text[])
		   AND target_id = ANY($2::text[])`,
		projectID, allIDs,
	)
	if err != nil {
		return expanded, nil, fmt.Errorf("traverse fetch relationships: %w", err)
	}
	defer relRows.Close()

	var rels []domain.Relationship
	for relRows.Next() {
		var r domain.Relationship
		if err := relRows.Scan(&r.ID, &r.SourceID, &r.TargetID, &r.Type, &r.Confidence); err != nil {
			return expanded, nil, err
		}
		rels = append(rels, r)
	}
	return expanded, rels, relRows.Err()
}

// confidenceDecay computes recency-weighted confidence for a set of observations.
// Shared by the Postgres learn path; mirrors retrieval/engine.go recencyScore.
func confidenceDecay(obs []*domain.Observation, now time.Time) float64 {
	if len(obs) == 0 {
		return 0.5
	}
	total := 0.0
	for _, o := range obs {
		ref := o.Temporal.OccurredAt
		if ref == nil {
			ref = o.Temporal.ObservedAt
		}
		if ref == nil {
			total += 0.5
			continue
		}
		days := now.Sub(*ref).Hours() / 24
		if days < 0 {
			days = 0
		}
		total += 0.5 + 0.5*math.Exp(-days/365)
	}
	return total / float64(len(obs))
}

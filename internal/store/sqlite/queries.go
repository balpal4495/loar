package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/balpal4495/loar/internal/domain"
	"github.com/google/uuid"
)

// --- projects ---

func (db *DB) CreateProject(ctx context.Context, p *domain.Project) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	_, err := db.db.ExecContext(ctx,
		`INSERT INTO projects (id, name, description) VALUES (?, ?, ?)
		 ON CONFLICT (name) DO NOTHING`,
		p.ID, p.Name, p.Description,
	)
	return err
}

func (db *DB) GetProject(ctx context.Context, id string) (*domain.Project, error) {
	row := db.db.QueryRowContext(ctx,
		`SELECT id, name, description FROM projects WHERE id = ?`, id)
	var p domain.Project
	if err := row.Scan(&p.ID, &p.Name, &p.Description); err != nil {
		return nil, err
	}
	return &p, nil
}

func (db *DB) GetProjectByName(ctx context.Context, name string) (*domain.Project, error) {
	row := db.db.QueryRowContext(ctx,
		`SELECT id, name, description FROM projects WHERE name = ?`, name)
	var p domain.Project
	if err := row.Scan(&p.ID, &p.Name, &p.Description); err != nil {
		return nil, err
	}
	return &p, nil
}

func (db *DB) ListProjects(ctx context.Context) ([]*domain.Project, error) {
	rows, err := db.db.QueryContext(ctx,
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

// --- entities ---

func (db *DB) CreateEntity(ctx context.Context, projectID string, e *domain.Entity) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	aliasesJSON, err := json.Marshal(e.Aliases)
	if err != nil {
		return err
	}
	metaJSON, err := json.Marshal(e.Metadata)
	if err != nil {
		return err
	}
	_, err = db.db.ExecContext(ctx,
		`INSERT INTO entities (id, project_id, type, canonical_name, aliases, metadata)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		e.ID, projectID, e.Type, e.CanonicalName, string(aliasesJSON), string(metaJSON),
	)
	return err
}

func (db *DB) GetEntity(ctx context.Context, id string) (*domain.Entity, error) {
	row := db.db.QueryRowContext(ctx,
		`SELECT id, type, canonical_name, aliases, metadata FROM entities WHERE id = ?`, id)
	return scanSQLiteEntity(row)
}

// FindEntityByName searches for an entity by canonical name (case-insensitive)
// or alias within a project. Alias lookup requires LIKE scan since aliases are
// stored as a JSON array string.
func (db *DB) FindEntityByName(ctx context.Context, projectID, name string) (*domain.Entity, error) {
	// Try canonical name first (fast).
	row := db.db.QueryRowContext(ctx,
		`SELECT id, type, canonical_name, aliases, metadata
		 FROM entities
		 WHERE project_id = ?
		   AND canonical_name = ? COLLATE NOCASE
		 LIMIT 1`,
		projectID, name,
	)
	e, err := scanSQLiteEntity(row)
	if err == nil {
		return e, nil
	}

	// Alias scan: look for the name inside the JSON array string.
	// JSON arrays store strings as `"value"` so we search for `"name"`.
	pattern := `%"` + strings.ReplaceAll(name, `"`, `\"`) + `"%`
	row = db.db.QueryRowContext(ctx,
		`SELECT id, type, canonical_name, aliases, metadata
		 FROM entities
		 WHERE project_id = ?
		   AND aliases LIKE ?
		 LIMIT 1`,
		projectID, pattern,
	)
	return scanSQLiteEntity(row)
}

func (db *DB) ListEntities(ctx context.Context, projectID string) ([]*domain.Entity, error) {
	rows, err := db.db.QueryContext(ctx,
		`SELECT id, type, canonical_name, aliases, metadata
		 FROM entities WHERE project_id = ? ORDER BY canonical_name`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entities []*domain.Entity
	for rows.Next() {
		e, err := scanSQLiteEntityRow(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, e)
	}
	return entities, rows.Err()
}

// --- observations ---

func (db *DB) CreateObservation(ctx context.Context, o *domain.Observation) error {
	if o.ID == "" {
		o.ID = uuid.NewString()
	}
	metaJSON, err := json.Marshal(o.Metadata)
	if err != nil {
		return err
	}
	_, err = db.db.ExecContext(ctx,
		`INSERT INTO observations
		 (id, project_id, source_id, content, occurred_at, observed_at, resolved_at, learned_at, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.ID, o.ProjectID, o.SourceID, o.Content,
		nullableTime(o.Temporal.OccurredAt),
		nullableTime(o.Temporal.ObservedAt),
		nullableTime(o.Temporal.ResolvedAt),
		nullableTime(o.Temporal.LearnedAt),
		string(metaJSON),
	)
	if err != nil {
		return err
	}
	// Maintain FTS5 index.
	_, err = db.db.ExecContext(ctx,
		`INSERT INTO observations_fts (id, content) VALUES (?, ?)`,
		o.ID, o.Content,
	)
	return err
}

func (db *DB) GetObservation(ctx context.Context, id string) (*domain.Observation, error) {
	row := db.db.QueryRowContext(ctx,
		`SELECT id, project_id, source_id, content,
		        occurred_at, observed_at, resolved_at, learned_at, metadata
		 FROM observations WHERE id = ?`, id)
	return scanSQLiteObservation(row)
}

func (db *DB) ExistsObservationBySourceID(ctx context.Context, projectID, sourceID string) (bool, error) {
	var exists bool
	err := db.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM observations WHERE project_id = ? AND source_id = ?)`,
		projectID, sourceID,
	).Scan(&exists)
	return exists, err
}

func (db *DB) ListObservations(ctx context.Context, projectID string) ([]*domain.Observation, error) {
	rows, err := db.db.QueryContext(ctx,
		`SELECT id, project_id, source_id, content,
		        occurred_at, observed_at, resolved_at, learned_at, metadata
		 FROM observations WHERE project_id = ? ORDER BY created_at`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var obs []*domain.Observation
	for rows.Next() {
		o, err := scanSQLiteObservationRow(rows)
		if err != nil {
			return nil, err
		}
		obs = append(obs, o)
	}
	return obs, rows.Err()
}

// SearchObservations uses FTS5 for ranked full-text search, falling back to
// ILIKE-style GLOB per keyword when FTS returns no results.
func (db *DB) SearchObservations(ctx context.Context, projectID, query string) ([]*domain.Observation, error) {
	keywords := sqliteKeywords(query)
	if len(keywords) == 0 {
		return nil, nil
	}

	// FTS5 match using porter stemmer. Join to observations to filter by project.
	ftsQuery := strings.Join(keywords, " ")
	rows, err := db.db.QueryContext(ctx,
		`SELECT o.id, o.project_id, o.source_id, o.content,
		        o.occurred_at, o.observed_at, o.resolved_at, o.learned_at, o.metadata
		 FROM observations o
		 JOIN observations_fts f ON f.id = o.id
		 WHERE o.project_id = ?
		   AND observations_fts MATCH ?
		 ORDER BY rank`,
		projectID, ftsQuery,
	)
	if err == nil {
		defer rows.Close()
		var obs []*domain.Observation
		for rows.Next() {
			o, err := scanSQLiteObservationRow(rows)
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

	// Keyword fallback using LIKE.
	seen := map[string]bool{}
	var results []*domain.Observation
	for _, kw := range keywords {
		kwRows, err := db.db.QueryContext(ctx,
			`SELECT id, project_id, source_id, content,
			        occurred_at, observed_at, resolved_at, learned_at, metadata
			 FROM observations
			 WHERE project_id = ? AND content LIKE ?
			 ORDER BY created_at`,
			projectID, "%"+kw+"%",
		)
		if err != nil {
			continue
		}
		for kwRows.Next() {
			o, err := scanSQLiteObservationRow(kwRows)
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

// --- relationships ---

func (db *DB) CreateRelationship(ctx context.Context, projectID string, r *domain.Relationship) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	_, err := db.db.ExecContext(ctx,
		`INSERT INTO relationships
		 (id, project_id, source_id, target_id, relationship_type, confidence)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (project_id, source_id, target_id, relationship_type) DO NOTHING`,
		r.ID, projectID, r.SourceID, r.TargetID, r.Type, r.Confidence,
	)
	return err
}

func (db *DB) ListRelationships(ctx context.Context, projectID string) ([]*domain.Relationship, error) {
	rows, err := db.db.QueryContext(ctx,
		`SELECT id, source_id, target_id, relationship_type, confidence
		 FROM relationships WHERE project_id = ?`,
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

func (db *DB) FindRelationships(ctx context.Context, projectID, entityID string) ([]*domain.Relationship, error) {
	rows, err := db.db.QueryContext(ctx,
		`SELECT id, source_id, target_id, relationship_type, confidence
		 FROM relationships
		 WHERE project_id = ? AND (source_id = ? OR target_id = ?)`,
		projectID, entityID, entityID,
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

// --- observation_entities ---

func (db *DB) Link(ctx context.Context, observationID, entityID, role string) error {
	_, err := db.db.ExecContext(ctx,
		`INSERT INTO observation_entities (observation_id, entity_id, role)
		 VALUES (?, ?, ?) ON CONFLICT DO NOTHING`,
		observationID, entityID, role,
	)
	return err
}

func (db *DB) EntitiesForObservation(ctx context.Context, observationID string) ([]*domain.Entity, error) {
	rows, err := db.db.QueryContext(ctx,
		`SELECT e.id, e.type, e.canonical_name, e.aliases, e.metadata
		 FROM entities e
		 JOIN observation_entities oe ON oe.entity_id = e.id
		 WHERE oe.observation_id = ?`,
		observationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entities []*domain.Entity
	for rows.Next() {
		e, err := scanSQLiteEntityRow(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, e)
	}
	return entities, rows.Err()
}

func (db *DB) ObservationsForEntity(ctx context.Context, entityID string) ([]*domain.Observation, error) {
	rows, err := db.db.QueryContext(ctx,
		`SELECT o.id, o.project_id, o.source_id, o.content,
		        o.occurred_at, o.observed_at, o.resolved_at, o.learned_at, o.metadata
		 FROM observations o
		 JOIN observation_entities oe ON oe.observation_id = o.id
		 WHERE oe.entity_id = ?
		 ORDER BY o.created_at`,
		entityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var obs []*domain.Observation
	for rows.Next() {
		o, err := scanSQLiteObservationRow(rows)
		if err != nil {
			return nil, err
		}
		obs = append(obs, o)
	}
	return obs, rows.Err()
}

// --- entity_confidence ---

func (db *DB) WriteEntityConfidence(ctx context.Context, ec *domain.EntityConfidence) error {
	if ec.ID == "" {
		ec.ID = uuid.NewString()
	}
	if ec.ComputedAt.IsZero() {
		ec.ComputedAt = time.Now()
	}
	_, err := db.db.ExecContext(ctx,
		`INSERT INTO entity_confidence
		 (id, entity_id, project_id, score, observation_count, resolved_count, accuracy_rate, computed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ec.ID, ec.EntityID, ec.ProjectID, ec.Score,
		ec.ObservationCount, ec.ResolvedCount, ec.AccuracyRate,
		ec.ComputedAt.UTC().Format(time.RFC3339),
	)
	return err
}

func (db *DB) LatestEntityConfidence(ctx context.Context, projectID, entityID string) (*domain.EntityConfidence, error) {
	row := db.db.QueryRowContext(ctx,
		`SELECT id, entity_id, project_id, score, observation_count, resolved_count, accuracy_rate, computed_at
		 FROM entity_confidence
		 WHERE project_id = ? AND entity_id = ?
		 ORDER BY computed_at DESC LIMIT 1`,
		projectID, entityID,
	)
	return scanSQLiteEntityConfidence(row)
}

func (db *DB) EntityConfidenceHistory(ctx context.Context, projectID, entityID string, limit int) ([]*domain.EntityConfidence, error) {
	rows, err := db.db.QueryContext(ctx,
		`SELECT id, entity_id, project_id, score, observation_count, resolved_count, accuracy_rate, computed_at
		 FROM entity_confidence
		 WHERE project_id = ? AND entity_id = ?
		 ORDER BY computed_at DESC LIMIT ?`,
		projectID, entityID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*domain.EntityConfidence
	for rows.Next() {
		ec, err := scanSQLiteEntityConfidenceRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, ec)
	}
	return results, rows.Err()
}

// --- graph traversal ---

// TraverseFromEntities expands seed entity IDs up to depth hops using a
// cycle-safe recursive CTE. The cycle prevention tracks visited IDs as a
// comma-delimited string (UUIDs never contain commas).
//
// This is the SQLite implementation. The Kuzu implementation would use Cypher:
//
//	MATCH (seed:Entity)-[*1..depth]-(n:Entity)
//	WHERE seed.id IN $seeds AND seed.project_id = $projectID
//	RETURN DISTINCT n
func (db *DB) TraverseFromEntities(ctx context.Context, projectID string, entityIDs []string, depth int) ([]domain.Entity, []domain.Relationship, error) {
	if len(entityIDs) == 0 || depth == 0 {
		return nil, nil, nil
	}

	// Pass seed IDs as a JSON array for json_each().
	seedsJSON, err := json.Marshal(entityIDs)
	if err != nil {
		return nil, nil, err
	}

	// Recursive CTE: visited is a comma-delimited string of seen entity IDs.
	// The NOT LIKE check prevents revisiting nodes (cycle safety).
	rows, err := db.db.QueryContext(ctx,
		`WITH RECURSIVE traversal(entity_id, depth, visited) AS (
		     SELECT e.id, 0, ',' || e.id || ','
		     FROM entities e
		     WHERE e.project_id = ?
		       AND e.id IN (SELECT value FROM json_each(?))
		     UNION ALL
		     SELECT
		         CASE WHEN r.source_id = t.entity_id THEN r.target_id ELSE r.source_id END,
		         t.depth + 1,
		         t.visited || CASE WHEN r.source_id = t.entity_id THEN r.target_id ELSE r.source_id END || ','
		     FROM relationships r
		     JOIN traversal t ON (r.source_id = t.entity_id OR r.target_id = t.entity_id)
		     WHERE r.project_id = ?
		       AND t.depth < ?
		       AND t.visited NOT LIKE '%,' ||
		               CASE WHEN r.source_id = t.entity_id THEN r.target_id ELSE r.source_id END
		               || ',%'
		 )
		 SELECT DISTINCT e.id, e.type, e.canonical_name, e.aliases, e.metadata
		 FROM entities e
		 JOIN (SELECT DISTINCT entity_id FROM traversal WHERE depth > 0) trav ON e.id = trav.entity_id
		 WHERE e.project_id = ?`,
		projectID, string(seedsJSON), projectID, depth, projectID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite: traverse entities: %w", err)
	}
	defer rows.Close()

	var expanded []domain.Entity
	expandedIDs := make([]string, 0)
	for rows.Next() {
		e, err := scanSQLiteEntityRow(rows)
		if err != nil {
			return nil, nil, err
		}
		expanded = append(expanded, *e)
		expandedIDs = append(expandedIDs, e.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(expanded) == 0 {
		return nil, nil, nil
	}

	// Fetch the induced subgraph relationships (both endpoints in seeds ∪ expanded).
	allIDs := append(entityIDs, expandedIDs...)
	allIDsJSON, _ := json.Marshal(allIDs)
	relRows, err := db.db.QueryContext(ctx,
		`SELECT id, source_id, target_id, relationship_type, confidence
		 FROM relationships
		 WHERE project_id = ?
		   AND source_id IN (SELECT value FROM json_each(?))
		   AND target_id IN (SELECT value FROM json_each(?))`,
		projectID, string(allIDsJSON), string(allIDsJSON),
	)
	if err != nil {
		return expanded, nil, fmt.Errorf("sqlite: traverse relationships: %w", err)
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

// --- helpers ---

type sqliteRowScanner interface {
	Scan(dest ...any) error
}

func scanSQLiteEntity(row sqliteRowScanner) (*domain.Entity, error) {
	var e domain.Entity
	var aliasesStr, metaStr string
	if err := row.Scan(&e.ID, &e.Type, &e.CanonicalName, &aliasesStr, &metaStr); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(aliasesStr), &e.Aliases); err != nil {
		e.Aliases = nil
	}
	if err := json.Unmarshal([]byte(metaStr), &e.Metadata); err != nil {
		e.Metadata = nil
	}
	return &e, nil
}

func scanSQLiteEntityRow(rows *sql.Rows) (*domain.Entity, error) {
	var e domain.Entity
	var aliasesStr, metaStr string
	if err := rows.Scan(&e.ID, &e.Type, &e.CanonicalName, &aliasesStr, &metaStr); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(aliasesStr), &e.Aliases); err != nil {
		e.Aliases = nil
	}
	if err := json.Unmarshal([]byte(metaStr), &e.Metadata); err != nil {
		e.Metadata = nil
	}
	return &e, nil
}

func scanSQLiteObservation(row sqliteRowScanner) (*domain.Observation, error) {
	var o domain.Observation
	var metaStr string
	var occurredAt, observedAt, resolvedAt, learnedAt sql.NullString
	if err := row.Scan(
		&o.ID, &o.ProjectID, &o.SourceID, &o.Content,
		&occurredAt, &observedAt, &resolvedAt, &learnedAt,
		&metaStr,
	); err != nil {
		return nil, err
	}
	o.Temporal = domain.Temporal{
		OccurredAt: parseNullTime(occurredAt),
		ObservedAt: parseNullTime(observedAt),
		ResolvedAt: parseNullTime(resolvedAt),
		LearnedAt:  parseNullTime(learnedAt),
	}
	if err := json.Unmarshal([]byte(metaStr), &o.Metadata); err != nil {
		o.Metadata = nil
	}
	return &o, nil
}

func scanSQLiteObservationRow(rows *sql.Rows) (*domain.Observation, error) {
	var o domain.Observation
	var metaStr string
	var occurredAt, observedAt, resolvedAt, learnedAt sql.NullString
	if err := rows.Scan(
		&o.ID, &o.ProjectID, &o.SourceID, &o.Content,
		&occurredAt, &observedAt, &resolvedAt, &learnedAt,
		&metaStr,
	); err != nil {
		return nil, err
	}
	o.Temporal = domain.Temporal{
		OccurredAt: parseNullTime(occurredAt),
		ObservedAt: parseNullTime(observedAt),
		ResolvedAt: parseNullTime(resolvedAt),
		LearnedAt:  parseNullTime(learnedAt),
	}
	if err := json.Unmarshal([]byte(metaStr), &o.Metadata); err != nil {
		o.Metadata = nil
	}
	return &o, nil
}

func scanSQLiteEntityConfidence(row sqliteRowScanner) (*domain.EntityConfidence, error) {
	var ec domain.EntityConfidence
	var computedAtStr string
	var accuracyRate sql.NullFloat64
	if err := row.Scan(
		&ec.ID, &ec.EntityID, &ec.ProjectID, &ec.Score,
		&ec.ObservationCount, &ec.ResolvedCount, &accuracyRate, &computedAtStr,
	); err != nil {
		return nil, err
	}
	if accuracyRate.Valid {
		v := accuracyRate.Float64
		ec.AccuracyRate = &v
	}
	if t, err := time.Parse(time.RFC3339, computedAtStr); err == nil {
		ec.ComputedAt = t
	}
	return &ec, nil
}

func scanSQLiteEntityConfidenceRow(rows *sql.Rows) (*domain.EntityConfidence, error) {
	var ec domain.EntityConfidence
	var computedAtStr string
	var accuracyRate sql.NullFloat64
	if err := rows.Scan(
		&ec.ID, &ec.EntityID, &ec.ProjectID, &ec.Score,
		&ec.ObservationCount, &ec.ResolvedCount, &accuracyRate, &computedAtStr,
	); err != nil {
		return nil, err
	}
	if accuracyRate.Valid {
		v := accuracyRate.Float64
		ec.AccuracyRate = &v
	}
	if t, err := time.Parse(time.RFC3339, computedAtStr); err == nil {
		ec.ComputedAt = t
	}
	return &ec, nil
}

// nullableTime converts a *time.Time to a SQL-compatible string or nil.
func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// parseNullTime converts a nullable SQL string into *time.Time.
// Accepts RFC3339 and a fallback to date-only format.
func parseNullTime(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, ns.String); err == nil {
			return &t
		}
	}
	return nil
}

// sqliteKeywords tokenises a natural-language query for FTS5 / LIKE matching.
func sqliteKeywords(query string) []string {
	stop := map[string]bool{
		"a": true, "an": true, "the": true, "is": true, "are": true,
		"was": true, "were": true, "what": true, "how": true, "why": true,
		"when": true, "did": true, "do": true, "does": true, "in": true,
		"of": true, "to": true, "for": true, "and": true, "or": true,
		"about": true, "made": true, "been": true, "has": true, "have": true,
	}
	replacer := strings.NewReplacer("?", "", "!", "", ".", "", ",", "", "'", "", `"`, "")
	var keywords []string
	for _, w := range strings.Fields(replacer.Replace(strings.ToLower(query))) {
		if len(w) >= 3 && !stop[w] {
			keywords = append(keywords, w)
		}
	}
	return keywords
}

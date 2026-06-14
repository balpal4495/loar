package postgres

import (
	"context"
	"encoding/json"
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
	return err
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

// SearchObservations performs a case-insensitive keyword search against
// observation content within a project.
func (db *DB) SearchObservations(ctx context.Context, projectID, query string) ([]*domain.Observation, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT id, project_id, source_id, content,
		        occurred_at, observed_at, resolved_at, learned_at, metadata
		 FROM observations
		 WHERE project_id = $1 AND content ILIKE '%' || $2 || '%'
		 ORDER BY created_at`,
		projectID, query,
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

// CreateRelationship inserts a new relationship. If r.ID is empty a new UUID is assigned.
func (db *DB) CreateRelationship(ctx context.Context, projectID string, r *domain.Relationship) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	_, err := db.pool.Exec(ctx,
		`INSERT INTO relationships
		 (id, project_id, source_id, target_id, relationship_type, confidence)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		r.ID, projectID, r.SourceID, r.TargetID, r.Type, r.Confidence,
	)
	return err
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

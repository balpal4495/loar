// Package store defines the storage interfaces used throughout Loar.
// Concrete implementations (e.g. Postgres) live in sub-packages.
package store

import (
	"context"

	"github.com/balpal4495/loar/internal/domain"
)

// ProjectStore manages project persistence.
type ProjectStore interface {
	CreateProject(ctx context.Context, p *domain.Project) error
	GetProject(ctx context.Context, id string) (*domain.Project, error)
	GetProjectByName(ctx context.Context, name string) (*domain.Project, error)
	ListProjects(ctx context.Context) ([]*domain.Project, error)
}

// EntityStore manages entity persistence.
type EntityStore interface {
	CreateEntity(ctx context.Context, projectID string, e *domain.Entity) error
	GetEntity(ctx context.Context, id string) (*domain.Entity, error)
	FindEntityByName(ctx context.Context, projectID, name string) (*domain.Entity, error)
	ListEntities(ctx context.Context, projectID string) ([]*domain.Entity, error)
}

// ObservationStore manages observation persistence.
type ObservationStore interface {
	CreateObservation(ctx context.Context, o *domain.Observation) error
	GetObservation(ctx context.Context, id string) (*domain.Observation, error)
	ListObservations(ctx context.Context, projectID string) ([]*domain.Observation, error)
	SearchObservations(ctx context.Context, projectID, query string) ([]*domain.Observation, error)
	// ExistsObservationBySourceID reports whether the project already contains
	// an observation with the given source_id. Used for incremental ingestion.
	ExistsObservationBySourceID(ctx context.Context, projectID, sourceID string) (bool, error)
}

// RelationshipStore manages relationship persistence.
type RelationshipStore interface {
	CreateRelationship(ctx context.Context, projectID string, r *domain.Relationship) error
	ListRelationships(ctx context.Context, projectID string) ([]*domain.Relationship, error)
	FindRelationships(ctx context.Context, projectID, entityID string) ([]*domain.Relationship, error)
}

// ObservationEntityStore links observations to entities.
type ObservationEntityStore interface {
	Link(ctx context.Context, observationID, entityID, role string) error
	EntitiesForObservation(ctx context.Context, observationID string) ([]*domain.Entity, error)
	ObservationsForEntity(ctx context.Context, entityID string) ([]*domain.Observation, error)
}

// EntityConfidenceStore tracks entity trust scores over time.
// Each loar-learn run appends a row rather than mutating the entity record,
// so confidence trend is queryable (improving vs degrading).
type EntityConfidenceStore interface {
	WriteEntityConfidence(ctx context.Context, ec *domain.EntityConfidence) error
	LatestEntityConfidence(ctx context.Context, projectID, entityID string) (*domain.EntityConfidence, error)
	EntityConfidenceHistory(ctx context.Context, projectID, entityID string, limit int) ([]*domain.EntityConfidence, error)
}

// GraphTraversalStore performs multi-hop traversal over the entity-relationship
// graph. The default implementations use recursive CTEs in the relational
// backends. A native graph database (Kuzu, Neo4j) would implement the same
// interface and replace the CTE approach with purpose-built graph queries.
type GraphTraversalStore interface {
	// TraverseFromEntities returns all entities reachable within depth hops
	// from the seed entity IDs, plus all relationships connecting any node in
	// the expanded set. depth=1 is direct neighbours only; depth=2 expands
	// one hop further, enabling "why is Romano reliable?" to discover Arsenal
	// → scouts → confirmations without an explicit join.
	TraverseFromEntities(ctx context.Context, projectID string, entityIDs []string, depth int) ([]domain.Entity, []domain.Relationship, error)
}

// Store is the unified storage interface used by the retrieval engine and
// ingestion layer.
type Store interface {
	ProjectStore
	EntityStore
	ObservationStore
	RelationshipStore
	ObservationEntityStore
	EntityConfidenceStore
	GraphTraversalStore
	Migrate(ctx context.Context) error
	Close()
}

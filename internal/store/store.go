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

// Store is the unified storage interface used by the retrieval engine and
// ingestion layer.
type Store interface {
	ProjectStore
	EntityStore
	ObservationStore
	RelationshipStore
	ObservationEntityStore
	Migrate(ctx context.Context) error
	Close()
}

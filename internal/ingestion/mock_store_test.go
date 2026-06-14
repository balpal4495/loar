package ingestion_test

import (
	"context"

	"github.com/balpal4495/loar/internal/domain"
)

// mockStore implements only the ObservationStore portion needed by Ingestor.
type mockStore struct {
	observations []*domain.Observation
}

func newMockStore() *mockStore { return &mockStore{} }

func (m *mockStore) CreateObservation(_ context.Context, o *domain.Observation) error {
	cp := *o
	m.observations = append(m.observations, &cp)
	return nil
}

// Unused methods – present to satisfy store.Store interface structurally.
// Ingestor only calls CreateObservation, so the others are stubs.
func (m *mockStore) CreateProject(_ context.Context, _ *domain.Project) error { return nil }
func (m *mockStore) GetProject(_ context.Context, _ string) (*domain.Project, error) {
	return nil, nil
}
func (m *mockStore) GetProjectByName(_ context.Context, _ string) (*domain.Project, error) {
	return nil, nil
}
func (m *mockStore) ListProjects(_ context.Context) ([]*domain.Project, error) { return nil, nil }
func (m *mockStore) CreateEntity(_ context.Context, _ string, _ *domain.Entity) error { return nil }
func (m *mockStore) GetEntity(_ context.Context, _ string) (*domain.Entity, error) { return nil, nil }
func (m *mockStore) FindEntityByName(_ context.Context, _, _ string) (*domain.Entity, error) {
	return nil, nil
}
func (m *mockStore) ListEntities(_ context.Context, _ string) ([]*domain.Entity, error) {
	return nil, nil
}
func (m *mockStore) GetObservation(_ context.Context, _ string) (*domain.Observation, error) {
	return nil, nil
}
func (m *mockStore) ListObservations(_ context.Context, _ string) ([]*domain.Observation, error) {
	return nil, nil
}
func (m *mockStore) SearchObservations(_ context.Context, _, _ string) ([]*domain.Observation, error) {
	return nil, nil
}
func (m *mockStore) CreateRelationship(_ context.Context, _ string, _ *domain.Relationship) error {
	return nil
}
func (m *mockStore) ListRelationships(_ context.Context, _ string) ([]*domain.Relationship, error) {
	return nil, nil
}
func (m *mockStore) FindRelationships(_ context.Context, _, _ string) ([]*domain.Relationship, error) {
	return nil, nil
}
func (m *mockStore) Link(_ context.Context, _, _, _ string) error { return nil }
func (m *mockStore) EntitiesForObservation(_ context.Context, _ string) ([]*domain.Entity, error) {
	return nil, nil
}
func (m *mockStore) ObservationsForEntity(_ context.Context, _ string) ([]*domain.Observation, error) {
	return nil, nil
}
func (m *mockStore) Migrate(_ context.Context) error { return nil }
func (m *mockStore) Close()                          {}

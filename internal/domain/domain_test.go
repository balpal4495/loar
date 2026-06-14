package domain_test

import (
	"testing"
	"time"

	"github.com/balpal4495/loar/internal/domain"
)

func TestTemporalFields(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Hour)

	temp := domain.Temporal{
		OccurredAt: &now,
		ObservedAt: &later,
	}

	if *temp.OccurredAt != now {
		t.Errorf("OccurredAt: want %v, got %v", now, *temp.OccurredAt)
	}
	if *temp.ObservedAt != later {
		t.Errorf("ObservedAt: want %v, got %v", later, *temp.ObservedAt)
	}
	if temp.ResolvedAt != nil {
		t.Error("ResolvedAt should be nil")
	}
	if temp.LearnedAt != nil {
		t.Error("LearnedAt should be nil")
	}
}

func TestObservationConstruction(t *testing.T) {
	now := time.Now()
	obs := domain.Observation{
		ID:        "obs-1",
		ProjectID: "proj-1",
		Content:   "Romano reported the transfer.",
		SourceID:  "twitter",
		Temporal:  domain.Temporal{ObservedAt: &now},
		Metadata:  map[string]any{"confidence": 0.9},
	}

	if obs.ID != "obs-1" {
		t.Errorf("ID: want obs-1, got %s", obs.ID)
	}
	if obs.Content == "" {
		t.Error("Content should not be empty")
	}
	if obs.Temporal.ObservedAt == nil {
		t.Error("ObservedAt should not be nil")
	}
}

func TestEntityAliases(t *testing.T) {
	e := domain.Entity{
		ID:            "entity-1",
		Type:          "person",
		CanonicalName: "Fabrizio Romano",
		Aliases:       []string{"Romano", "@FabrizioRomano"},
		Metadata:      map[string]any{"role": "journalist"},
	}

	if e.CanonicalName != "Fabrizio Romano" {
		t.Errorf("CanonicalName: want 'Fabrizio Romano', got %s", e.CanonicalName)
	}
	if len(e.Aliases) != 2 {
		t.Errorf("Aliases: want 2, got %d", len(e.Aliases))
	}
}

func TestRelationshipConfidence(t *testing.T) {
	r := domain.Relationship{
		ID:         "rel-1",
		SourceID:   "obs-1",
		TargetID:   "entity-1",
		Type:       "reported_by",
		Confidence: 0.95,
	}

	if r.Confidence < 0 || r.Confidence > 1 {
		t.Errorf("Confidence should be in [0, 1], got %f", r.Confidence)
	}
}

func TestContextPackageZeroValue(t *testing.T) {
	var pkg domain.ContextPackage
	if pkg.Confidence != 0 {
		t.Error("zero ContextPackage should have Confidence 0")
	}
	if pkg.Entities != nil {
		t.Error("zero ContextPackage should have nil Entities slice")
	}
}

package domain

// TimelineEvent represents a point in time within a retrieved context.
type TimelineEvent struct {
	Temporal Temporal
	Summary  string
}

// Evidence represents a piece of supporting evidence for a context package.
type Evidence struct {
	ObservationID string
	Summary       string
	Confidence    float64
}

// Contradiction represents conflicting observations within a context package.
type Contradiction struct {
	ObservationAID string
	ObservationBID string
	Summary        string
}

// ContextPackage is the primary retrieval output of Loar.
// Loar never exposes raw database rows; it generates a ContextPackage.
type ContextPackage struct {
	Query          string
	Summary        string
	Entities       []Entity
	Observations   []Observation
	Relationships  []Relationship
	Timeline       []TimelineEvent
	Evidence       []Evidence
	Contradictions []Contradiction
	Confidence     float64
	RelatedTopics  []string
}

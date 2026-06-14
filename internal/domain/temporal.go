package domain

import "time"

// Temporal captures multiple time dimensions for an observation.
// Time is a first-class concern in Loar.
type Temporal struct {
	// OccurredAt is when the event happened.
	OccurredAt *time.Time

	// ObservedAt is when Loar became aware.
	ObservedAt *time.Time

	// ResolvedAt is when the outcome became known.
	ResolvedAt *time.Time

	// LearnedAt is when Loar derived new understanding.
	LearnedAt *time.Time
}

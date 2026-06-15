package domain

import "time"

// EntityConfidence records the computed trust score for an entity at a point
// in time. Each `loar learn` run appends a new row rather than mutating the
// entity record — preserving the trend so confidence can be tracked over time.
//
// This is the architectural lesson from SSC's weight_regime: a mechanism for
// capturing not just the current score, but whether confidence is improving or
// degrading across phases, ingests, or source updates.
type EntityConfidence struct {
	ID string

	EntityID  string
	ProjectID string

	// Score is the mean recency-weighted confidence across all linked
	// observations, normalised to [0, 1]. Formula: 0.5 + 0.5*e^(-days/365)
	// per observation, then averaged.
	Score float64

	ObservationCount int

	// ResolvedCount is the number of linked observations with a non-nil
	// ResolvedAt. Used as the denominator for AccuracyRate.
	ResolvedCount int

	// AccuracyRate is nil until source reliability scoring is implemented.
	// When outcome scoring is added, this becomes the fraction of resolved
	// observations that matched their predicted outcome.
	AccuracyRate *float64

	ComputedAt time.Time
}

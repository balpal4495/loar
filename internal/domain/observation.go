package domain

// Observation is the smallest meaningful unit of knowledge.
// Everything begins as an Observation.
// Examples: rumour, claim, prediction, event, outcome, signal.
type Observation struct {
	ID        string
	ProjectID string
	Content   string
	SourceID  string
	Temporal  Temporal
	Metadata  map[string]any
}

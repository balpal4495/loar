package domain

// Relationship connects two entities or observations.
// Examples: reported_by, caused, supports, contradicts, related_to, part_of.
type Relationship struct {
	ID         string
	SourceID   string
	TargetID   string
	Type       string
	Confidence float64
}

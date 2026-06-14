package domain

// Entity represents something that exists within a project.
// Entities act as retrieval anchors.
// Examples: person, company, team, stock, project, organisation, character.
type Entity struct {
	ID            string
	Type          string
	CanonicalName string
	Aliases       []string
	Metadata      map[string]any
}

package domain

// Project is a knowledge boundary.
// Knowledge is scoped to a project to prevent unintended cross-contamination.
type Project struct {
	ID          string
	Name        string
	Description string
}

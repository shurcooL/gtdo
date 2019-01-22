package page

// State that is passed to the frontend script from the backend handler.
type State struct {
	ImportPath   string
	ProcessedRev string // ProcessedRev is processed rev; its value is replaced by default branch if empty.
	CommitID     string
}

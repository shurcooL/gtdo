package page

// State that is passed to the frontend script from the backend handler.
type State struct {
	ImportPath   string
	RepoSpec     repoSpec
	ProcessedRev string // ProcessedRev is processed rev; its value is replaced by default branch if empty.
	CommitID     string
}

// TODO: Dedup. But probably by moving it to a common lower level package for types... Not sure if this package is best for it.
// repoSpec identifies a repository for go-vcs purposes.
type repoSpec struct {
	VCSType  string
	CloneURL string
}

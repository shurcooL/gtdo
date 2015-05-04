package server

import (
	"fmt"
	"net/http"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

func (h *Handler) serveRepoTags(w http.ResponseWriter, r *http.Request) error {
	repo, _, done, err := h.getRepo(r)
	if err != nil {
		return err
	}
	defer done()

	type tags interface {
		Tags() ([]*vcs.Tag, error)
	}
	if repo, ok := repo.(tags); ok {
		tags, err := repo.Tags()
		if err != nil {
			return err
		}

		return writeJSON(w, tags)
	}

	return &httpError{http.StatusNotImplemented, fmt.Errorf("Tags not yet implemented for %T", repo)}
}

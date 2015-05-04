package server

import (
	"fmt"
	"net/http"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

func (h *Handler) serveRepoBranches(w http.ResponseWriter, r *http.Request) error {
	repo, _, done, err := h.getRepo(r)
	if err != nil {
		return err
	}
	defer done()

	type branches interface {
		Branches() ([]*vcs.Branch, error)
	}
	if repo, ok := repo.(branches); ok {
		branches, err := repo.Branches()
		if err != nil {
			return err
		}

		return writeJSON(w, branches)
	}

	return &httpError{http.StatusNotImplemented, fmt.Errorf("Branches not yet implemented for %T", repo)}
}

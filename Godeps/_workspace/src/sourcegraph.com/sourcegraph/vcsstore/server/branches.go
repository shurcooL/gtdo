package server

import (
	"fmt"
	"net/http"

	"github.com/dustin/go-humanize"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

func (h *Handler) serveRepoBranches(w http.ResponseWriter, r *http.Request) error {
	repo, _, done, lu, err := h.getRepo(r)
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

		w.Header().Set("Last-Updated", lu.Format(http.TimeFormat))
		fmt.Println("serveRepoBranches: serving LastUpdated:", humanize.Time(lu))

		return writeJSON(w, branches)
	}

	return &httpError{http.StatusNotImplemented, fmt.Errorf("Branches not yet implemented for %T", repo)}
}

package server

import (
	"fmt"
	"log"
	"net/http"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

func (h *Handler) serveRepoSearch(w http.ResponseWriter, r *http.Request) error {
	repo, _, done, err := h.getRepo(r)
	if err != nil {
		return err
	}
	defer done()

	var opt vcs.SearchOptions
	if err := schemaDecoder.Decode(&opt, r.URL.Query()); err != nil {
		log.Println(err)
		return err
	}

	rev, canon, err := getCommitID(r)
	if err != nil {
		return err
	}

	var commitID vcs.CommitID
	if canon {
		commitID = rev
	} else {
		var err error
		type revisionResolver interface {
			ResolveRevision(string) (vcs.CommitID, error)
		}
		commitID, err = repo.(revisionResolver).ResolveRevision(string(rev))
		if err != nil {
			return err
		}
	}

	if repo, ok := repo.(vcs.Searcher); ok {
		res, err := repo.Search(commitID, opt)
		if err != nil {
			return err
		}

		if canon {
			setLongCache(w)
		} else {
			setShortCache(w)
		}

		return writeJSON(w, res)
	}

	return &httpError{http.StatusNotImplemented, fmt.Errorf("Search not yet implemented for %T", repo)}
}

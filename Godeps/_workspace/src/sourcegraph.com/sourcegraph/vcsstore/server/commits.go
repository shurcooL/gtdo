package server

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	"sourcegraph.com/sourcegraph/vcsstore/vcsclient"
)

func (h *Handler) serveRepoCommits(w http.ResponseWriter, r *http.Request) error {
	repo, _, done, err := h.getRepo(r)
	if err != nil {
		return err
	}
	defer done()

	var opt vcs.CommitsOptions
	if err := schemaDecoder.Decode(&opt, r.URL.Query()); err != nil {
		log.Println(err)
		return err
	}

	head, canon, err := checkCommitID(string(opt.Head))
	if err != nil {
		return err
	}
	opt.Head = head

	type commits interface {
		Commits(opt vcs.CommitsOptions) ([]*vcs.Commit, uint, error)
	}
	if repo, ok := repo.(commits); ok {
		commits, total, err := repo.Commits(opt)
		if err != nil {
			return err
		}

		if canon {
			setLongCache(w)
		} else {
			setShortCache(w)
		}

		w.Header().Set(vcsclient.TotalCommitsHeader, strconv.FormatUint(uint64(total), 10))

		return writeJSON(w, commits)
	}

	return &httpError{http.StatusNotImplemented, fmt.Errorf("Commits not yet implemented for %T", repo)}
}

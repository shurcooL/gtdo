package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/sourcegraph/mux"
	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

func (h *Handler) serveRepoCommit(w http.ResponseWriter, r *http.Request) error {
	v := mux.Vars(r)

	repo, cloneURL, done, err := h.getRepo(r)
	if err != nil {
		return err
	}
	defer done()

	commitID, canon, err := getCommitID(r)
	if err != nil {
		return err
	}

	type getCommit interface {
		GetCommit(vcs.CommitID) (*vcs.Commit, error)
	}
	if repo, ok := repo.(getCommit); ok {
		commit, err := repo.GetCommit(commitID)
		if err != nil {
			return err
		}

		if commit.ID != commitID {
			setShortCache(w)
			http.Redirect(w, r, h.router.URLToRepoCommit(v["VCS"], cloneURL, commit.ID).String(), http.StatusFound)
			return nil
		}

		if canon {
			setLongCache(w)
		}
		return writeJSON(w, commit)
	}

	return &httpError{http.StatusNotImplemented, fmt.Errorf("GetCommit not yet implemented for %T", repo)}
}

// getCommitID retrieves the CommitID from the route variables and
// runs checkCommitID on it.
func getCommitID(r *http.Request) (vcs.CommitID, bool, error) {
	return checkCommitID(mux.Vars(r)["CommitID"])
}

// checkCommitID returns whether the commit ID is canonical (i.e., the
// full 40-character commit ID), and an error (if any).
func checkCommitID(commitID string) (vcs.CommitID, bool, error) {
	if commitID == "" {
		return "", false, &httpError{http.StatusBadRequest, errors.New("CommitID is empty")}
	}

	if !isLowercaseHex(commitID) {
		return "", false, &httpError{http.StatusBadRequest, errors.New("CommitID must be lowercase hex")}
	}

	return vcs.CommitID(commitID), commitIDIsCanon(commitID), nil
}

func commitIDIsCanon(commitID string) bool {
	return len(commitID) == 40
}

func isLowercaseHex(s string) bool {
	return strings.IndexFunc(s, func(c rune) bool {
		return !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'))
	}) == -1
}

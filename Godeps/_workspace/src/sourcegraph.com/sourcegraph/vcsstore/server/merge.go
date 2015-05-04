package server

import (
	"fmt"
	"net/http"

	"github.com/sourcegraph/mux"
	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

func (h *Handler) serveRepoMergeBase(w http.ResponseWriter, r *http.Request) error {
	v := mux.Vars(r)

	repo, cloneURL, done, err := h.getRepo(r)
	if err != nil {
		return err
	}
	defer done()

	if repo, ok := repo.(vcs.Merger); ok {
		a, b := vcs.CommitID(v["CommitIDA"]), vcs.CommitID(v["CommitIDB"])

		mb, err := repo.MergeBase(a, b)
		if err != nil {
			return err
		}

		var statusCode int
		if commitIDIsCanon(string(a)) && commitIDIsCanon(string(b)) {
			setLongCache(w)
			statusCode = http.StatusMovedPermanently
		} else {
			setShortCache(w)
			statusCode = http.StatusFound
		}
		http.Redirect(w, r, h.router.URLToRepoCommit(v["VCS"], cloneURL, mb).String(), statusCode)
		return nil
	}

	return &httpError{http.StatusNotImplemented, fmt.Errorf("Merger not yet implemented by %T", repo)}
}

func (h *Handler) serveRepoCrossRepoMergeBase(w http.ResponseWriter, r *http.Request) error {
	v := mux.Vars(r)

	repoA, cloneURLA, doneA, err := h.getRepo(r)
	if err != nil {
		return err
	}
	defer doneA()

	repoB, _, doneB, err := h.getRepoLabeled(r, "B")
	if err != nil {
		return err
	}
	defer doneB()

	if repoA, ok := repoA.(vcs.CrossRepoMerger); ok {
		a, b := vcs.CommitID(v["CommitIDA"]), vcs.CommitID(v["CommitIDB"])

		mb, err := repoA.CrossRepoMergeBase(a, repoB.(vcs.Repository), b)
		if err != nil {
			return err
		}

		var statusCode int
		if commitIDIsCanon(string(a)) && commitIDIsCanon(string(b)) {
			setLongCache(w)
			statusCode = http.StatusMovedPermanently
		} else {
			setShortCache(w)
			statusCode = http.StatusFound
		}
		http.Redirect(w, r, h.router.URLToRepoCommit(v["VCS"], cloneURLA, mb).String(), statusCode)
		return nil
	}

	return &httpError{http.StatusNotImplemented, fmt.Errorf("CrossRepoMerger not yet implemented by %T", repoA)}
}

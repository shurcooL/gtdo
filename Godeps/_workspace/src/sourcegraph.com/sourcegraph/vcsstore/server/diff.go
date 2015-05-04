package server

import (
	"fmt"
	"log"
	"net/http"

	"github.com/sourcegraph/mux"
	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

func (h *Handler) serveRepoDiff(w http.ResponseWriter, r *http.Request) error {
	v := mux.Vars(r)

	repo, _, done, err := h.getRepo(r)
	if err != nil {
		return err
	}
	defer done()

	var opt vcs.DiffOptions
	if err := schemaDecoder.Decode(&opt, r.URL.Query()); err != nil {
		log.Println(err)
		return err
	}

	if repo, ok := repo.(vcs.Differ); ok {
		diff, err := repo.Diff(vcs.CommitID(v["Base"]), vcs.CommitID(v["Head"]), &opt)
		if err != nil {
			return err
		}

		_, baseCanon, err := checkCommitID(v["Base"])
		if err != nil {
			return err
		}
		_, headCanon, err := checkCommitID(v["Head"])
		if err != nil {
			return err
		}
		if baseCanon && headCanon {
			setLongCache(w)
		} else {
			setShortCache(w)
		}

		return writeJSON(w, diff)
	}

	return &httpError{http.StatusNotImplemented, fmt.Errorf("Diff not yet implemented for %T", repo)}
}

func (h *Handler) serveRepoCrossRepoDiff(w http.ResponseWriter, r *http.Request) error {
	v := mux.Vars(r)

	baseRepo, _, doneBase, err := h.getRepo(r)
	if err != nil {
		return err
	}
	defer doneBase()

	headRepo, _, doneHead, err := h.getRepoLabeled(r, "Head")
	if err != nil {
		return err
	}
	defer doneHead()

	var opt vcs.DiffOptions
	if err := schemaDecoder.Decode(&opt, r.URL.Query()); err != nil {
		log.Println(err)
		return err
	}

	if baseRepo, ok := baseRepo.(vcs.CrossRepoDiffer); ok {
		diff, err := baseRepo.CrossRepoDiff(vcs.CommitID(v["Base"]), headRepo.(vcs.Repository), vcs.CommitID(v["Head"]), &opt)
		if err != nil {
			return err
		}

		_, baseCanon, err := checkCommitID(v["Base"])
		if err != nil {
			return err
		}
		_, headCanon, err := checkCommitID(v["Head"])
		if err != nil {
			return err
		}
		if baseCanon && headCanon {
			setLongCache(w)
		} else {
			setShortCache(w)
		}

		return writeJSON(w, diff)
	}

	return &httpError{http.StatusNotImplemented, fmt.Errorf("CrossRepoDiff not yet implemented for %T", baseRepo)}
}

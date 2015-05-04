package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/sourcegraph/mux"
	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	"sourcegraph.com/sourcegraph/vcsstore/vcsclient"
)

func (h *Handler) serveRepo(w http.ResponseWriter, r *http.Request) error {
	repo, cloneURL, done, err := h.getRepo(r)
	if err != nil {
		return err
	}
	defer done()

	return writeJSON(w, struct {
		ImplementationType string
		CloneURL           string
	}{fmt.Sprintf("%T", repo), cloneURL.String()})
}

func (h *Handler) serveRepoCreateOrUpdate(w http.ResponseWriter, r *http.Request) error {
	var opt vcs.RemoteOpts
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&opt); err != nil {
			return err
		}
	}

	var cloned bool // whether the repo was newly cloned
	vcsType := mux.Vars(r)["VCS"]
	repo, cloneURL, _, err := h.getRepo(r)
	if errorHTTPStatusCode(err) == http.StatusNotFound {
		cloned = true
		repo, err = h.Service.Clone(vcsType, cloneURL, opt)
	}
	if err != nil {
		return cloneOrUpdateError(err)
	}
	defer h.Service.Close(vcsType, cloneURL)

	if cloned {
		w.WriteHeader(http.StatusCreated)
		return nil
	}

	type updateEverythinger interface {
		UpdateEverything(opt vcs.RemoteOpts) error
	}
	if repo, ok := repo.(updateEverythinger); ok {
		err := repo.UpdateEverything(opt)
		if err != nil {
			return cloneOrUpdateError(err)
		}

		return nil
	}
	return &httpError{http.StatusNotImplemented, fmt.Errorf("Remote updates not yet implemented for %T", repo)}
}

func cloneOrUpdateError(err error) error {
	if err != nil {
		var c int
		switch err.Error() {
		case "authentication required but no callback set":
			c = http.StatusUnauthorized
		case "callback returned unsupported credentials type":
			c = http.StatusUnauthorized
		case "Failed to authenticate SSH session: Waiting for USERAUTH response":
			c = http.StatusForbidden
		}
		if c != 0 {
			return &httpError{err: err, statusCode: c}
		}
	}
	return err
}

type getRepoMode int

const (
	cloneIfNotExists = 1 << iota
)

func (h *Handler) getRepo(r *http.Request) (repo interface{}, cloneURL *url.URL, done func(), err error) {
	return h.getRepoLabeled(r, "")
}

// getRepoLabel allows either getting the main repo in the URL or
// another one, such as the head repo for cross-repo diffs.
func (h *Handler) getRepoLabeled(r *http.Request, label string) (repo interface{}, cloneURL *url.URL, done func(), err error) {
	v := mux.Vars(r)
	vcsType := v[label+"VCS"]
	cloneURLStr := v[label+"CloneURL"]
	if cloneURLStr == "" {
		// If cloneURLStr is empty, then the CloneURLEscaped route var failed to
		// be unescaped using url.QueryUnescape.
		return nil, nil, nil, &httpError{http.StatusBadRequest, errors.New("invalid clone URL (unescaping failed)")}
	}

	cloneURL, err = url.Parse(cloneURLStr)
	if err != nil {
		return nil, nil, nil, &httpError{http.StatusBadRequest, errors.New("invalid clone URL (parsing failed)")}
	}

	if cloneURL.Scheme == "" || cloneURL.Host == "" {
		return nil, cloneURL, nil, errors.New("invalid clone URL")
	}

	repo, err = h.Service.Open(vcsType, cloneURL)
	if err != nil {
		if os.IsNotExist(err) {
			err = &httpError{http.StatusNotFound, vcsclient.ErrRepoNotExist}
		}
		return nil, cloneURL, nil, err
	}

	done = func() {
		h.Service.Close(vcsType, cloneURL)
	}

	return repo, cloneURL, done, nil
}

package server

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
	"time"

	"github.com/gorilla/schema"
	"github.com/sourcegraph/mux"
	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	"sourcegraph.com/sourcegraph/vcsstore"
	"sourcegraph.com/sourcegraph/vcsstore/vcsclient"
)

type Handler struct {
	Service vcsstore.Service

	router *vcsclient.Router

	Log *log.Logger

	// Debug is whether to report internal error messages to HTTP clients.
	//
	// IMPORTANT NOTE: This should be set to false in publicly available
	// servers, as internal error messages may reveal sensitive information.
	Debug bool
}

// NewHandler adds routes and handlers to an existing parent router (or creates
// one if parent is nil). If wrap is non-nil, it is called on each internal
// handler before being registered as the handler for a router.
func NewHandler(svc vcsstore.Service, parent *mux.Router, wrap func(http.Handler) http.Handler) *Handler {
	router := vcsclient.NewRouter(parent)
	r := (*mux.Router)(router)

	if wrap == nil {
		wrap = func(h http.Handler) http.Handler { return h }
	}

	h := &Handler{
		Service: svc,
		router:  router,
		Log:     log.New(ioutil.Discard, "", 0),
	}

	r.Get(vcsclient.RouteRoot).Handler(wrap(handler{h, h.serveRoot}))
	r.Get(vcsclient.RouteRepo).Handler(wrap(handler{h, h.serveRepo}))
	r.Get(vcsclient.RouteRepoCreateOrUpdate).Handler(wrap(handler{h, h.serveRepoCreateOrUpdate}))
	r.Get(vcsclient.RouteRepoBlameFile).Handler(wrap(handler{h, h.serveRepoBlameFile}))
	r.Get(vcsclient.RouteRepoBranch).Handler(wrap(handler{h, h.serveRepoBranch}))
	r.Get(vcsclient.RouteRepoBranches).Handler(wrap(handler{h, h.serveRepoBranches}))
	r.Get(vcsclient.RouteRepoCommit).Handler(wrap(handler{h, h.serveRepoCommit}))
	r.Get(vcsclient.RouteRepoCommits).Handler(wrap(handler{h, h.serveRepoCommits}))
	r.Get(vcsclient.RouteRepoDiff).Handler(wrap(handler{h, h.serveRepoDiff}))
	r.Get(vcsclient.RouteRepoCrossRepoDiff).Handler(wrap(handler{h, h.serveRepoCrossRepoDiff}))
	r.Get(vcsclient.RouteRepoMergeBase).Handler(wrap(handler{h, h.serveRepoMergeBase}))
	r.Get(vcsclient.RouteRepoCrossRepoMergeBase).Handler(wrap(handler{h, h.serveRepoCrossRepoMergeBase}))
	r.Get(vcsclient.RouteRepoSearch).Handler(wrap(handler{h, h.serveRepoSearch}))
	r.Get(vcsclient.RouteRepoRevision).Handler(wrap(handler{h, h.serveRepoRevision}))
	r.Get(vcsclient.RouteRepoTag).Handler(wrap(handler{h, h.serveRepoTag}))
	r.Get(vcsclient.RouteRepoTags).Handler(wrap(handler{h, h.serveRepoTags}))
	r.Get(vcsclient.RouteRepoTreeEntry).Handler(wrap(handler{h, h.serveRepoTreeEntry}))

	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("date", time.Now().UTC().Format(http.TimeFormat))
	(*mux.Router)(h.router).ServeHTTP(w, r)
}

type handler struct {
	h           *Handler
	handlerFunc func(w http.ResponseWriter, r *http.Request) error
}

// handler wraps f to handle errors it returns.
func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := h.handlerFunc(w, r)
	if err != nil {
		c := errorHTTPStatusCode(err)
		h.h.Log.Printf("HTTP %d error serving %q: %s.", c, r.URL.RequestURI(), err)
		w.Header().Set("cache-control", "no-cache, max-age=0") // don't cache errors
		http.Error(w, errorBody(h.h.Debug, err), c)
	}
}

// errorBody formats an error message for the HTTP response.
func errorBody(debug bool, err error) string {
	if debug {
		data, _ := json.Marshal(&vcsclient.ErrorResponse{Message: err.Error()})
		return string(data)
	}
	return ""
}

// writeJSON writes a JSON Content-Type header and a JSON-encoded object to the
// http.ResponseWriter.
func writeJSON(w http.ResponseWriter, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return &httpError{http.StatusInternalServerError, err}
	}

	w.Header().Set("content-type", "application/json; charset=utf-8")
	_, err = w.Write(data)
	return err
}

var schemaDecoder = schema.NewDecoder()

func init() {
	schemaDecoder.RegisterConverter(vcs.CommitID(""), func(s string) reflect.Value {
		return reflect.ValueOf(vcs.CommitID(s))
	})
}

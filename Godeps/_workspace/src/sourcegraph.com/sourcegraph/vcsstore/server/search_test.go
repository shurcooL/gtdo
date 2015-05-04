package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

func TestServeRepoSearch(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	const rev = "c"

	cloneURL, _ := url.Parse("git://a.b/c")
	opt := vcs.SearchOptions{Query: "q", QueryType: "t"}

	rm := &mockSearch{
		t:   t,
		rev: rev,
		at:  vcs.CommitID(strings.Repeat("a", 40)),
		opt: opt,
		res: []*vcs.SearchResult{{File: "f", Match: []byte("abc"), StartLine: 1, EndLine: 2}},
	}
	sm := &mockServiceForExistingRepo{
		t:        t,
		vcs:      "git",
		cloneURL: cloneURL,
		repo:     rm,
	}
	testHandler.Service = sm

	resp, err := http.Get(server.URL + testHandler.router.URLToRepoSearch("git", cloneURL, rev, opt).String())
	if err != nil && !isIgnoredRedirectErr(err) {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if !sm.opened {
		t.Errorf("!opened")
	}
	if !rm.called {
		t.Errorf("!called")
	}

	var res []*vcs.SearchResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(res, rm.res) {
		t.Errorf("got res %+v, want %+v", res, rm.res)
	}
}

type mockSearch struct {
	t *testing.T

	// expected args
	rev string
	at  vcs.CommitID
	opt vcs.SearchOptions

	// return values
	res []*vcs.SearchResult
	err error

	called bool
}

func (m *mockSearch) ResolveRevision(rev string) (vcs.CommitID, error) {
	if rev != m.rev {
		m.t.Errorf("mock: got rev %q, want %q", rev, m.rev)
	}
	return m.at, nil
}

func (m *mockSearch) Search(at vcs.CommitID, opt vcs.SearchOptions) ([]*vcs.SearchResult, error) {
	if at != m.at {
		m.t.Errorf("mock: got at %q, want %q", at, m.at)
	}
	if opt != m.opt {
		m.t.Errorf("mock: got opt %+v, want %+v", opt, m.opt)
	}
	m.called = true
	return m.res, m.err
}

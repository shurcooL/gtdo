package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

func TestServeRepoBranches(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	cloneURL, _ := url.Parse("git://a.b/c")
	rm := &mockBranches{
		t:        t,
		branches: []*vcs.Branch{{Name: "t", Head: "c"}},
	}
	sm := &mockServiceForExistingRepo{
		t:        t,
		vcs:      "git",
		cloneURL: cloneURL,
		repo:     rm,
	}
	testHandler.Service = sm

	resp, err := http.Get(server.URL + testHandler.router.URLToRepoBranches("git", cloneURL).String())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if !sm.opened {
		t.Errorf("!opened")
	}
	if !rm.called {
		t.Errorf("!called")
	}

	var branches []*vcs.Branch
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(branches, rm.branches) {
		t.Errorf("got branches %+v, want %+v", branches, rm.branches)
	}
}

type mockBranches struct {
	t *testing.T

	// return values
	branches []*vcs.Branch
	err      error

	called bool
}

func (m *mockBranches) Branches() ([]*vcs.Branch, error) {
	m.called = true
	return m.branches, m.err
}

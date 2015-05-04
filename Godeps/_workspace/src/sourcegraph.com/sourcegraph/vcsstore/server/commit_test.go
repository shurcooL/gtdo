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

func TestServeRepoCommit(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	commitID := vcs.CommitID(strings.Repeat("a", 40))

	cloneURL, _ := url.Parse("git://a.b/c")
	rm := &mockGetCommit{
		t:      t,
		id:     commitID,
		commit: &vcs.Commit{ID: commitID},
	}
	sm := &mockServiceForExistingRepo{
		t:        t,
		vcs:      "git",
		cloneURL: cloneURL,
		repo:     rm,
	}
	testHandler.Service = sm

	resp, err := http.Get(server.URL + testHandler.router.URLToRepoCommit("git", cloneURL, commitID).String())
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

	var commit *vcs.Commit
	if err := json.NewDecoder(resp.Body).Decode(&commit); err != nil {
		t.Fatal(err)
	}

	normalizeCommit(rm.commit)
	if !reflect.DeepEqual(commit, rm.commit) {
		t.Errorf("got commit %+v, want %+v", commit, rm.commit)
	}

	if cc := resp.Header.Get("cache-control"); cc != longCacheControl {
		t.Errorf("got cache-control %q, want %q", cc, longCacheControl)
	}
}

func TestServeRepoCommit_RedirectToFull(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	cloneURL, _ := url.Parse("git://a.b/c")
	rm := &mockGetCommit{
		t:      t,
		id:     "ab",
		commit: &vcs.Commit{ID: "abcd"},
	}
	sm := &mockServiceForExistingRepo{
		t:        t,
		vcs:      "git",
		cloneURL: cloneURL,
		repo:     rm,
	}
	testHandler.Service = sm

	resp, err := ignoreRedirectsClient.Get(server.URL + testHandler.router.URLToRepoCommit("git", cloneURL, "ab").String())
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
	testRedirectedTo(t, resp, http.StatusFound, testHandler.router.URLToRepoCommit("git", cloneURL, "abcd"))

	if cc := resp.Header.Get("cache-control"); cc != shortCacheControl {
		t.Errorf("got cache-control %q, want %q", cc, shortCacheControl)
	}
}

// TODO(sqs): Add redirects to the full commit ID for other endpoints that
// include the commit ID.

type mockGetCommit struct {
	t *testing.T

	// expected args
	id vcs.CommitID

	// return values
	commit *vcs.Commit
	err    error

	called bool
}

func (m *mockGetCommit) GetCommit(id vcs.CommitID) (*vcs.Commit, error) {
	if id != m.id {
		m.t.Errorf("mock: got id arg %q, want %q", id, m.id)
	}
	m.called = true
	return m.commit, m.err
}

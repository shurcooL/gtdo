package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

func TestServeRepoBlameFile(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	commitID := vcs.CommitID(strings.Repeat("a", 40))

	cloneURL, _ := url.Parse("git://a.b/c")
	path := "f"
	opt := vcs.BlameOptions{NewestCommit: commitID, OldestCommit: "oc", StartLine: 1, EndLine: 2}

	rm := &mockBlameFile{
		t:     t,
		path:  path,
		opt:   opt,
		hunks: []*vcs.Hunk{{StartLine: 1, EndLine: 2, CommitID: "c"}},
	}
	sm := &mockServiceForExistingRepo{
		t:        t,
		vcs:      "git",
		cloneURL: cloneURL,
		repo:     rm,
	}
	testHandler.Service = sm

	resp, err := http.Get(server.URL + testHandler.router.URLToRepoBlameFile("git", cloneURL, path, &opt).String())
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

	var hunks []*vcs.Hunk
	if err := json.NewDecoder(resp.Body).Decode(&hunks); err != nil {
		t.Fatal(err)
	}

	for _, h := range rm.hunks {
		normalizeHunk(h)
	}
	if !reflect.DeepEqual(hunks, rm.hunks) {
		t.Errorf("got hunks %+v, want %+v", hunks, rm.hunks)
	}
}

type mockBlameFile struct {
	t *testing.T

	// expected args
	path string
	opt  vcs.BlameOptions

	// return values
	hunks []*vcs.Hunk
	err   error

	called bool
}

func (m *mockBlameFile) BlameFile(path string, opt *vcs.BlameOptions) ([]*vcs.Hunk, error) {
	if path != m.path {
		m.t.Errorf("mock: got path %q, want %q", path, m.path)
	}
	if *opt != m.opt {
		m.t.Errorf("mock: got opt %+v, want %+v", opt, m.opt)
	}
	m.called = true
	return m.hunks, m.err
}

func normalizeHunk(c *vcs.Hunk) {
	c.Author.Date = c.Author.Date.In(time.UTC)
}

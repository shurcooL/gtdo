package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"strings"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	vcs_testing "sourcegraph.com/sourcegraph/go-vcs/vcs/testing"
)

func TestServeRepoDiff(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	cloneURL, _ := url.Parse("git://a.b/c")
	opt := vcs.DiffOptions{}

	rm := &mockDiff{
		t:    t,
		base: vcs.CommitID(strings.Repeat("a", 40)),
		head: vcs.CommitID(strings.Repeat("b", 40)),
		opt:  opt,
		diff: &vcs.Diff{Raw: "diff"},
	}
	sm := &mockServiceForExistingRepo{
		t:        t,
		vcs:      "git",
		cloneURL: cloneURL,
		repo:     rm,
	}
	testHandler.Service = sm

	resp, err := http.Get(server.URL + testHandler.router.URLToRepoDiff("git", cloneURL, rm.base, rm.head, &opt).String())
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

	var diff *vcs.Diff
	if err := json.NewDecoder(resp.Body).Decode(&diff); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(diff, rm.diff) {
		t.Errorf("got diff %+v, want %+v", diff, rm.diff)
	}
}

type mockDiff struct {
	t *testing.T

	// expected args
	base, head vcs.CommitID
	opt        vcs.DiffOptions

	// return values
	diff *vcs.Diff
	err  error

	called bool
}

func (m *mockDiff) Diff(base, head vcs.CommitID, opt *vcs.DiffOptions) (*vcs.Diff, error) {
	if base != m.base {
		m.t.Errorf("mock: got base %q, want %q", base, m.base)
	}
	if head != m.head {
		m.t.Errorf("mock: got head %q, want %q", head, m.head)
	}
	if !reflect.DeepEqual(opt, &m.opt) {
		m.t.Errorf("mock: got opt %+v, want %+v", opt, &m.opt)
	}
	m.called = true
	return m.diff, m.err
}

func TestServeRepoCrossRepoDiff(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	baseCloneURL, _ := url.Parse("git://a.b/c")
	headCloneURL, _ := url.Parse("git://x.y/z")
	mockHeadRepo := vcs_testing.MockRepository{}
	opt := vcs.DiffOptions{}

	rm := &mockCrossRepoDiff{
		t:        t,
		base:     vcs.CommitID(strings.Repeat("a", 40)),
		headRepo: mockHeadRepo,
		head:     vcs.CommitID(strings.Repeat("b", 40)),
		opt:      opt,
		diff:     &vcs.Diff{Raw: "diff"},
	}
	sm := &mockService{
		t: t,
		open: func(vcs string, cloneURL *url.URL) (interface{}, error) {
			switch cloneURL.String() {
			case baseCloneURL.String():
				return rm, nil
			case headCloneURL.String():
				return mockHeadRepo, nil
			default:
				panic("unexpected repo clone: " + cloneURL.String())
			}
		},
	}
	testHandler.Service = sm

	resp, err := http.Get(server.URL + testHandler.router.URLToRepoCrossRepoDiff("git", baseCloneURL, rm.base, "git", headCloneURL, rm.head, &opt).String())
	if err != nil && !isIgnoredRedirectErr(err) {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if !rm.called {
		t.Errorf("!called")
	}

	var diff *vcs.Diff
	if err := json.NewDecoder(resp.Body).Decode(&diff); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(diff, rm.diff) {
		t.Errorf("got crossRepoDiff %+v, want %+v", diff, rm.diff)
	}
}

type mockCrossRepoDiff struct {
	t *testing.T

	// expected args
	base         vcs.CommitID
	headRepo     vcs.Repository
	headVCS      string
	headCloneURL *url.URL
	head         vcs.CommitID
	opt          vcs.DiffOptions

	// return values
	diff *vcs.Diff
	err  error

	called bool
}

func (m *mockCrossRepoDiff) CrossRepoDiff(base vcs.CommitID, headRepo vcs.Repository, head vcs.CommitID, opt *vcs.DiffOptions) (*vcs.Diff, error) {
	if base != m.base {
		m.t.Errorf("mock: got base %q, want %q", base, m.base)
	}
	if !reflect.DeepEqual(headRepo, m.headRepo) {
		m.t.Errorf("mock: got headRepo %v (%T), want %v (%T)", headRepo, headRepo, m.headRepo, m.headRepo)
	}
	if head != m.head {
		m.t.Errorf("mock: got head %q, want %q", head, m.head)
	}
	if !reflect.DeepEqual(opt, &m.opt) {
		m.t.Errorf("mock: got opt %+v, want %+v", opt, &m.opt)
	}
	m.called = true
	return m.diff, m.err
}

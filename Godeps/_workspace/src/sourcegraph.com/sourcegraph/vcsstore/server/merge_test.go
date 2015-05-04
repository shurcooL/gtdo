package server

import (
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	vcs_testing "sourcegraph.com/sourcegraph/go-vcs/vcs/testing"
)

func TestServeRepoMergeBase(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	cloneURL, _ := url.Parse("git://a.b/c")
	rm := &mockMergeBase{
		t:         t,
		a:         "a",
		b:         "b",
		mergeBase: "abcd",
	}
	sm := &mockServiceForExistingRepo{
		t:        t,
		vcs:      "git",
		cloneURL: cloneURL,
		repo:     rm,
	}
	testHandler.Service = sm

	resp, err := ignoreRedirectsClient.Get(server.URL + testHandler.router.URLToRepoMergeBase("git", cloneURL, "a", "b").String())
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
}

type mockMergeBase struct {
	t *testing.T

	// expected args
	a, b vcs.CommitID

	// return values
	mergeBase vcs.CommitID
	err       error

	called bool
}

func (m *mockMergeBase) MergeBase(a, b vcs.CommitID) (vcs.CommitID, error) {
	if a != m.a {
		m.t.Errorf("mock: got a == %q, want %q", a, m.a)
	}
	if b != m.b {
		m.t.Errorf("mock: got b == %q, want %q", b, m.b)
	}
	m.called = true
	return m.mergeBase, m.err
}

func TestServeRepoCrossRepoMergeBase(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	aCloneURL, _ := url.Parse("git://a.b/c")
	bCloneURL, _ := url.Parse("git://x.y/z")
	mockRepoB := vcs_testing.MockRepository{}

	rm := &mockCrossRepoMergeBase{
		t:         t,
		a:         "a",
		repoB:     mockRepoB,
		b:         "b",
		mergeBase: "abcd",
	}
	sm := &mockService{
		t: t,
		open: func(vcs string, cloneURL *url.URL) (interface{}, error) {
			switch cloneURL.String() {
			case aCloneURL.String():
				return rm, nil
			case bCloneURL.String():
				return mockRepoB, nil
			default:
				panic("unexpected repo clone: " + cloneURL.String())
			}
		},
	}
	testHandler.Service = sm

	resp, err := ignoreRedirectsClient.Get(server.URL + testHandler.router.URLToRepoCrossRepoMergeBase("git", aCloneURL, "a", "git", bCloneURL, "b").String())
	if err != nil && !isIgnoredRedirectErr(err) {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if !rm.called {
		t.Errorf("!called")
	}
	testRedirectedTo(t, resp, http.StatusFound, testHandler.router.URLToRepoCommit("git", aCloneURL, "abcd"))
}

type mockCrossRepoMergeBase struct {
	t *testing.T

	// expected args
	a, b  vcs.CommitID
	repoB vcs.Repository

	// return values
	mergeBase vcs.CommitID
	err       error

	called bool
}

func (m *mockCrossRepoMergeBase) CrossRepoMergeBase(a vcs.CommitID, repoB vcs.Repository, b vcs.CommitID) (vcs.CommitID, error) {
	if a != m.a {
		m.t.Errorf("mock: got a == %q, want %q", a, m.a)
	}
	if !reflect.DeepEqual(repoB, m.repoB) {
		m.t.Errorf("mock: got repoB %v (%T), want %v (%T)", repoB, repoB, m.repoB, m.repoB)
	}
	if b != m.b {
		m.t.Errorf("mock: got b == %q, want %q", b, m.b)
	}
	m.called = true
	return m.mergeBase, m.err
}

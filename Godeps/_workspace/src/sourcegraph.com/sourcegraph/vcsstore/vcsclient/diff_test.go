package vcsclient

import (
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

func TestRepository_Diff(t *testing.T) {
	setup()
	defer teardown()

	cloneURL, _ := url.Parse("git://a.b/c")
	repo_, _ := vcsclient.Repository("git", cloneURL)
	repo := repo_.(*repository)

	want := &vcs.Diff{Raw: "diff"}

	var called bool
	mux.HandleFunc(urlPath(t, RouteRepoDiff, repo, map[string]string{"VCS": "git", "CloneURL": cloneURL.String(), "Base": "b", "Head": "h"}), func(w http.ResponseWriter, r *http.Request) {
		called = true
		testMethod(t, r, "GET")

		writeJSON(w, want)
	})

	diff, err := repo.Diff("b", "h", nil)
	if err != nil {
		t.Errorf("Repository.Diff returned error: %v", err)
	}

	if !called {
		t.Fatal("!called")
	}

	if !reflect.DeepEqual(diff, want) {
		t.Errorf("Repository.Diff returned %+v, want %+v", diff, want)
	}
}

func TestRepository_CrossRepoDiff(t *testing.T) {
	setup()
	defer teardown()

	cloneURL, _ := url.Parse("git://a.b/c")
	repo_, _ := vcsclient.Repository("git", cloneURL)
	repo := repo_.(*repository)

	want := &vcs.Diff{Raw: "diff"}

	var called bool
	mux.HandleFunc(urlPath(t, RouteRepoCrossRepoDiff, repo, map[string]string{"VCS": "git", "CloneURL": cloneURL.String(), "Base": "b", "HeadVCS": "git", "HeadCloneURL": "https://x.com/y", "Head": "h"}), func(w http.ResponseWriter, r *http.Request) {
		called = true
		testMethod(t, r, "GET")

		writeJSON(w, want)
	})

	headCloneURL, _ := url.Parse("https://x.com/y")
	headRepo, _ := vcsclient.Repository("git", headCloneURL)

	diff, err := repo.CrossRepoDiff("b", headRepo, "h", nil)
	if err != nil {
		t.Errorf("Repository.CrossRepoDiff returned error: %v", err)
	}

	if !called {
		t.Fatal("!called")
	}

	if !reflect.DeepEqual(diff, want) {
		t.Errorf("Repository.CrossRepoDiff returned %+v, want %+v", diff, want)
	}
}

package vcsclient

import (
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

func TestRepository_BlameFile(t *testing.T) {
	setup()
	defer teardown()

	cloneURL, _ := url.Parse("git://a.b/c")
	repo_, _ := vcsclient.Repository("git", cloneURL)
	repo := repo_.(*repository)

	want := []*vcs.Hunk{{StartLine: 1, EndLine: 2, CommitID: "c"}}
	normalizeTime(&want[0].Author.Date)

	var called bool
	mux.HandleFunc(urlPath(t, RouteRepoBlameFile, repo, map[string]string{"VCS": "git", "CloneURL": cloneURL.String(), "Path": "f"}), func(w http.ResponseWriter, r *http.Request) {
		called = true
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"NewestCommit": "nc", "OldestCommit": "oc", "StartLine": "1", "EndLine": "2"})

		writeJSON(w, want)
	})

	hunks, err := repo.BlameFile("f", &vcs.BlameOptions{NewestCommit: "nc", OldestCommit: "oc", StartLine: 1, EndLine: 2})
	if err != nil {
		t.Errorf("Repository.Blame returned error: %v", err)
	}

	if !called {
		t.Fatal("!called")
	}

	if !reflect.DeepEqual(hunks, want) {
		t.Errorf("Repository.BlameFile returned %+v, want %+v", hunks, want)
	}
}

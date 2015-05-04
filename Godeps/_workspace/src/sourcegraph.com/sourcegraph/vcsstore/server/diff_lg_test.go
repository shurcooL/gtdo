// +build lgtest

package server

import (
	"io/ioutil"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"

	"sync"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	"sourcegraph.com/sourcegraph/vcsstore"
	"sourcegraph.com/sourcegraph/vcsstore/vcsclient"
)

func TestCrossRepoDiff_git_git_lg(t *testing.T) {
	t.Parallel()

	storageDir, err := ioutil.TempDir("", "vcsstore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(storageDir)

	conf := &vcsstore.Config{
		StorageDir: storageDir,
		Log:        log.New(os.Stderr, "", 0),
		DebugLog:   log.New(os.Stderr, "", log.LstdFlags),
	}

	h := NewHandler(vcsstore.NewService(conf), nil, nil)
	h.Log = log.New(os.Stderr, "", 0)
	h.Debug = true

	srv := httptest.NewServer(h)
	defer srv.Close()

	baseURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	c := vcsclient.New(baseURL, nil)

	baseRepo := openAndCloneRepo(t, c, "git", "https://github.com/sgtest/vcsstore-cross-repo-diff-test.git")
	headRepo := openAndCloneRepo(t, c, "git", "https://github.com/sqs/vcsstore-cross-repo-diff-test.git")

	const (
		baseCommit = "e7b2d6b444232fb1174fdd7561c25e94b0f62b60"
		headCommit = "843e1b8483f1542eeab08990b528608f5b318960"
	)
	want := &vcs.Diff{Raw: `diff --git f f
index 78981922613b2afb6025042ff6bd878ac1994e85..422c2b7ab3b3c668038da977e4e93a5fc623169c 100644
--- f
+++ f
@@ -1 +1,2 @@
 a
+b
`}

	// Run this a lot to ferret out concurrency issues.
	const n = 5000
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			diff, err := baseRepo.(vcs.CrossRepoDiffer).CrossRepoDiff(baseCommit, headRepo, headCommit, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(diff, want) {
				t.Errorf("CrossRepoDiff: got %v, want %v", diff, want)
			}
		}()
	}
	wg.Wait()
}

func openAndCloneRepo(t *testing.T, c *vcsclient.Client, vcsType, urlStr string) vcs.Repository {
	url, err := url.Parse(urlStr)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := c.Repository(vcsType, url)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.(vcsclient.RepositoryCloneUpdater).CloneOrUpdate(vcs.RemoteOpts{}); err != nil {
		t.Fatal(err)
	}
	return repo
}

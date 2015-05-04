package vcsstore

import (
	"net/url"
	"testing"
)

func TestEncodeAndDecodeRepositoryPath(t *testing.T) {
	repos := []struct {
		vcsType     string
		cloneURLStr string
		want        string
	}{
		{"git", "git://foo.com/bar/baz.git", "git/git/foo.com/bar/baz.git"},
		{"git", "ssh://git@github.com/sourcegraph/go-sourcegraph.git", "git/ssh/git@github.com/sourcegraph/go-sourcegraph.git"},
	}
	for _, repo := range repos {
		cloneURL, err := url.Parse(repo.cloneURLStr)
		if err != nil {
			t.Fatal(err)
		}

		encPath := EncodeRepositoryPath(repo.vcsType, cloneURL)

		if encPath != repo.want {
			t.Errorf("got encoded path == %q, want %q", encPath, repo.want)
		}

		vcsType, cloneURL2, err := DecodeRepositoryPath(encPath)
		if err != nil {
			t.Errorf("decodeRepoPath(%q): %s", encPath, err)
			continue
		}
		if vcsType != repo.vcsType {
			t.Errorf("got vcsType == %q, want %q", vcsType, repo.vcsType)
		}
		if cloneURL2.String() != repo.cloneURLStr {
			t.Errorf("got cloneURL == %q, want %q", cloneURL2, repo.cloneURLStr)
		}
	}
}

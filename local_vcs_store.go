package main

import (
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

// vs is the VCS store being used, if any.
var vs *localVCSStore

// localVCSStore is a local VCS store. It allows cloning, accessing, and updating repos on disk.
type localVCSStore struct {
	// dir is the root dir of the store. All repos are kept inside.
	dir string
}

// Repository opens the specified repo, cloning it if it doesn't already exist.
func (c *localVCSStore) Repository(vcsType string, cloneURL *url.URL, vcsPassword string) (_ vcs.Repository, repoDir string, _ error) {
	repoDir = filepath.Join(c.dir, vcsType, cloneURL.Scheme, filepath.FromSlash(pathpkg.Join(cloneURL.Host, cloneURL.Path)))
	repo, err := vcs.Open(vcsType, repoDir)
	if os.IsNotExist(err) {
		opt := vcs.CloneOpt{Bare: true, Mirror: true, RemoteOpts: vcs.RemoteOpts{HTTPS: &vcs.HTTPSConfig{Pass: vcsPassword}}}
		repo, err = vcs.Clone(vcsType, cloneURL.String(), repoDir, opt)
		if err != nil {
			os.RemoveAll(repoDir)
		}
	}
	return repo, repoDir, err
}

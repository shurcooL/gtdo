// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hgo provides read access to Mercurial repositories.
package hgo

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/beyang/hgo/store"
)

// http://mercurial.selenic.com/wiki/FileFormats

// Find the root of a project, given the name of a file or directory
// anywhere within the project's directory tree.
func FindProjectRoot(orig string) (root string, err error) {
	isRel := !filepath.IsAbs(orig)

	origAbs, err := filepath.Abs(orig)
	if err != nil {
		return
	}
	path := origAbs

	// if path is not a directory, skip the last part; (ignore an error,
	// because it is not important whether `path' exists at this place)
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		path = filepath.Dir(path)
	}

	for {
		if fi, err1 := os.Stat(filepath.Join(path, ".hg")); err1 == nil && fi.IsDir() {
			break
		}

		old := path
		path = filepath.Clean(filepath.Join(path, ".."))
		if path == old {
			err = errors.New("no repository found")
			return
		}
	}

	// found
	root = path
	if isRel {
		// if the original path was a relative one, try to make
		// `root' relative to the current working directory again
		if wd, err1 := os.Getwd(); err1 == nil {
			if r, err1 := filepath.Rel(wd, path); err1 == nil {
				root = r
			}
		}
	}
	return
}

type Repository struct {
	root     string
	requires map[string]bool
}

// Open a repository located at the given project root directory,
// i.e. a directory that contains a subdirectory named ‘.hg’.
func OpenRepository(root string) (r *Repository, err error) {
	var t Repository
	t.root = root
	f, err := t.open(".hg/requires")
	if err != nil {
		return
	}
	t.requires, err = parseRequires(f)
	f.Close()
	if err != nil {
		return
	}
	r = &t
	return
}

// For a given absolute or relative file name, compute a name relative
// to the repository's root.
func (r *Repository) RelFileName(name string) (rel string, err error) {
	absName, err := filepath.Abs(name)
	if err != nil {
		return
	}
	absRoot, err := filepath.Abs(r.root)
	if err != nil {
		return
	}
	rel, err = filepath.Rel(absRoot, absName)
	if err != nil {
		return
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == ".." || strings.HasPrefix(rel, "../") {
		rel = filepath.ToSlash(name)
	}
	return
}

// NewStore returns a new Store instance that provides
// access to the repository's changelog, manifests, and filelogs.
func (r *Repository) NewStore() *store.Store {
	return store.New(r.root, r.requires)
}

func (r *Repository) open(name string) (*os.File, error) {
	return os.Open(filepath.Join(r.root, filepath.FromSlash(name)))
}

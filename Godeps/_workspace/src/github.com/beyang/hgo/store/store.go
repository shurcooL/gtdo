// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package store provides access to Mercurial's ‘store’  repository format.
package store

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/beyang/hgo/revlog"
)

type Store struct {
	root string
	fe   *filenameEncoder
}

func New(repoRoot string, requires map[string]bool) (s *Store) {
	s = new(Store)
	s.root = filepath.Join(repoRoot, ".hg", "store")
	s.fe = newFilenameEncoder(requires)
	return
}

func (s *Store) OpenRevlog(fileName string) (*revlog.Index, error) {
	i, d := s.fe.Encode("data/" + fileName)
	return revlog.Open(&encodedName{s.root, i, d})
}

type Manifests struct {
	*revlog.Index
}

func (s *Store) OpenManifests() (m *Manifests, err error) {
	const name = "00manifest"

	c, err := revlog.Open(&encodedName{s.root, name, name})
	if err == nil {
		m = &Manifests{c}
	}
	return
}

type Manifest []ManifestEnt

func (m *Manifests) LookupRevision(linkrev int, wantId revlog.NodeId) (r *revlog.Rec, err error) {
	r, err = revlog.FileRevSpec(linkrev).Lookup(m.Index)
	if err != nil {
		r = m.Tip()
		err = nil
	}
	for int(r.Linkrev) > linkrev {
		r = r.Prev()
	}
	if !wantId.Eq(r.Id()) {
		err = errors.New("manifest node id does not match changelog entry")
	}
	return
}

func BuildManifest(r *revlog.Rec, fb *revlog.FileBuilder) (m Manifest, err error) {
	err = fb.BuildWrite(nil, r)
	if err != nil {
		return
	}

	m, err = ParseManifestData(fb.Bytes())
	return
}

// Create a map with filename keys from a list of manifest entries.
func (list Manifest) Map() (m map[string]*ManifestEnt) {
	m = make(map[string]*ManifestEnt, len(list))
	for i, e := range list {
		m[e.FileName] = &list[i]
	}
	return
}

type ManifestEnt struct {
	FileName string
	hash     string
}

func (e *ManifestEnt) value() (hash, opt string) {
	hash = e.hash
	if n := len(hash); n%2 == 1 {
		n--
		opt = hash[n:]
		hash = hash[:n]
	}
	return
}

func (e *ManifestEnt) IsLink() bool {
	_, o := e.value()
	return o == "l"
}

func (e *ManifestEnt) IsExecutable() bool {
	_, o := e.value()
	return o == "x"
}
func (e *ManifestEnt) Id() (revlog.NodeId, error) {
	hash, _ := e.value()
	return revlog.NewId(hash)
}

func ParseManifestData(data []byte) (m Manifest, err error) {
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.SplitN(line, "\000", 2)
		if len(f) != 2 {
			continue
		}
		m = append(m, ManifestEnt{f[0], f[1]})
	}
	return
}

func (s *Store) OpenChangeLog() (*revlog.Index, error) {
	const name = "00changelog"
	return revlog.Open(&encodedName{s.root, name, name})
}

type encodedName struct {
	root      string
	indexPart string
	dataPart  string
}

func (e *encodedName) Index() string {
	s := filepath.FromSlash(e.indexPart)
	return filepath.Join(e.root, s+".i")
}

func (e *encodedName) Data() string {
	s := filepath.FromSlash(e.dataPart)
	return filepath.Join(e.root, s+".d")
}

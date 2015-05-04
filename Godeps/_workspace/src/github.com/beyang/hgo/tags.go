// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hgo

import (
	"bytes"
	"io"
	"io/ioutil"
	"sort"
	"strings"

	"github.com/beyang/hgo/revlog"
)

// Tags contains a mapping from tag names to changeset IDs,
// and a mapping from changesetIDs to slices of tag names.
type Tags struct {
	IdByName map[string]string
	ById     map[string][]string
}

func newTags() *Tags {
	return &Tags{
		ById:     map[string][]string{},
		IdByName: map[string]string{},
	}
}

func (dest *Tags) copy(src *Tags) {
	for k, v := range src.ById {
		dest.ById[k] = v
	}
	for k, v := range src.IdByName {
		dest.IdByName[k] = v
	}
}

// Parse tags, and return one Tags structure containing only
// global tags, another one containing both global and local tags.
func (r *Repository) Tags() (tGlobal, tAll *Tags) {
	tGlobal, tAll = newTags(), newTags()

	st := r.NewStore()
	index, err := st.OpenRevlog(".hgtags")
	if err == nil {
		fb := revlog.NewFileBuilder()
		if err := fb.BuildWrite(nil, index.Tip()); err == nil {
			tAll.parseFile(bytes.NewReader(fb.Bytes()))
		}
	}

	f, err := r.open(".hg/localtags")
	if err == nil {
		tGlobal.copy(tAll)
		err = tAll.parseFile(f)
		f.Close()
	} else {
		tGlobal = tAll
	}
	return
}

func (t *Tags) parseFile(r io.Reader) (err error) {
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return
	}

	lines := strings.Split(string(buf), "\n")
	m := make(map[string]string, len(lines))

	for _, line := range lines {
		tag := strings.SplitN(strings.TrimSpace(line), " ", 2)
		if len(tag) != 2 {
			continue
		}
		m[tag[1]] = tag[0]
	}
	// unify
	for name, id := range m {
		t.ById[id] = append(t.ById[id], name)
		t.IdByName[name] = id
	}
	return
}

// Associate a new tag with a changeset ID.
func (t *Tags) Add(name, id string) {
	t.ById[id] = append(t.ById[id], name)
	t.IdByName[name] = id
}

// For each changeset ID within the ById member, sort the tag names
// associated with it in increasing order. This function should be called
// if one or more tags have been inserted using Add.
func (t *Tags) Sort() {
	for _, v := range t.ById {
		switch len(v) {
		case 1:
		case 2:
			if v[0] > v[1] {
				v[0], v[1] = v[1], v[0]
			}
		default:
			sort.Strings(v)
		}
	}
}

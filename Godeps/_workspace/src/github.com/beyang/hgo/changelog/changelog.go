// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package changelog provides read access to the changelog.
package changelog

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/beyang/hgo/revlog"
)

// http://mercurial.selenic.com/wiki/ChangelogEncodingPlan
// http://mercurial.selenic.com/wiki/ChangeSet

type Entry struct {
	Summary      string
	Comment      string
	Files        []string
	Committer    string
	Date         time.Time
	ManifestNode revlog.NodeId
	Branch       string
	Tags         []string
	*revlog.Rec
	Id string
}

type LatestTag struct {
	Names    []string
	Distance int
}

func (e *Entry) LatestTag(tags map[string][]string) (lt *LatestTag) {
	lt = new(LatestTag)
	lt.Names = []string{"null"}

	d := 0
	for r := e.Rec; r.FileRev() != -1; d++ {
		if t, ok := tags[r.Id().Node()]; ok {
			lt.Names = t
			break
		}
		r = r.Prev()
	}
	lt.Distance = d
	return
}

func BuildEntry(r *revlog.Rec, fb *revlog.FileBuilder) (e *Entry, err error) {
	err = fb.BuildWrite(nil, r)
	if err != nil {
		return
	}
	e, err = parseEntryData(fb.Bytes())
	if err == nil {
		e.Rec = r
		e.Id = r.Id().Node()
	}
	return
}

func parseEntryData(data []byte) (result *Entry, err error) {
	var c Entry

	// separate comments from other data
	s := string(data)
	if i := strings.Index(s, "\n\n"); i != -1 {
		c.Comment = strings.TrimSpace(s[i+2:])
		c.Summary = strings.TrimSpace(strings.Split(c.Comment, "\n")[0])
		s = s[:i]
	}

	// split data into lines
	f := strings.Split(s, "\n")
	if len(f) < 3 {
		err = ErrCorrupted
		return
	}

	c.ManifestNode, err = revlog.NewId(f[0])
	if err != nil {
		return
	}

	c.Committer = f[1]

	// f[2] contains date/timezone information, as well
	// as probably branch and source information
	tf := strings.SplitN(f[2], " ", 3)
	if len(tf) < 2 {
		err = ErrCorrupted
		return
	}

	c.Files = f[3:]

	// parse date/timezone
	us, err := strconv.ParseInt(tf[0], 10, 64)
	if err != nil {
		return
	}
	offset, err := strconv.Atoi(tf[1])
	if err != nil {
		return
	}
	c.Date = time.Unix(us, 0).In(time.FixedZone("", -offset))

	if len(tf) == 3 {
		c.Branch = parseMetaSection(tf[2], "\000")["branch"]
	}

	result = &c
	return
}

func parseMetaSection(text string, sep string) (m map[string]string) {
	m = make(map[string]string, 8)
	for _, s := range strings.Split(text, sep) {
		f := strings.Split(s, ":")
		if len(f) == 2 {
			m[f[0]] = strings.TrimSpace(f[1])
		}
	}
	return
}

var ErrCorrupted = errors.New("changelog corrupted")

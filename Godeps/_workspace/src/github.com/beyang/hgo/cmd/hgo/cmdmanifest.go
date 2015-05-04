// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"

	"github.com/beyang/hgo/changelog"
	"github.com/beyang/hgo/revlog"
	"github.com/beyang/hgo/store"
)

var cmdManifest = &Command{
	UsageLine: "manifest [-R dir] [-r rev]",
	Short:     "show the project's manifest",
	Long:      ``,
}

func init() {
	addStdFlags(cmdManifest)
	addRevFlag(cmdManifest)
	cmdManifest.Run = runManifest
}

func runManifest(cmd *Command, w io.Writer, args []string) {
	openRepository(args)
	rs := getRevisionSpec()
	b := revlog.NewFileBuilder()
	c, err := getChangeset(rs, b)
	if err != nil {
		fatalf("%s", err)
	}
	mm, err := getManifest(int(c.Linkrev), c.ManifestNode, b)
	if err != nil {
		fatalf("%s", err)
	}
	for i := range mm {
		fmt.Fprintln(w, mm[i].FileName)
	}
}

func getChangeset(rs revlog.RevisionSpec, b *revlog.FileBuilder) (c *changelog.Entry, err error) {
	st := repo.NewStore()
	clIndex, err := st.OpenChangeLog()
	if err != nil {
		return
	}
	r, err := rs.Lookup(clIndex)
	if err != nil {
		return
	}
	c, err = changelog.BuildEntry(r, b)
	if err == nil {
		c.Rec = r
	}
	return
}

func getManifest(linkrev int, id revlog.NodeId, b *revlog.FileBuilder) (m store.Manifest, err error) {
	st := repo.NewStore()
	mlog, err := st.OpenManifests()
	if err != nil {
		return
	}

	r, err := mlog.LookupRevision(linkrev, id)
	if err != nil {
		return
	}

	m, err = store.BuildManifest(r, b)
	return
}

// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/beyang/hgo/changelog"
	"github.com/beyang/hgo/revlog"
	"github.com/beyang/hgo/store"
)

var cmdCat = &Command{
	UsageLine: "cat [-R dir] [-r rev] [file]",
	Short:     "write the current or given revision of a file to stdout",
	Long:      ``,
}

func init() {
	addStdFlags(cmdCat)
	addRevFlag(cmdCat)
	cmdCat.Run = runCat
}

func findPresentByNodeId(ent *store.ManifestEnt, rlist []*revlog.Rec) (index int, err error) {
	wantId, err := ent.Id()
	if err != nil {
		return
	}

	for i, r := range rlist {
		if wantId.Eq(r.Id()) {
			index = i
			return
		}
	}

	err = fmt.Errorf("internal error: none of the given records matches node id %v", wantId)
	return
}

func runCat(cmd *Command, w io.Writer, args []string) {
	openRepository(args)
	rs := getRevisionSpec()
	fileArg := getFileArg(args)
	st := repo.NewStore()

	fileLog, err := st.OpenRevlog(fileArg)
	if err != nil {
		fatalf("%s", err)
	}

	ra := repoAccess{
		fb: revlog.NewFileBuilder(),
		st: st,
	}
	localId, ok := rs.(revlog.FileRevSpec)
	if !ok {
		localId, err = ra.localChangesetId(rs)
		if err != nil {
			return
		}
	}

	link := revlog.NewLinkRevSpec(int(localId))
	link.FindPresent = func(rlist []*revlog.Rec) (index int, err error) {
		if len(rlist) > 1 {
			// Does link.Rev refer to a changelog revision that is a
			// descendant of one of the revisions in rlist?
			for i, r := range rlist {
				cr, err1 := ra.clRec(revlog.FileRevSpec(r.Linkrev))
				if err1 != nil {
					err = err1
					return
				}
				if cr.IsDescendant(link.Rev) {
					index = i
					goto found
				}
			}
			err = fmt.Errorf("internal error: none of the given records is an ancestor of rev %v", link.Rev)
			return

		found:
			if !rlist[index].IsLeaf() {
				return
			}
		}

		// Check for the file's existence using the manifest.
		ent, err := ra.manifestEntry(link.Rev, fileArg)
		if err == nil {
			index, err = findPresentByNodeId(ent, rlist)
		}
		return
	}
	r, err := link.Lookup(fileLog)
	if err != nil {
		fatalf("%s", err)
	}

	fb := revlog.NewFileBuilder()
	err = fb.BuildWrite(w, r)
	if err != nil {
		fatalf("%s", err)
	}
}

type repoAccess struct {
	fb        *revlog.FileBuilder
	st        *store.Store
	changelog *revlog.Index
}

func (ra *repoAccess) manifestEntry(chgId int, fileName string) (me *store.ManifestEnt, err error) {
	r, err := ra.clRec(revlog.FileRevSpec(chgId))
	if err != nil {
		return
	}
	c, err := changelog.BuildEntry(r, ra.fb)
	if err != nil {
		return
	}
	m, err := getManifest(int(c.Linkrev), c.ManifestNode, ra.fb)
	if err != nil {
		return
	}
	me = m.Map()[fileName]
	if me == nil {
		err = errors.New("file does not exist in given revision")
	}
	return
}

func (ra *repoAccess) localChangesetId(rs revlog.RevisionSpec) (chgId revlog.FileRevSpec, err error) {
	r, err := ra.clRec(rs)
	if err == nil {
		chgId = revlog.FileRevSpec(r.FileRev())
	}
	return
}

func (ra *repoAccess) clRec(rs revlog.RevisionSpec) (r *revlog.Rec, err error) {
	if ra.changelog == nil {
		log, err1 := ra.st.OpenChangeLog()
		if err1 != nil {
			err = err1
			return
		}
		ra.changelog = log
	}
	r, err = rs.Lookup(ra.changelog)
	return
}

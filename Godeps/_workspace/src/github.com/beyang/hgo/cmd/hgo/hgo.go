// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// An example program that aims to imitate a subset of Mercurial's sub-commands.
package main

import (
	"strconv"
	"strings"

	"github.com/beyang/hgo"
	"github.com/beyang/hgo/revlog"
)

var (
	verbose  bool
	repoRoot string
	revSpec  string

	repo                *hgo.Repository
	globalTags, allTags *hgo.Tags
	branchHeads         *hgo.BranchHeads
)

func addStdFlags(cmd *Command) {
	cmd.Flag.BoolVar(&verbose, "v", false, "verbose output")
	cmd.Flag.StringVar(&repoRoot, "R", "", "The root directory of a project, containing the .hgo subdirectory")
}

func addRevFlag(cmd *Command) {
	cmd.Flag.StringVar(&revSpec, "r", "", "a specific revision")
}

func openRepository(args []string) {
	if repoRoot == "" {
		arg0 := "."
		if len(args) > 0 {
			arg0 = args[0]
		}
		r, err := hgo.FindProjectRoot(arg0)
		if err != nil {
			fatalf("%s", err)
		}
		repoRoot = r
	}
	r, err := hgo.OpenRepository(repoRoot)
	if err != nil {
		fatalf("%s", err)
	}
	repo = r

	globalTags, allTags = repo.Tags()
	globalTags.Sort()
	allTags.Sort()

	branchHeads, err = repo.BranchHeads()
	if err != nil {
		fatalf("%s", err)
	}

	return
}

func getRevisionSpec() revlog.RevisionSpec {
	return parseRevisionSpec(revSpec, "tip")
}

func getRevisionRangeSpec() (first, last revlog.RevisionSpec) {
	s := revSpec
	if s == "" {
		s = "tip:0"
	}

	f := strings.SplitN(s, ":", 2)
	switch len(f) {
	case 1:
		first = parseRevisionSpec(f[0], "null")
	case 2:
		first = parseRevisionSpec(f[0], "0")
		last = parseRevisionSpec(f[1], "tip")
	default:
		fatalf("too many fields in revision spec")
	}
	return
}

func parseRevisionSpec(s, dflt string) revlog.RevisionSpec {
	if s == "" {
		s = dflt
	}
	if s == "tip" {
		return revlog.TipRevSpec{}
	}
	if s == "null" {
		return revlog.NullRevSpec{}
	}
	if id, ok := allTags.IdByName[s]; ok {
		s = id
	} else if id, ok := branchHeads.IdByName[s]; ok {
		s = id
	} else if i, err := strconv.Atoi(s); err == nil {
		return revlog.FileRevSpec(i)
	}

	return revlog.NodeIdRevSpec(s)
}

func getRecord(i *revlog.Index, rs revlog.RevisionSpec) (r *revlog.Rec) {
	r, err := rs.Lookup(i)
	if err != nil {
		fatalf("%s", err)
	}
	return
}

func getFileArg(args []string) (fileName string) {
	if len(args) == 0 {
		return
	}
	fileName, err := repo.RelFileName(args[0])
	if err != nil {
		fatalf("%s", err)
	}
	return
}

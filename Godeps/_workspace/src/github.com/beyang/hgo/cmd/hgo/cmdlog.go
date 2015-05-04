// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"io"
	"strings"
	"text/template"

	"github.com/beyang/hgo/changelog"
	"github.com/beyang/hgo/revlog"
)

var cmdLog = &Command{
	UsageLine: "log [-R dir] [-r rev] [-l n] [-v] [file]",
	Short:     "list changeset information",
	Long:      ``,
}

func init() {
	addStdFlags(cmdLog)
	addRevFlag(cmdLog)
	cmdLog.Run = runLog
}

var logL = cmdLog.Flag.Int("l", 0, "list at most n changesets")

const logTemplate = `{{range .}}changeset:   {{.FileRev}}:{{.Id|short}}
{{with .Branch}}branch:      {{.}}
{{end}}{{range index (tags) .Id}}tag:         {{.}}
{{end}}{{if .Parent1NotPrevious}}parent:      {{.Parent.FileRev}}:{{.Parent.Id}}
{{end}}{{if .Parent2Present}}parent:      {{.Parent2.FileRev}}:{{.Parent2.Id}}
{{end}}user:        {{.Committer}}
date:        {{.Date.Format "Mon Jan 02 15:04:05 2006 -0700"}}
{{if verbose}}{{with .Files}}files:      {{range $_, $i := .}} {{.}}{{end}}
{{end}}description:
{{.Comment}}

{{else}}summary:     {{.Summary}}
{{end}}
{{end}}`

func runLog(cmd *Command, w io.Writer, args []string) {
	openRepository(args)
	rsFrom, rsTo := getRevisionRangeSpec()
	fileArg := getFileArg(args)

	st := repo.NewStore()
	clIndex, err := st.OpenChangeLog()
	if err != nil {
		fatalf("%s", err)
	}

	allTags.Add("tip", clIndex.Tip().Id().Node())

	rFrom := getRecord(clIndex, rsFrom)
	rTo := rFrom
	if rsTo != nil {
		rTo = getRecord(clIndex, rsTo)
	}
	var logIndex *revlog.Index
	wantFile := ""
	wantDir := ""
	if fileArg != "" && rTo != rFrom {
		if fileIndex, err1 := st.OpenRevlog(fileArg); err1 != nil {
			wantDir = fileArg + "/"
		} else {
			if rFrom.FileRev() > rTo.FileRev() {
				rTo, rFrom = mapLinkrevToFilerevRange(fileIndex, rTo, rFrom)
			} else {
				rFrom, rTo = mapLinkrevToFilerevRange(fileIndex, rFrom, rTo)
			}
			if rFrom == nil && rTo == nil {
				// range is empty
				return
			}
			logIndex = clIndex
		}
	} else {
		wantFile = fileArg
	}

	err = printChangelog(w, rFrom, rTo, logIndex, allTags.ById, wantDir, wantFile)
	if err != nil {
		fatalf("%s", err)
	}
}

func printChangelog(w io.Writer, rFrom, rTo *revlog.Rec, logIndex *revlog.Index, tags map[string][]string, wantDirPrefix, wantFile string) (err error) {
	var clr *revlog.Rec

	match := func([]string) bool { return true }
	if wantFile != "" {
		match = func(files []string) (ok bool) {
			for _, f := range files {
				if f == wantFile {
					ok = true
					return
				}
			}
			return
		}
	} else if wantDirPrefix != "" {
		match = func(files []string) (ok bool) {
			for _, f := range files {
				if strings.HasPrefix(f, wantDirPrefix) {
					ok = true
					return
				}
			}
			return
		}
	}

	fb := revlog.NewFileBuilder()
	fb.SetDataCache(&dc)
	fb.KeepDataOpen()
	defer fb.CloseData()
	t, err := setupLogTemplate(logTemplate, tags)
	if err != nil {
		return
	}
	ch := make(chan *changelog.Entry, 16)
	errch := make(chan error, 0)
	go func() {
		errch <- t.Execute(w, ch)
	}()
	r := rFrom
	target := rTo.FileRev()
	var next func()
	if rFrom.FileRev() > target {
		next = func() { r = r.Prev() }
	} else {
		next = func() { r = r.Next() }
	}
	i := 0
	for {
		if logIndex == nil {
			clr = r
		} else {
			clr, err = revlog.FileRevSpec(r.Linkrev).Lookup(logIndex)
			if err != nil {
				return
			}
		}
		c, err1 := changelog.BuildEntry(clr, fb)
		if err1 != nil {
			err = err1
			return
		}
		c.Rec = clr

		if match(c.Files) {
			select {
			case ch <- c:
				i++
			case err = <-errch:
				return
			}
		}
		if r.FileRev() == target {
			break
		}
		next()
		if r.FileRev() == -1 {
			break
		}
		if *logL != 0 && i == *logL {
			break
		}
	}
	close(ch)
	err = <-errch
	return
}

func setupLogTemplate(tpl string, tags map[string][]string) (*template.Template, error) {
	t := template.New("logentry")
	t.Funcs(template.FuncMap{
		"verbose": func() bool {
			return verbose
		},
		"short": func(s string) string {
			if len(s) > 12 {
				return s[:12]
			}
			return s
		},
		"tags": func() map[string][]string {
			return tags
		},
	})
	return t.Parse(tpl)
}

func mapLinkrevToFilerevRange(i *revlog.Index, rLo, rHi *revlog.Rec) (r1, r2 *revlog.Rec) {
	r1 = getRecord(i, revlog.NewLinkRevSpec(int(rLo.Linkrev)))
	r2 = getRecord(i, revlog.NewLinkRevSpec(int(rHi.Linkrev)))
	if r1.Linkrev < rLo.Linkrev {
		if r1.Linkrev == r2.Linkrev {
			goto emptyRange
		}
		r1 = r1.Next()
	}
	if r1.Linkrev == r2.Linkrev && r2.Linkrev < rHi.Linkrev {
		goto emptyRange
	}
	return

emptyRange:
	r1 = nil
	r2 = nil
	return
}

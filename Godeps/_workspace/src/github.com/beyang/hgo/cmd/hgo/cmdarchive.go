// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"strings"
	"time"

	"github.com/beyang/hgo/changelog"
	"github.com/beyang/hgo/revlog"
	"github.com/beyang/hgo/store"
)

var cmdArchive = &Command{
	UsageLine: "archive [-R dir] [-r rev] [-t type] [dest]",
	Short:     "extract all files of the project at a certain revision",
	Long:      ``,
}

var archiveType = cmdArchive.Flag.String("t", "files", "type of archive to be created")

const archTemplate = `repo: {{(.Index.Record 0).Id.Node}}
node: {{.Id}}
branch: {{if .Branch}}{{.Branch}}{{else}}default{{end}}
{{with .LatestTag (tags)}}{{range .Names}}latesttag: {{.}}
{{end}}latesttagdistance: {{.Distance}}
{{end}}`

func init() {
	addStdFlags(cmdArchive)
	addRevFlag(cmdArchive)
	cmdArchive.Run = runArchive
}

type archiver interface {
	createFile(name string, mode int, sz int64, mTime time.Time) (io.Writer, error)
	symlink(name, target string, mTime time.Time) error
	Close() error
}

var amap = map[string]func(dest string) (archiver, string, error){
	"tar": newTarArchiver,
	"tgz": newTarGzipArchiver,
	"zip": newZipArchiver,
}

func runArchive(cmd *Command, w io.Writer, args []string) {
	openRepository(args)
	rs := getRevisionSpec()
	b := revlog.NewFileBuilder()
	c, err := getChangeset(rs, b)
	if err != nil {
		fatalf("%s", err)
	}

	var ent *store.ManifestEnt
	link := revlog.NewLinkRevSpec(int(c.Linkrev))
	link.FindPresent = func(rlist []*revlog.Rec) (index int, err error) {
		index, err = findPresentByNodeId(ent, rlist)
		return
	}

	mm, err := getManifest(link.Rev, c.ManifestNode, b)
	if err != nil {
		fatalf("%s", err)
	}
	st := repo.NewStore()

	fileArg := getFileArg(args)

	newArchive, ok := amap[*archiveType]
	if !ok {
		fatalf("unknown archive type: %s", *archiveType)
	}
	a, pathPfx, err := newArchive(fileArg)
	if err != nil {
		fatalf("%s", err)
	}

	err = createArchivalTxt(a, pathPfx, c)
	if err != nil {
		fatalf("%s", err)
	}
	pathPfx += "/"

	for i := range mm {
		ent = &mm[i]

		f, err := st.OpenRevlog(ent.FileName)
		if err != nil {
			fatalf("%s", err)
		}
		r, err := link.Lookup(f)
		if err != nil {
			fatalf("%s", err)
		}

		name := pathPfx + ent.FileName
		if ent.IsLink() {
			buf, err := b.Build(r)
			if err != nil {
				fatalf("%s", err)
			}
			err = a.symlink(name, string(buf), c.Date)
			if err != nil {
				fatalf("%s", err)
			}
		} else {
			var mode int
			if ent.IsExecutable() {
				mode = 0755
			} else {
				mode = 0644
			}
			p, err := b.PreparePatch(r)
			if err != nil {
				fatalf("%s", err)
			}

			w, err := a.createFile(name, mode, int64(r.FileLength)-int64(len(p.MetaData)), c.Date)
			if err != nil {
				fatalf("%s", err)
			}
			err = p.Apply(w)
			if err != nil {
				fatalf("%s", err)
			}
		}
	}
	err = a.Close()
	if err != nil {
		fatalf("%s", err)
	}
}

func createArchivalTxt(a archiver, pathPrefix string, c *changelog.Entry) (err error) {
	t, err := setupLogTemplate(archTemplate, globalTags.ById)
	if err != nil {
		return
	}
	var b bytes.Buffer
	err = t.Execute(&b, c)
	if err != nil {
		return
	}
	w, err := a.createFile(pathPrefix+"/.hg_archival.txt", 0644, int64(b.Len()), c.Date)
	if err != nil {
		return
	}
	_, err = b.WriteTo(w)
	return
}

type tarArchiver struct {
	*tar.Writer
	file io.WriteCloser
	h    tar.Header
}

func newTarArchiver(dest string) (a archiver, pathPfx string, err error) {
	f, err := os.Create(dest)
	if err != nil {
		return
	}
	a = &tarArchiver{Writer: tar.NewWriter(f), file: f}
	pathPfx = stripExt(dest, ".tar")
	return
}

func (a *tarArchiver) createFile(name string, mode int, sz int64, mTime time.Time) (w io.Writer, err error) {
	hdr := a.initHeader(name, mode, mTime)
	hdr.Typeflag = tar.TypeReg
	hdr.Size = sz
	w = a.Writer
	err = a.WriteHeader(hdr)
	return
}

func (a *tarArchiver) symlink(name, target string, mTime time.Time) (err error) {
	hdr := a.initHeader(name, 0777, mTime)
	hdr.Typeflag = tar.TypeSymlink
	hdr.Linkname = target
	err = a.WriteHeader(hdr)
	return
}

func (a *tarArchiver) initHeader(name string, mode int, mTime time.Time) (hdr *tar.Header) {
	a.h = tar.Header{ /*Uname: "root", Gname: "root"*/}
	hdr = &a.h
	hdr.Name = name
	hdr.ModTime = mTime
	hdr.Mode = int64(mode)
	return
}

func (a *tarArchiver) close() (err error) {
	err = a.Writer.Close()
	if err == nil {
		err = a.file.Close()
	}
	return
}

type tarGzipArchiver struct {
	tarArchiver
	zf io.Closer
}

func newTarGzipArchiver(dest string) (a archiver, pathPfx string, err error) {
	f, err := os.Create(dest)
	if err != nil {
		return
	}
	zf := gzip.NewWriter(f)
	a = &tarGzipArchiver{tarArchiver: tarArchiver{Writer: tar.NewWriter(zf), file: f}, zf: zf}
	pathPfx = stripExt(dest, ".tar.gz")
	pathPfx = stripExt(pathPfx, ".tgz")
	return
}

func (a *tarGzipArchiver) close() (err error) {
	err = a.Writer.Close()
	if err == nil {
		err = a.zf.Close()
		if err == nil {
			err = a.file.Close()
		}
	}
	return
}

type zipArchiver struct {
	*zip.Writer
	file io.WriteCloser
	h    zip.FileHeader
}

func newZipArchiver(dest string) (a archiver, pathPfx string, err error) {
	f, err := os.Create(dest)
	if err != nil {
		return
	}
	a = &zipArchiver{Writer: zip.NewWriter(f), file: f}
	pathPfx = stripExt(dest, ".zip")
	return
}

func (a *zipArchiver) createFile(name string, mode int, sz int64, mTime time.Time) (w io.Writer, err error) {
	hdr := a.initHeader(name, os.FileMode(mode), mTime)
	hdr.UncompressedSize64 = uint64(sz)
	w, err = a.CreateHeader(hdr)
	return
}

func (a *zipArchiver) symlink(name, target string, mTime time.Time) (err error) {
	hdr := a.initHeader(name, 0777|os.ModeSymlink, mTime)
	w, err := a.CreateHeader(hdr)
	if err != nil {
		return
	}
	w.Write([]byte(target))
	return
}

func (a *zipArchiver) initHeader(name string, mode os.FileMode, mTime time.Time) (hdr *zip.FileHeader) {
	hdr = new(zip.FileHeader)
	hdr.Name = name
	hdr.SetModTime(mTime)
	hdr.SetMode(os.FileMode(mode))
	hdr.Method = zip.Deflate
	return
}

func (a *zipArchiver) close() (err error) {
	err = a.Writer.Close()
	if err == nil {
		err = a.file.Close()
	}
	return
}

func stripExt(fileName, ext string) (baseName string) {
	if strings.HasSuffix(fileName, ext) {
		baseName = fileName[:len(fileName)-len(ext)]
	} else {
		baseName = fileName
	}
	return
}

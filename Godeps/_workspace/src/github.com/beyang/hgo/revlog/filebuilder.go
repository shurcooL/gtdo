// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package revlog

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/beyang/hgo/revlog/patch"
)

func (fb *FileBuilder) swap() {
	fb.w, fb.w1 = fb.w1, fb.w
}

type DataCache interface {
	Get(int) []byte
	Store(int, []byte)
}

type noCache struct{}

func (noCache) Get(int) (data []byte) { return }
func (noCache) Store(int, []byte)     {}

type FileBuilder struct {
	w, w1 *patch.Joiner

	dataCache DataCache
	data      dataHelper
	fileBuf   bytes.Buffer
	metaBuf   bytes.Buffer
}

func NewFileBuilder() (p *FileBuilder) {
	p = new(FileBuilder)
	p.w = patch.NewJoiner(600)
	p.w1 = patch.NewJoiner(600)
	p.data.tmp = bytes.NewBuffer(make([]byte, 0, 128))
	return
}

type dataHelper struct {
	file     DataReadCloser
	tmp      *bytes.Buffer
	keepOpen bool
}

func (dh *dataHelper) Open(fileName string) (file DataReadCloser, err error) {
	if dh.file != nil {
		file = dh.file
		return
	}
	file, err = os.Open(fileName)
	if err == nil {
		dh.file = file
	}
	return
}

func (dh *dataHelper) TmpBuffer() *bytes.Buffer {
	return dh.tmp
}

func (p *FileBuilder) SetDataCache(dc DataCache) {
	p.dataCache = dc
}
func (p *FileBuilder) Bytes() []byte {
	return p.fileBuf.Bytes()
}

func (p *FileBuilder) KeepDataOpen() {
	p.data.keepOpen = true
}
func (p *FileBuilder) CloseData() (err error) {
	if p.data.file != nil {
		err = p.data.file.Close()
		p.data.file = nil
	}
	return
}

func (p *FileBuilder) PreparePatch(r *Rec) (f *FilePatch, err error) {
	var prevPatch []patch.Hunk
	rsav := r
	dc := p.dataCache
	if dc == nil {
		dc = noCache{}
	}

	if !p.data.keepOpen {
		defer p.CloseData()
	}

	for {
		d := dc.Get(r.i)
		if d == nil {
			d, err = r.GetData(&p.data)
			if err != nil {
				err = fmt.Errorf("rev %d: get data: %v", r.i, err)
				return
			}
			dc.Store(r.i, d)
		}
		if r.IsBase() {
			f = new(FilePatch)
			if rsav.IsStartOfBranch() {
				if r == rsav {
					// The normal case, rsav is a base revision, the
					// complete meta header is at the top of the data
					f.MetaData = scanMetaData(d)
					f.MetaSkip = len(f.MetaData)
				} else if len(prevPatch) > 0 {
					baseMeta := scanMetaData(d)
					skipFirst := false

					prevPatch[0].Adjust(func(begin, end int, data []byte) []byte {
						if n := len(baseMeta); n > 0 && begin >= 2 && end <= n-2 {
							// A rare case: There is a meta header at the top of the
							// data of the base revision (already parsed into tmp), but
							// there's also the first patch that modifies some content of the
							// meta header. (For a more robust solution the patches
							// following the first one would need to be checked too
							// whether they are located within the original meta header.)
							// An example is:
							//	hgo revlog $GOROOT/.hg/store/data/test/fixedbugs/bug136.go.i
							//	file revision 1
							b := &p.metaBuf
							b.Reset()
							b.Write(baseMeta[:begin])
							b.Write(data)
							b.Write(baseMeta[end:])
							f.MetaSkip = n
							f.MetaData = b.Bytes()
							skipFirst = true
							return data[:0]
						}

						// Another rare case, rsav is an incremental revision, the
						// meta header is at the top of the first hunk.
						// Example: hgo revlog -r 1 $PLAN9/.hg/store/data/src/cmd/dd.c.i
						if begin > 0 {
							return data
						}
						f.MetaData = scanMetaData(data)
						return data[len(f.MetaData):]
					})

					if skipFirst {
						prevPatch = prevPatch[1:]
					}
				}
			}
			f.baseData = d
			f.patch = prevPatch
			f.rev = rsav
			f.fb = p
			return
		}
		hunks, err1 := patch.Parse(d)
		if err1 != nil {
			err = err1
			return
		}
		if prevPatch == nil {
			prevPatch = hunks
		} else {
			prevPatch = p.w.JoinPatches(hunks, prevPatch)
			p.swap()
		}
		r = r.Prev()
	}
	panic("not reached")

}

func scanMetaData(d []byte) (meta []byte) {
	if len(d) <= 4 {
		return
	}
	if d[0] != '\001' || d[1] != '\n' {
		return
	}
	if i := bytes.Index(d[2:], []byte{'\001', '\n'}); i != -1 {
		meta = d[:i+4]
	}
	return
}

func (p *FileBuilder) BuildWrite(w io.Writer, r *Rec) (err error) {
	fp, err := p.PreparePatch(r)
	if err == nil {
		err = fp.Apply(w)
	}
	return
}
func (p *FileBuilder) Build(r *Rec) (file []byte, err error) {
	fp, err := p.PreparePatch(r)
	if err == nil {
		err = fp.Apply(nil)
		if err == nil {
			file = p.Bytes()
		}
	}
	return
}

type FilePatch struct {
	fb       *FileBuilder
	rev      *Rec
	baseData []byte
	patch    []patch.Hunk

	MetaData []byte
	MetaSkip int
}

func (p *FilePatch) Apply(w io.Writer) (err error) {
	if w == nil {
		p.fb.fileBuf.Reset()
		w = &p.fb.fileBuf
	}

	r := p.rev

	h := r.Index.NewHash()
	for _, id := range sortedPair(r.Parent().Id(), r.Parent2().Id()) {
		h.Write([]byte(id))
	}

	orig := p.baseData
	skip := 0
	nAdjust := 0
	if len(p.MetaData) > 0 {
		h.Write(p.MetaData)
		skip = p.MetaSkip
		nAdjust = len(p.MetaData) - skip

	}
	n, err := patch.Apply(io.MultiWriter(h, w), orig, skip, p.patch)
	if err != nil {
		return
	}
	n += nAdjust

	if n != int(r.FileLength) {
		err = fmt.Errorf("revlog: length of computed file differs from the expected value: %d != %d", n, r.FileLength)
	} else {
		fileId := NodeId(h.Sum(nil))
		if !fileId.Eq(r.Id()) {
			err = fmt.Errorf("revlog: hash mismatch: internal error or corrupted data")
		}
	}
	return
}

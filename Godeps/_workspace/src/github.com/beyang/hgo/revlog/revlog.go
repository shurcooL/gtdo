// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package revlog provides read access to RevlogNG files.
package revlog

// http://mercurial.selenic.com/wiki/Repository
// http://mercurial.selenic.com/wiki/RevlogNG

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

var (
	byteOrder = binary.BigEndian
)

const (
	knownFlagsMask uint16 = 1
	inlineData     uint16 = 1
	versionPos     uint32 = 16
)

type Name interface {
	Index() string
	Data() string
}

type record struct {
	OffsetHi   uint16
	OffsetLo   uint32
	Flags      uint16
	DataLength uint32
	FileLength uint32
	Baserev    rev
	Linkrev    rev
	Parent1    rev
	Parent2    rev
	NodeId     NodeId
}

type rev uint32

func (r *rev) parse(buf []byte) {
	*r = rev(byteOrder.Uint32(buf))
}

func (r rev) isNull() bool {
	return r == ^rev(0)
}

func (r rev) String() string {
	if r.isNull() {
		return "null"
	}
	return strconv.FormatUint(uint64(r), 10)
}

func (r *record) offset() int64 {
	return int64(r.OffsetHi)<<32 | int64(r.OffsetLo)
}

func (r *record) decode(buf []byte, cur rev, rPrev *record) (err error) {
	if cur > 0 {
		r.OffsetHi = byteOrder.Uint16(buf[0:])
		r.OffsetLo = byteOrder.Uint32(buf[2:])
	}

	r.Flags = byteOrder.Uint16(buf[6:])

	r.DataLength = byteOrder.Uint32(buf[8:])
	r.FileLength = byteOrder.Uint32(buf[12:])

	r.Baserev.parse(buf[16:])
	r.Linkrev.parse(buf[20:])
	r.Parent1.parse(buf[24:])
	r.Parent2.parse(buf[28:])

	id := make([]byte, 32)
	copy(id, buf[32:])
	r.NodeId = id

	// sanity check:
	switch {
	case cur == 0 && r.Baserev != 0:
		err = errors.New("first record not a base record")
		return
	case cur < r.Baserev:
	case !r.Parent1.isNull() && cur <= r.Parent1:
	case !r.Parent2.isNull() && cur <= r.Parent2:
	case rPrev != nil && r.offset() < rPrev.offset():
	default:
		return
	}
	err = errors.New("revlog: corrupted record")
	return
}

func parseV1Flags(buf []byte) (flags uint16, err error) {
	offsetHi := byteOrder.Uint16(buf[0:])
	offsetLo := byteOrder.Uint32(buf[2:])
	flags = offsetHi
	if flags & ^knownFlagsMask != 0 {
		err = errors.New("unknown flags")
		return
	}
	if version := offsetLo >> versionPos; version != 1 {
		err = errors.New("unknown version")
	}
	return
}

type Rec struct {
	i int
	*record
	Index *Index
}

func (r *Rec) IsBase() bool {
	return r.i == int(r.Baserev)
}
func (r *Rec) BaseRev() int {
	return int(r.Baserev)
}
func (r *Rec) Prev() *Rec {
	if r.i == 0 {
		return &r.Index.null
	}
	return r.Index.Record(r.i - 1)
}
func (r *Rec) Next() *Rec {
	return r.Index.Record(r.i + 1)
}
func (r *Rec) Parent() *Rec {
	return r.parent(r.Parent1)
}
func (r *Rec) Parent2() *Rec {
	return r.parent(r.record.Parent2)
}

func (r *Rec) Parent1NotPrevious() bool {
	p := r.record.Parent1
	return r.Parent2Present() || int(p) != r.i-1 && !p.isNull()
}

func (r *Rec) Parent2Present() bool {
	p := r.record.Parent2
	return !p.isNull()
}

func (r *Rec) IsLeaf() (yes bool) {
	tail := r.Index.index[r.i+1:]
	for i := range tail {
		switch rev(r.i) {
		case tail[i].Parent1, tail[i].Parent2:
			return
		}
	}
	yes = true
	return
}

func (r *Rec) IsStartOfBranch() bool {
	return r.record.Parent1.isNull() && r.record.Parent2.isNull()
}

func (r *Rec) FileRev() int {
	return r.i
}
func (r *Rec) parent(idx rev) *Rec {
	if r == nil || idx.isNull() {
		return &r.Index.null
	}
	return r.Index.Record(int(idx))
}

func (r *Rec) Id() NodeId {
	return r.Index.NewNodeId(r.record.NodeId[:])
}

type DataReadCloser interface {
	io.ReaderAt
	io.Closer
}

type Index struct {
	name    Name
	version int

	flags struct {
		inlineData bool
	}
	index []record
	data  []byte
	v1nodeid
	null Rec
}

func (i *Index) Tip() *Rec {
	return i.Record(len(i.index) - 1)
}

func (i *Index) Null() *Rec {
	return &i.null
}
func Open(name Name) (rlog *Index, err error) {
	var (
		data     []byte
		idata    uint32
		inline   bool
		r, rPrev *record
	)

	f, err := os.Open(name.Index())
	if err != nil {
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return
	}

	index := make([]record, fi.Size()/64)
	i := 0
	br := bufio.NewReader(f)
	buf := make([]byte, 64)
	for ; ; i++ {
		if _, err1 := io.ReadFull(br, buf); err1 != nil {
			if err1 == io.EOF {
				break
			}
			err = err1
			return
		}
		if i == len(index) {
			old := index
			index = make([]record, 2*len(index))
			copy(index, old)
		}
		if i == 0 {
			flags, err1 := parseV1Flags(buf)
			if err1 != nil {
				err = err1
				return
			}
			inline = flags&inlineData != 0
		}
		r = &index[i]
		if err = r.decode(buf, rev(i), rPrev); err != nil {
			return
		}
		rPrev = r

		if inline {
			data = append(data, make([]byte, r.DataLength)...)
			if _, err = io.ReadFull(br, data[idata:]); err != nil {
				return
			}
			idata += r.DataLength
		}
	}

	rlog = new(Index)
	rlog.data = data
	rlog.index = index[:i]
	rlog.name = name
	rlog.flags.inlineData = inline
	rlog.null = Rec{
		i:      -1,
		Index:  rlog,
		record: &record{NodeId: make([]byte, 32)},
	}
	return
}

func (rv *Index) Record(i int) *Rec {
	return &Rec{i, &rv.index[i], rv}
}

var ErrRevNotFound = errors.New("revision not found")

func (rv *Index) Dump(w io.Writer) {
	for i, _ := range rv.index {
		fmt.Fprintf(w, "%d:\t%v\n", i, rv.index[i])
	}
}

type DataHelper interface {
	Open(string) (DataReadCloser, error)
	TmpBuffer() *bytes.Buffer
}

func (r *Rec) GetData(dh DataHelper) (data []byte, err error) {
	if r.DataLength == 0 {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
		}
	}()

	var dataType byte
	rv := r.Index
	if rv.flags.inlineData {
		o := int(r.offset())
		data = rv.data[o : o+int(r.DataLength)]
		dataType = data[0]
	} else {
		f, err1 := dh.Open(rv.name.Data())
		if err1 != nil {
			err = err1
			return
		}
		o := r.offset()
		sr := io.NewSectionReader(f, o, int64(r.DataLength))
		b := []byte{0}
		if n, err1 := sr.ReadAt(b, 0); n != 1 {
			err = err1
			return
		}
		dataType = b[0]
		if dataType == 'x' {
			buf := dh.TmpBuffer()
			buf.Reset()
			if _, err = buf.ReadFrom(sr); err != nil {
				return
			}
			data = buf.Bytes()
		} else {
			data = make([]byte, r.DataLength)
			if _, err = io.ReadFull(sr, data); err != nil {
				data = nil
				return
			}
		}
	}
	switch dataType {
	default:
		err = errors.New("unknown data type")
	case 'u':
		data = data[1:]
	case 'x':
		zr, err1 := zlib.NewReader(bytes.NewReader(data))
		data = nil
		if err1 != nil {
			err = err1
			return
		}
		if !r.IsBase() {
			data = make([]byte, r.DataLength*2)
			n := 0
			for {
				nr, err1 := io.ReadFull(zr, data[n:])
				n += nr
				if err1 != nil {
					if err1 == io.ErrUnexpectedEOF || err1 == io.EOF {
						data = data[:n]
						break
					}
					err = err1
					return
				} else {
					data = append(data, make([]byte, cap(data))...)
				}
			}
		} else {
			data = make([]byte, r.FileLength)
			_, err = io.ReadFull(zr, data)
		}
		zr.Close()
	case 0:
	}
	return
}

// IsDescendant follows all branches that originate in r
// until it passes record rev2. If that record is found to
// be on one of these branches, it is a descendant of r.
func (r *Rec) IsDescendant(rev2 int) (is bool) {
	// A region refers to a number of adjacent records in the revlog
	// that all have r as an ancestor.
	type region struct{ from, to int }
	var regions []region
	var cur region

	insideRegion := true
	cur.from = r.FileRev()
	index := r.Index.index

	if rev2 >= len(index) {
		return
	}

L:
	for i := cur.from + 1; i <= rev2; i++ {
		// If parent1 or parent2 are found to point into
		// one of the regions, i is the index of a descendant
		// record.
		p1, p2 := index[i].Parent1, index[i].Parent2

		if insideRegion {
			if !p1.isNull() && int(p1) >= cur.from {
				continue
			}
			if !p2.isNull() && int(p2) >= cur.from {
				continue
			}
		}

		for _, r := range regions {
			switch {
			case !p1.isNull() && r.from <= int(p1) && int(p1) <= r.to:
				fallthrough
			case !p2.isNull() && r.from <= int(p2) && int(p2) <= r.to:
				if !insideRegion {
					cur.from = i
					insideRegion = true
				}
				continue L
			}
		}
		if insideRegion {
			insideRegion = false
			cur.to = i - 1
			regions = append(regions, cur)
		}
	}
	is = insideRegion
	return
}

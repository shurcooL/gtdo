// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The patch package provides support for calculating and applying revlog patches
package patch

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

var (
	byteOrder = binary.BigEndian
)

// A hunk describes how a certain region of original data
// is to be modified.
type Hunk struct {
	begin int
	end   int
	data  []byte
}

// Dump prints the information stored in a Hunk for debug purposes.
func (h *Hunk) Dump(w io.Writer) {
	Δ := len(h.data) - (h.end - h.begin)
	fmt.Fprintf(w, "@@ %d..%d %+d @@\n", h.begin, h.end, Δ)
	w.Write(h.data)
	fmt.Fprint(w, "\n")
}

func (h *Hunk) Adjust(f func(begin, end int, data []byte) []byte) {
	h.data = f(h.begin, h.end, h.data)
}

// Parse converts a patch from its binary representation into a slice of Hunks.
func Parse(data []byte) (hunks []Hunk, err error) {
	var h Hunk
	prevEnd := -1

	for len(data) > 0 {
		if len(data) < 3*4 {
			err = errors.New("corrupt hunk: less bytes than needed for hunk header")
			return
		}
		h.begin = int(byteOrder.Uint32(data[0:]))
		h.end = int(byteOrder.Uint32(data[4:]))

		if h.begin <= prevEnd {
			err = errors.New("corrupt hunk: overlapps with previous")
			return
		}
		if h.end < h.begin {
			err = errors.New("corrupt hunk: end position before beginning")
			return
		}
		prevEnd = h.end
		n := int(byteOrder.Uint32(data[8:]))
		data = data[12:]
		if len(data) < n {
			err = errors.New("patch: short read")
			return
		}
		h.data = data[:n]
		data = data[n:]
		hunks = append(hunks, h)
	}
	return
}

// A Joiner is a patch buffer that is the target of usually many
// calls to its method JoinPatches.
type Joiner struct {
	w []Hunk
}

// Creates a new Joiner. To avoid reallocations of the patch buffer
// as the patch grows, an initial size may be stated.
func NewJoiner(preAlloc int) *Joiner {
	return &Joiner{w: make([]Hunk, 0, preAlloc)}
}

func (j *Joiner) reset() {
	j.w = j.w[:0]
}

// Append a single hunk to the resulting patch, probably
// merging it with the previous hunk.
func (j *Joiner) emit(h Hunk) {
	if len(h.data) == 0 && h.begin == h.end {
		return
	}

	if n := len(j.w); n > 0 {
		p := &j.w[n-1]
		if p.end == h.begin {
			if len(h.data) == 0 {
				p.end = h.end
				return
			}
			if len(p.data) == 0 {
				p.data = h.data
				p.end = h.end
				return
			}
		}
	}

	j.w = append(j.w, h)
}

// Append all remaining hunks in h to the Joiner's patch.
func (j *Joiner) emitTail(h []Hunk, roff0, roff int) {
	adjust := roff0 != 0 || roff != 0
	for i := range h {
		if adjust {
			h[i].begin -= roff0
			h[i].end -= roff
			roff0 = roff
		}
	}
	j.w = append(j.w, h...)
}

// With ‘left’ and ‘right’ being patches of two adjacent revlog revisions,
// JoinPatches propagates hunk by hunk from the right to the left side,
// probably intersecting hunks one or more times. The resulting patch will be
// stored into the Joiner. Both right and left hunks may be altered.
func (j *Joiner) JoinPatches(left, right []Hunk) []Hunk {
	var roff, roff0 int

	// Loop over the hunks on the left side, and sort,
	// one after another, hunks from the right into the output
	// stream, probably splitting or overlapping left side hunks.

	j.reset()

	for i, lh := range left {
	again:
		if len(right) == 0 {
			// no hunk remains on the right	#1
			j.emit(lh)
			j.emitTail(left[i+1:], 0, 0)
			break
		}
		rh := right[0]
		ld := lh.end - lh.begin

		// the number of bytes lh will add
		Δlh := len(lh.data) - ld

		// translate lh's coordinates to the right side
		begin := lh.begin + roff
		end := begin + len(lh.data)

		Δfrontvis := rh.begin - begin
		switch {
		case begin >= rh.end:
			// rh comes before lh	#2
			rh.begin -= roff0
			rh.end -= roff
			j.emit(rh)
			patchStats.inserted++
			goto nextrh

		case end <= rh.begin:
			// lh comes before rh	#3
			roff += Δlh
			roff0 = roff
			j.emit(lh)

		case Δfrontvis > 0:
			// lh starts before rh		#4

			if end > rh.end {
				// rh is embedded in lh	#5

				// emit front hunk
				h := lh
				h.data = h.data[:Δfrontvis]
				j.emit(h)

				roff += rh.end - begin - (ld)

				// trim lh
				lh.begin = lh.end
				lh.data = lh.data[rh.end-begin:]

				// emit rh
				rh.begin = lh.end
				rh.end = lh.end
				j.emit(rh)

				roff0 = roff
				patchStats.over.mid++
				goto nextrh

			} else {
				// rh covers the end of the current lh	#6

				// trim lh
				lh.end = lh.begin
				lh.data = lh.data[:Δfrontvis]
				j.emit(lh)
				roff0 = roff + (rh.begin - begin)
				roff += Δlh
				patchStats.over.bottom++
			}
		case end > rh.end:
			// rh covers the beginning of current lh	#7

			// trim lh
			lh.data = lh.data[rh.end-begin:]

			// emit rh
			rh.begin -= roff0
			roff += rh.end - begin - ld
			roff0 = roff
			rh.end = lh.end
			j.emit(rh)

			lh.begin = lh.end
			patchStats.over.top++
			goto nextrh

		default:
			// rh covers lh fully	#8
			roff += Δlh
			patchStats.over.full++

		}
		continue
	nextrh:
		right = right[1:]
		roff0 = roff
		goto again
	}

	if len(right) > 0 {
		j.emitTail(right, roff0, roff)
		patchStats.inserted += len(right)
	}
	return j.w
}

var patchStats struct {
	inserted int
	over     struct {
		top, mid, bottom, full int
	}
}

// Apply a patch to a slice of original bytes, writing the output to an io.Writer.
// The number of bytes written, i.e. the size of the resulting file, is returned.
func Apply(w io.Writer, orig []byte, pos int, patch []Hunk) (n int, err error) {
	var nw int

	n = pos
	for _, h := range patch {
		pos, nw, err = h.apply(w, orig, pos)
		n += nw
		if err != nil {
			return
		}
	}
	if len(orig) < pos {
		err = &posError{"current position is behind the end of the original file", pos, 0}
		return
	}
	nw, err = w.Write(orig[pos:])
	if err != nil {
		return
	}
	n += nw
	return
}

func (h *Hunk) apply(w io.Writer, orig []byte, pos int) (newPos, n int, err error) {
	var nw int

	switch {
	case len(orig) < h.begin:
		err = &posError{"hunk starts after the end of the original file", pos, h.begin}

	case pos < 0 || len(orig) < pos:
		err = &posError{"current position is not within the boundaries of the original file", pos, 0}

	case h.begin < 0:
		err = &posError{"negative hunk start position", pos, h.begin}

	case h.begin < pos:
		err = &posError{"hunk starts before current position", pos, h.begin}

	default:
		nw, err = w.Write(orig[pos:h.begin])
		if err != nil {
			break
		}
		n += nw
		nw, err = w.Write(h.data)
		if err != nil {
			break
		}
		n += nw
		newPos = h.end
	}

	return
}

type posError struct {
	message   string
	origPos   int
	hunkBegin int
}

func (e *posError) Error() string {
	return "patch: corrupt data or internal error: " + e.message
}

// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package store

import (
	"strings"
)

// http://mercurial.selenic.com/wiki/CaseFoldingPlan
// http://mercurial.selenic.com/wiki/fncacheRepoFormat
// http://mercurial.selenic.com/wiki/fncache2RepoFormat

type filenameEncoder struct {
	buf       []byte
	fncache   bool
	dotencode bool
}

func newFilenameEncoder(requires map[string]bool) *filenameEncoder {
	return &filenameEncoder{
		dotencode: requires["dotencode"],
		fncache:   requires["fncache"],
	}
}

const hexdigits = "0123456789abcdef"

func byteToHex(hex []byte, b byte) []byte {
	return hex
}

func (e *filenameEncoder) Encode(orig string) (indexName, dataName string) {
	var modified, segMod bool
	b := e.buf

	indexName = orig
	dataName = orig
	for i, seg := range strings.Split(orig, "/") {
		if i > 0 {
			b = append(b, '/')
		}
		if b, segMod = e.encodeSegment(b, seg, false); segMod {
			modified = true
		}
	}

	e.buf = b[:0]
	if e.fncache && len(b) > 120 {
		indexName = "dh/__BUG__hash_encoded_names_not_supported"
		dataName = indexName
	} else if modified {
		indexName = string(b)
		dataName = indexName
	}
	return
}

func (e *filenameEncoder) encodeSegment(b []byte, seg string, hashPreEncode bool) (result []byte, modified bool) {
	n := len(seg)
	switch {
	case !e.fncache:
	case n > 2 && strings.HasSuffix(seg, ".d"):
		// In various repositories, like golang's, it can be seen that ".d" gets
		// translated into ".d.hg". There seems to be no documentation in
		// Mercurial's wiki about that; for the nonce we encode .d for
		// the fncache option.
		seg += ".hg"
		modified = true
		n += 3
	case n >= 3:
		// check for some names reserved on MS-Windows
		nres := 3
		switch seg[:nres] {
		case "com", "lpt":
			nres = 4
			if n <= 3 || seg[3] < '1' || seg[3] > '9' {
				break
			}
			fallthrough
		case "con", "prn", "aux", "nul":
			if n > nres && seg[nres] != '.' {
				break
			}
			b = append(b, seg[0], seg[1], '~', hexdigits[seg[2]>>4], hexdigits[seg[2]&0xf])
			result = append(b, seg[3:]...)
			modified = true
			return
		}
	}

	for i := 0; i < n; i++ {
		c := seg[i]
		switch {
		case c < ' ' || c >= '~':
			goto hex

		case i == 0 && e.dotencode && (c == ' ' || c == '.'):
			goto hex

		case c >= 'A' && c <= 'Z':
			if !hashPreEncode {
				b = append(b, '_')
			}
			c = c - 'A' + 'a'
			modified = true

		default:
			switch c {
			case '_':
				b = append(b, c)
				modified = true

			case '\\', ':', '*', '?', '"', '<', '>', '|':
				goto hex
			}
		}
		b = append(b, c)
		continue
	hex:
		modified = true
		b = append(b, '~', hexdigits[c>>4], hexdigits[c&0xf])
	}
	result = b
	return
}

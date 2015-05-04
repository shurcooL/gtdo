// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package revlog

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"hash"
)

// http://mercurial.selenic.com/wiki/Nodeid
// http://mercurial.selenic.com/wiki/ChangeSetID

type NodeId []byte

func (i NodeId) String() string {
	return hex.EncodeToString(i[:6])
}
func (i NodeId) Node() string {
	return hex.EncodeToString(i)
}

func (i NodeId) Eq(i2 NodeId) bool {
	return bytes.Equal(i, i2)
}

func NewId(hash string) (id NodeId, err error) {
	buf, err := hex.DecodeString(hash)
	if err == nil {
		id = NodeId(buf)
	}
	return
}

type NodeIdImpl interface {
	NewHash() hash.Hash
	NewNodeId([]byte) NodeId
}

type v1nodeid byte

func (id v1nodeid) NewNodeId(b []byte) NodeId {
	if len(b) > 20 {
		b = b[:20]
	}
	return b
}

func (id v1nodeid) NewHash() hash.Hash {
	return sha1.New()
}

func sortedPair(i1, i2 NodeId) []NodeId {
	switch {
	case i1 == nil && i2 == nil:
	case i1 == nil:
		i1 = make(NodeId, len(i2))
	case i2 == nil:
		i2 = make(NodeId, len(i1))
	}
	if bytes.Compare(i1, i2) > 0 {
		return []NodeId{i2, i1}
	}
	return []NodeId{i1, i2}
}

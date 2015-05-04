// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package revlog

import "errors"

type RevisionSpec interface {
	Lookup(*Index) (*Rec, error)
}

type FileRevSpec int

func (n FileRevSpec) Lookup(i *Index) (r *Rec, err error) {
	if n < 0 {
		n += FileRevSpec(len(i.index))
	}
	if n < 0 || int(n) >= len(i.index) {
		err = ErrRevisionNotFound
	} else {
		r = i.Record(int(n))
	}
	return
}

// A LinkRevSpec can be used to find a file revision
// that was present at a certain changelog revision, by
// examining the filelog records' linkrev values.
// The behaviour of the Lookup method can be configured
// by setting FindPresent to a user implemented function.
type LinkRevSpec struct {
	Rev int

	// FindPresent should examine maybeAncestors' Linkrev values to
	// find a changelog record that is an ancestor of Rev. It also has to
	// make sure that the file actually existed in the revision specified
	// by Rev.
	// If FindPresent is nil (the default), Lookup will -- in case of multiple
	// matching branches -- return the last visited record, or a Null record
	// if no revision matches at all.
	FindPresent func(maybeAncestors []*Rec) (index int, err error)
}

func NewLinkRevSpec(rev int) *LinkRevSpec {
	return &LinkRevSpec{Rev: rev}
}

func (l LinkRevSpec) Lookup(i *Index) (match *Rec, err error) {
	// While doing the range loop, vr keeps track of the
	// last visited records of all branches.
	var vr []*Rec
	branch := 0

	for j := range i.index {
		if li := int(i.index[j].Linkrev); li == int(l.Rev) {
			// exact match
			match = i.Record(j)
			return
		} else if li > int(l.Rev) {
			break
		}

		r := i.Record(j)
		if vr == nil {
			vr = append(vr, r)
			continue
		}

		// If Parent2 points to one of the last visited
		// records of all visited branches, store the
		// entries index into p2branch.
		p2branch := -1
		if r.Parent2Present() {
			p := r.Parent2().FileRev()
			for k, r := range vr {
				if r == nil {
					continue
				}
				if r.FileRev() == p {
					p2branch = k
				}
			}
		}

		// If the parent of the current record is not a member of the
		// last visited branch, look if either parent or parent2 is one of
		// the other last visited records. Else, create a new branch.
		if p := r.Parent().FileRev(); vr[branch].FileRev() != p {
			for k, r := range vr {
				if r == nil {
					continue
				}
				if r.FileRev() == p {
					branch = k
					goto found
				}
			}
			if p2branch != -1 {
				branch = p2branch
			} else {
				branch = len(vr)
				vr = append(vr, r)
			}
		found:
		}
		vr[branch] = r
		if p2branch != -1 && p2branch != branch {
			vr[p2branch] = nil
		}
	}

	// Sort out nil entries.
	w := 0
	numNilsBeforeBranch := 0
	for i := range vr {
		if vr[i] != nil {
			vr[w] = vr[i]
			w++
		} else if i < branch {
			numNilsBeforeBranch++
		}
	}
	vr = vr[:w]
	branch -= numNilsBeforeBranch

	switch len(vr) {
	case 0:
		if l.FindPresent != nil {
			match = nil
			err = ErrRevisionNotFound
			return
		}
		match = i.Null()
	default:
		if l.FindPresent != nil {
			// make sure the most recent updated entry comes first
			if branch != 0 {
				vr[0], vr[branch] = vr[branch], vr[0]
			}
			branch, err = l.FindPresent(vr)
			if err == nil {
				match = vr[branch]
			}
			return
		}
		fallthrough

	case 1:
		match = vr[branch]
		if match.IsLeaf() {
			if l.FindPresent != nil {
				_, err = l.FindPresent([]*Rec{match})
			}
		}
	}

	return
}

type NodeIdRevSpec string

func (hash NodeIdRevSpec) Lookup(rv *Index) (r *Rec, err error) {
	var i = -1
	var found bool

	wantid, err := NewId(string(hash))
	if err != nil {
		return
	}
	for j := range rv.index {
		nodeid := rv.NewNodeId(rv.index[j].NodeId[:])
		if len(wantid) <= len(nodeid) {
			if wantid.Eq(nodeid[:len(wantid)]) {
				if found {
					err = ErrRevisionAmbiguous
				}
				found = true
				i = j
			}
		}
	}
	if i == -1 {
		err = ErrRevNotFound
	} else {
		r = rv.Record(i)
	}
	return
}

type TipRevSpec struct{}

func (TipRevSpec) String() string {
	return "tip"
}

func (TipRevSpec) Lookup(i *Index) (r *Rec, err error) {
	if n := len(i.index); n == 0 {
		err = ErrRevisionNotFound
	} else {
		r = i.Record(n - 1)
	}
	return
}

type NullRevSpec struct{}

func (NullRevSpec) String() string {
	return "null"
}

func (NullRevSpec) Lookup(i *Index) (r *Rec, err error) {
	r = &i.null
	return
}

var ErrRevisionNotFound = errors.New("hg/revlog: revision not found")
var ErrRevisionAmbiguous = errors.New("hg/revlog: ambiguous revision spec")

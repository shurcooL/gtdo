// Copyright 2013 The hgo Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hgo

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

var needFlags = map[string]bool{
	"revlogv1": true,
	"store":    true,
}
var knownFlags = map[string]bool{
	"revlogv1":  true,
	"store":     true,
	"fncache":   true,
	"dotencode": true,
}

func parseRequires(r io.Reader) (m map[string]bool, err error) {
	m = make(map[string]bool, 8)
	f := bufio.NewReader(r)
	for {
		s, err1 := f.ReadString('\n')
		if err1 != nil {
			break
		}
		s = strings.TrimSpace(s)
		if !knownFlags[s] {
			err = fmt.Errorf(".hg/requires: unknown requirement: %s", s)
			return
		}
		m[s] = true
	}
	for k := range needFlags {
		if !m[k] {
			err = fmt.Errorf(".hg/requires: requirement `%s' not satisfied", k)
			return
		}
	}
	return
}

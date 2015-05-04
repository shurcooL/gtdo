package fileutil

import (
	"testing"

	"sourcegraph.com/sourcegraph/vcsstore/vcsclient"
)

func TestComputeFileRange(t *testing.T) {
	tests := map[string]struct {
		data []byte
		opt  vcsclient.GetFileOptions
		want vcsclient.FileRange
	}{
		"zero": {
			data: []byte(``),
			opt:  vcsclient.GetFileOptions{},
			want: vcsclient.FileRange{StartLine: 0, EndLine: 0},
		},
		"1 char": {
			data: []byte(`a`),
			opt:  vcsclient.GetFileOptions{},
			want: vcsclient.FileRange{StartLine: 1, EndLine: 1, EndByte: 1},
		},
		"1 line": {
			data: []byte("a\n"),
			opt:  vcsclient.GetFileOptions{},
			want: vcsclient.FileRange{StartLine: 1, EndLine: 1, EndByte: 2},
		},
		"2 lines, no trailing newline": {
			data: []byte("a\nb"),
			opt:  vcsclient.GetFileOptions{},
			want: vcsclient.FileRange{StartLine: 1, EndLine: 2, EndByte: 3},
		},
		"2 lines, trailing newline": {
			data: []byte("a\nb\n"),
			opt:  vcsclient.GetFileOptions{},
			want: vcsclient.FileRange{StartLine: 1, EndLine: 2, EndByte: 4},
		},
		"2 lines, byte range": {
			data: []byte("a\nb\n"),
			opt:  vcsclient.GetFileOptions{FileRange: vcsclient.FileRange{StartByte: 2, EndByte: 3}},
			want: vcsclient.FileRange{StartLine: 2, EndLine: 2, StartByte: 2, EndByte: 3},
		},
		"2 lines, byte range, full lines": {
			data: []byte("a\nb\n"),
			opt:  vcsclient.GetFileOptions{FileRange: vcsclient.FileRange{StartByte: 2, EndByte: 3}, FullLines: true},
			want: vcsclient.FileRange{StartLine: 2, EndLine: 2, StartByte: 2, EndByte: 4},
		},
		"2 lines, line range": {
			data: []byte("a\nb\n"),
			opt:  vcsclient.GetFileOptions{FileRange: vcsclient.FileRange{StartLine: 2, EndLine: 2}},
			want: vcsclient.FileRange{StartLine: 2, EndLine: 2, StartByte: 2, EndByte: 4},
		},
	}
	for label, test := range tests {
		got, _, err := ComputeFileRange(test.data, test.opt)
		if err != nil {
			t.Errorf("%s: ComputeFileRange error: %s", label, err)
			continue
		}

		if *got != test.want {
			t.Errorf("%s: got %+v, want %+v", label, *got, test.want)
		}
	}
}

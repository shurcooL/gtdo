package fileutil

import (
	"fmt"

	"github.com/sqs/fileset"
	"sourcegraph.com/sourcegraph/vcsstore/vcsclient"
)

// ComputeFileRange determines the actual file range according to the
// input range parameter. For example, if input has a line range set,
// the returned FileRange will contain the byte range that corresponds
// to the input line range.
func ComputeFileRange(data []byte, opt vcsclient.GetFileOptions) (*vcsclient.FileRange, *fileset.File, error) {
	fr := opt.FileRange // alias for brevity

	fset := fileset.NewFileSet()
	f := fset.AddFile("", 1, len(data))
	f.SetLinesForContent(data)

	if opt.EntireFile || (fr.StartLine == 0 && fr.EndLine == 0 && fr.StartByte == 0 && fr.EndByte == 0) {
		fr.StartLine, fr.EndLine = 0, 0
		fr.StartByte, fr.EndByte = 0, len(data)
	}

	lines := fr.StartLine != 0 || fr.EndLine != 0
	bytes := fr.StartByte != 0 || fr.EndByte != 0
	if lines && bytes {
		return nil, nil, fmt.Errorf("must specify a line range OR a byte range, not both (%+v)", fr)
	}

	if lines {
		// Given line range, validate it and return byte range.
		if fr.StartLine == 0 {
			fr.StartLine = 1 // 1-indexed
		}
		if fr.StartLine == 1 && fr.EndLine == 1 && f.LineCount() == 0 {
			// Empty file.
			return &fr, f, nil
		}
		if fr.StartLine < 0 || fr.StartLine > f.LineCount() {
			return nil, nil, fmt.Errorf("start line %d out of bounds (%d lines total)", fr.StartLine, f.LineCount())
		}
		if fr.EndLine <= 0 || fr.EndLine > f.LineCount() {
			return nil, nil, fmt.Errorf("end line %d out of bounds (%d lines total)", fr.EndLine, f.LineCount())
		}
		fr.StartByte, fr.EndByte = f.LineOffset(fr.StartLine), f.LineEndOffset(fr.EndLine)
	} else if bytes {
		if fr.StartByte < 0 || fr.StartByte > len(data)-1 {
			return nil, nil, fmt.Errorf("start byte %d out of bounds (%d bytes total)", fr.StartByte, len(data))
		}
		if fr.EndByte < 0 || fr.EndByte > len(data) {
			return nil, nil, fmt.Errorf("end byte %d out of bounds (%d bytes total)", fr.EndByte, len(data))
		}

		fr.StartLine, fr.EndLine = f.Line(f.Pos(fr.StartByte)), f.Line(f.Pos(fr.EndByte))
		if opt.ExpandContextLines > 0 {
			fr.StartLine -= opt.ExpandContextLines
			if fr.StartLine < 1 {
				fr.StartLine = 1
			}
			fr.EndLine += opt.ExpandContextLines
			if fr.EndLine > f.LineCount() {
				fr.EndLine = f.LineCount()
			}
		}
		if opt.ExpandContextLines > 0 || opt.FullLines {
			fr.StartByte, fr.EndByte = f.LineOffset(fr.StartLine), f.LineEndOffset(fr.EndLine)
		}
	}

	return &fr, f, nil
}

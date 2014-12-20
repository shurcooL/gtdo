package main

import (
	"go/ast"
	"go/token"

	"github.com/sourcegraph/annotate"
)

// fileOffset returns the offset of pos within its file.
func fileOffset(fset *token.FileSet, pos token.Pos) int {
	return fset.File(pos).Offset(pos)
}

// annotateNode annonates the given node with left and right.
func annotateNode(fset *token.FileSet, node ast.Node, left, right string, level int) *annotate.Annotation {
	return &annotate.Annotation{
		Start:     fileOffset(fset, node.Pos()),
		End:       fileOffset(fset, node.End()),
		WantInner: level,

		Left:  []byte(left),
		Right: []byte(right),
	}
}

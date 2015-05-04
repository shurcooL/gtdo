package main

import (
	"fmt"
	"io"
)

var cmdBranches = &Command{
	UsageLine: "branches [-v]",
	Short:     "list repository branches",
	Long:      ``,
}

func init() {
	addStdFlags(cmdBranches)
	cmdBranches.Run = runBranches
}

func runBranches(cmd *Command, w io.Writer, args []string) {
	openRepository(args)

	st := repo.NewStore()
	clIndex, err := st.OpenChangeLog()
	if err != nil {
		fatalf("%s", err)
	}

	for r := clIndex.Tip(); r.FileRev() != -1; r = r.Prev() {
		id := r.Id().Node()
		s := branchHeads.ById[id]
		for i := range s {
			t := s[len(s)-i-1]
			fmt.Fprintf(w, "%-26s %5d:%s\n", t, r.FileRev(), id[:12])
		}
	}
}

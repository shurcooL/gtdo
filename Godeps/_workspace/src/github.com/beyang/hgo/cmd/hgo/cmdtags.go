package main

import (
	"fmt"
	"io"
)

var cmdTags = &Command{
	UsageLine: "tags [-v]",
	Short:     "list repository tags",
	Long:      ``,
}

func init() {
	addStdFlags(cmdTags)
	cmdTags.Run = runTags
}

func runTags(cmd *Command, w io.Writer, args []string) {
	openRepository(args)

	st := repo.NewStore()
	clIndex, err := st.OpenChangeLog()
	if err != nil {
		fatalf("%s", err)
	}

	if globalTags != allTags {
		globalTags.Add("tip", clIndex.Tip().Id().Node())
	}
	allTags.Add("tip", clIndex.Tip().Id().Node())

	for r := clIndex.Tip(); r.FileRev() != -1; r = r.Prev() {
		id := r.Id().Node()
		s := allTags.ById[id]
		for i := range s {
			t := s[len(s)-i-1]
			local := ""
			if verbose && globalTags.IdByName[t] == "" {
				local = " local"
			}
			fmt.Fprintf(w, "%-30s %5d:%s%s\n", t, r.FileRev(), id[:12], local)
		}
	}
}

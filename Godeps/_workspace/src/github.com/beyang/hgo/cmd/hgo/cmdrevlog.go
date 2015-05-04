package main

import (
	"bytes"
	"io"
	"os"

	"github.com/beyang/hgo/revlog"
	"github.com/beyang/hgo/revlog/patch"
)

var cmdRevlog = &Command{
	UsageLine: "revlog [-r filerev] [-build] file.i",
	Short:     "dump a revlog index, or the contents of a revision specified by -r",
	Long:      ``,
}

var revlogR = cmdRevlog.Flag.Int("r", -1, "file revision")
var revlogBuild = cmdRevlog.Flag.Bool("build", false, "build the file, don't show the incremental data")

func init() {
	cmdRevlog.Run = runRevlog
}

func runRevlog(cmd *Command, w io.Writer, args []string) {
	if len(args) == 0 {
		fatalf("missing argument: revlog index file")
	}
	index, err := revlog.Open(storeName(args[0]))
	if err != nil {
		fatalf("%s", err)
	}

	if *revlogR == -1 {
		index.Dump(w)
		return
	}

	r, err := revlog.FileRevSpec(*revlogR).Lookup(index)
	if err != nil {
		fatalf("%s", err)
	}
	if !*revlogBuild {
		dh := &dataHelper{}
		d, err := r.GetData(dh)
		if dh.file != nil {
			dh.file.Close()
		}
		if err != nil {
			fatalf("%s", err)
		}
		if r.IsBase() {
			w.Write(d)
		} else {
			hunks, err := patch.Parse(d)
			if err != nil {
				fatalf("%s", err)
			}
			for _, h := range hunks {
				h.Dump(w)
			}
		}
	} else {
		fb := revlog.NewFileBuilder()
		err = fb.BuildWrite(w, r)
		if err != nil {
			fatalf("%s", err)
		}
	}
}

type storeName string

func (s storeName) Index() string {
	return string(s)
}
func (s storeName) Data() string {
	return string(s[:len(s)-2] + ".d")
}

type dataHelper struct {
	file revlog.DataReadCloser
	tmp  *bytes.Buffer
}

func (dh *dataHelper) Open(fileName string) (file revlog.DataReadCloser, err error) {
	if dh.file != nil {
		file = dh.file
		return
	}
	file, err = os.Open(fileName)
	if err == nil {
		dh.file = file
	}
	return
}

func (dh *dataHelper) TmpBuffer() *bytes.Buffer {
	return dh.tmp
}

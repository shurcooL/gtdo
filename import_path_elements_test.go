package main_test

import (
	"fmt"

	gtdo "github.com/shurcooL/gtdo"
)

func ExampleImportPathElementsHtml() {
	inputs := []struct {
		repoImportPath string
		importPath     string
	}{
		{"github.com/shurcooL/go", "github.com/shurcooL/go/u/u10"},
		{"rsc.io/pd&f", "rsc.io/pd&f"},
		{"rsc.io/pdf", "rsc.io/pdf"},
		{"rsc.io/pdf", "rsc.io/pdf/pdfpasswd"},
		{"io", "io"},
		{"io", "io/ioutil"},
	}

	for _, i := range inputs {
		html := gtdo.ImportPathElementsHtml(i.repoImportPath, i.importPath, "")
		fmt.Println(html)
	}

	// Output:
	// <a href="/github.com/shurcooL/go">github.com/shurcooL/go</a>/<a href="/github.com/shurcooL/go/u">u</a>/u10
	// rsc.io/pd&amp;f
	// rsc.io/pdf
	// <a href="/rsc.io/pdf">rsc.io/pdf</a>/pdfpasswd
	// io
	// <a href="/io">io</a>/ioutil
}

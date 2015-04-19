package main_test

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"path"
	"strings"

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
		out1 := gtdo.ImportPathElementsHtml(i.repoImportPath, i.importPath, "")
		out2 := previousHtmlTemplateApproach(i.repoImportPath, i.importPath)

		if out1 != out2 {
			log.Printf("out1 != out2\n%q\n%q\n", out1, out2)
		}

		fmt.Println(out1)
	}

	// Output:
	// <a href="/github.com/shurcooL/go">github.com/shurcooL/go</a>/<a href="/github.com/shurcooL/go/u">u</a>/u10
	// rsc.io/pd&amp;f
	// rsc.io/pdf
	// <a href="/rsc.io/pdf">rsc.io/pdf</a>/pdfpasswd
	// io
	// <a href="/io">io</a>/ioutil
}

func previousHtmlTemplateApproach(repoImportPath, importPath string) template.HTML {
	data := struct {
		ImportPathElements [][2]string // Element name, and full path to element.
	}{}

	{
		elements := strings.Split(importPath, "/")
		elements = elements[len(strings.Split(repoImportPath, "/")):]

		data.ImportPathElements = [][2]string{
			[2]string{repoImportPath, repoImportPath},
		}
		for i, e := range elements {
			data.ImportPathElements = append(data.ImportPathElements,
				[2]string{e, repoImportPath + "/" + path.Join(elements[:i+1]...)},
			)
		}
		// Don't link the last element, since it's the current page.
		data.ImportPathElements[len(data.ImportPathElements)-1][1] = ""
	}

	t, err := template.New("import-path.html.tmpl").Parse(`{{define "ImportPath"}}{{range $i, $v := .}}{{if $i}}/{{end}}{{if (index $v 1)}}<a href="/{{(index $v 1)}}">{{(index $v 0)}}</a>{{else}}{{(index $v 0)}}{{end}}{{end}}{{end}}`)
	if err != nil {
		panic(err)
	}

	var buf bytes.Buffer
	err = t.ExecuteTemplate(&buf, "ImportPath", data.ImportPathElements)
	if err != nil {
		panic(err)
	}

	return template.HTML(buf.String())
}

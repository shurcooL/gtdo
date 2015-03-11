package main

import (
	"html/template"
	"path"
	"strings"

	"github.com/shurcooL/go/html_gen"
	"golang.org/x/net/html"
)

func ImportPathElementsHtml(repoImportPath, importPath string) template.HTML {
	// Elements of importPath, first element being repoImportPath.
	// E.g., {"github.com/user/repo", "subpath", "package"}.
	elements := []string{repoImportPath}
	elements = append(elements, strings.Split(importPath[len(repoImportPath):], "/")[1:]...)

	var ns []*html.Node
	for i, element := range elements {
		if i != 0 {
			ns = append(ns, html_gen.Text("/"))
		}

		path := path.Join(elements[:i+1]...)

		// Don't link last importPath element, since it's the current page.
		if path != importPath {
			ns = append(ns, html_gen.A(element, template.URL("/"+path)))
		} else {
			ns = append(ns, html_gen.Text(element))
		}
	}

	importPathElements, err := html_gen.RenderNodes(ns...)
	if err != nil {
		panic(err)
	}
	return importPathElements
}

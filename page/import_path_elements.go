package page

import (
	"html/template"
	"net/url"
	"path"
	"strings"

	"github.com/shurcooL/htmlg"
	"golang.org/x/net/html"
)

// ImportPathElementsHTML renders the HTML of the import path with linkified elements.
func ImportPathElementsHTML(repoImportPath, importPath, rawQuery string) template.HTML {
	// Elements of importPath, first element being repoImportPath.
	// E.g., {"github.com/user/repo", "subpath", "package"}.
	elements := []string{repoImportPath}
	elements = append(elements, strings.Split(importPath[len(repoImportPath):], "/")[1:]...)

	var ns []*html.Node
	for i, element := range elements {
		if i != 0 {
			ns = append(ns, htmlg.Text("/"))
		}

		path := path.Join(elements[:i+1]...)

		// Don't link last importPath element, since it's the current page.
		if path != importPath {
			url := url.URL{
				Path:     "/" + path,
				RawQuery: rawQuery,
			}
			ns = append(ns, htmlg.A(element, template.URL(url.String())))
		} else {
			ns = append(ns, htmlg.Text(element))
		}
	}

	return htmlg.Render(ns...)
}

package page

import (
	"html/template"
	"net/url"

	"github.com/shurcooL/htmlg"
	"github.com/shurcooL/octiconssvg"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Tabs renders the html for <nav> element with tab header links.
func Tabs(path string, rawQuery string) template.HTML {
	query, _ := url.ParseQuery(rawQuery)

	const key = "tab"
	selectedTab := query.Get(key)

	var ns []*html.Node

	for _, tab := range []struct {
		id   string
		name string
		icon func() *html.Node
		beta bool
	}{
		{id: "summary", name: "Summary"},
		{id: "", name: "Code", icon: octiconssvg.Code},
		{id: "imports", name: "Imports"},
		{id: "dependents", name: "Dependents"},
	} {
		a := &html.Node{Type: html.ElementNode, Data: atom.A.String()}
		aClass := "tabnav-tab"
		if tab.id == selectedTab {
			aClass += " selected"
		} else {
			q := query
			if tab.id == "" {
				q.Del(key)
			} else {
				q.Set(key, tab.id)
			}
			u := url.URL{
				Path:     path,
				RawQuery: q.Encode(),
			}
			a.Attr = []html.Attribute{
				{Key: atom.Href.String(), Val: u.String()},
			}
		}
		a.Attr = append(a.Attr, html.Attribute{Key: atom.Class.String(), Val: aClass})
		if tab.icon != nil {
			icon := htmlg.Span(tab.icon())
			icon.Attr = append(icon.Attr, html.Attribute{
				Key: atom.Style.String(), Val: "margin-right: 4px;",
			})
			a.AppendChild(icon)
		}
		a.AppendChild(htmlg.Text(tab.name))
		if tab.beta {
			span := &html.Node{
				Type: html.ElementNode, Data: atom.Span.String(),
				Attr: []html.Attribute{{Key: atom.Class.String(), Val: "beta"}},
			}
			span.AppendChild(htmlg.Text("beta"))
			a.AppendChild(span)
		}
		ns = append(ns, a)
	}

	nav := &html.Node{
		Type: html.ElementNode, Data: atom.Nav.String(),
		Attr: []html.Attribute{{Key: atom.Class.String(), Val: "tabnav-tabs"}},
	}
	htmlg.AppendChildren(nav, ns...)
	return template.HTML(htmlg.Render(htmlg.DivClass("tabnav", nav)))
}

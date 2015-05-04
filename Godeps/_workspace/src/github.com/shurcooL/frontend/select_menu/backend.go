// +build !js

package select_menu

import (
	"fmt"
	"html/template"
	"net/url"
	"strconv"

	"github.com/shurcooL/go/html_gen"
	"golang.org/x/net/html"
)

// New creates the HTML for a select menu instance with the specified parameters.
func New(options []string, defaultOption string, query url.Values, queryParameter string) template.HTML {
	//selectElement := document.CreateElement("select").(*dom.HTMLSelectElement)
	selectElement := &html.Node{Type: html.ElementNode, Data: "select"}

	{
		var selectedOption = defaultOption
		if query.Get(queryParameter) != "" {
			selectedOption = query.Get(queryParameter)
		}
		if !contains(options, selectedOption) {
			options = append(options, selectedOption)
		}

		for _, option := range options {
			/*o := document.CreateElement("option").(*dom.HTMLOptionElement)
			o.Text = option
			if option == selectedOption {
				o.DefaultSelected = true
			}
			selectElement.AppendChild(o)*/

			o := &html.Node{Type: html.ElementNode, Data: "option"}
			o.AppendChild(html_gen.Text(option))
			if option == selectedOption {
				o.Attr = append(o.Attr, html.Attribute{Key: "selected"})
			}
			selectElement.AppendChild(o)
		}
	}

	/*selectElement.AddEventListener("input", false, func(event dom.Event) {
		selectedIndex := selectElement.Underlying().Get("selectedIndex").Int()
		selected := options[selectedIndex]

		if selected == defaultOption {
			query.Del(queryParameter)
		} else {
			query.Set(queryParameter, selected)
		}

		dom.GetWindow().Location().Search = "?" + query.Encode()
	})*/

	selectElement.Attr = append(selectElement.Attr, html.Attribute{
		Key: "oninput",
		// HACK: Don't use Sprintf, properly encode (as json at this time).
		Val: fmt.Sprintf(`SelectMenuOnInput(event, this, %q, %q);`, strconv.Quote(defaultOption), strconv.Quote(queryParameter)),
	})

	html, err := html_gen.RenderNodes(selectElement)
	if err != nil {
		panic(err)
	}
	return html
}

func contains(ss []string, t string) bool {
	for _, s := range ss {
		if s == t {
			return true
		}
	}
	return false
}

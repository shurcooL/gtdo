// +build !js

package checkbox

import (
	"fmt"
	"html/template"
	"net/url"
	"strconv"

	"github.com/shurcooL/go/html_gen"
	"golang.org/x/net/html"
)

// New creates the HTML for a checkbox instance. Its checked value is directly connected
// to the presence of queryParameter.
// Changing either the presence of queryParameter, or checking/unchecking the checkbox
// will result in the other updating to match.
func New(defaultValue bool, query url.Values, queryParameter string) template.HTML {
	inputElement := &html.Node{
		Type: html.ElementNode,
		Data: "input",
		Attr: []html.Attribute{{Key: "type", Val: "checkbox"}},
	}

	{
		var selectedValue = defaultValue
		if _, set := query[queryParameter]; set {
			selectedValue = !selectedValue
		}

		if selectedValue {
			inputElement.Attr = append(inputElement.Attr, html.Attribute{Key: "checked"})
		}
	}

	inputElement.Attr = append(inputElement.Attr, html.Attribute{
		Key: "onchange",
		// HACK: Don't use Sprintf, properly encode (as json at this time).
		Val: fmt.Sprintf(`CheckboxOnChange(event, this, %v, %q);`, defaultValue, strconv.Quote(queryParameter)),
	})

	html, err := html_gen.RenderNodes(inputElement)
	if err != nil {
		panic(err)
	}
	return html
}

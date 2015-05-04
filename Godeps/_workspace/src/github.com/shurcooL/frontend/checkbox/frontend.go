// +build js

package checkbox

import (
	"net/url"
	"strings"

	"github.com/gopherjs/gopherjs/js"
	"github.com/shurcooL/go/gopherjs_http/jsutil"
	"honnef.co/go/js/dom"
)

func init() {
	js.Global.Set("CheckboxOnChange", jsutil.Wrap(CheckboxOnChange))
}

func CheckboxOnChange(event dom.Event, object dom.HTMLElement, defaultValue bool, queryParameter string) {
	rawQuery := strings.TrimPrefix(dom.GetWindow().Location().Search, "?")
	query, _ := url.ParseQuery(rawQuery)

	inputElement := object.(*dom.HTMLInputElement)

	selected := inputElement.Checked

	if selected == defaultValue {
		query.Del(queryParameter)
	} else {
		query.Set(queryParameter, "")
	}

	dom.GetWindow().Location().Search = "?" + query.Encode()
}

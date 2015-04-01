// +build js

package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gopherjs/gopherjs/js"
	_ "github.com/shurcooL/frontend/checkbox"
	_ "github.com/shurcooL/frontend/select-list-view"
	_ "github.com/shurcooL/frontend/select_menu"
	_ "github.com/shurcooL/frontend/table-of-contents"
	"github.com/shurcooL/go/gopherjs_http/jsutil"
	"honnef.co/go/js/dom"
)

var document = dom.GetWindow().Document().(dom.HTMLDocument)

// targetId must point to a valid target.
func MustScrollTo(event dom.Event, targetId string) {
	target := document.GetElementByID(targetId).(dom.HTMLElement)

	// dom.GetWindow().History().ReplaceState(nil, nil, href)
	js.Global.Get("window").Get("history").Call("replaceState", nil, nil, "#"+targetId)

	windowHalfHeight := dom.GetWindow().InnerHeight() * 2 / 5
	dom.GetWindow().ScrollTo(dom.GetWindow().ScrollX(), int(target.OffsetTop()+target.OffsetHeight())-windowHalfHeight)

	processHash(targetId, true)

	fmt.Println("MustScrollTo:", targetId)
}

// targetId must point to a valid target.
func LineNumber(event dom.Event, targetId string) {
	//target := document.GetElementByID(targetId).(dom.HTMLElement)

	// dom.GetWindow().History().ReplaceState(nil, nil, href)
	js.Global.Get("window").Get("history").Call("replaceState", nil, nil, "#"+targetId)

	//windowHalfHeight := dom.GetWindow().InnerHeight() * 2 / 5
	//dom.GetWindow().ScrollTo(dom.GetWindow().ScrollX(), int(target.OffsetTop()+target.OffsetHeight())-windowHalfHeight)

	processHash(targetId, true)

	fmt.Println("LineNumber:", targetId)
}

// valid is true iff the hash points to a valid target.
func processHash(hash string, valid bool) {
	// Clear everything.
	for _, e := range document.GetElementsByClassName("selected-line") {
		e.Class().Remove("selected-line")
	}

	if !valid {
		return
	}

	file, line := parseHash(hash)
	_, _ = file, line

	if line != 0 {
		// DEBUG: Highlight entire file in red.
		/*fileHeader := document.GetElementByID(file).(dom.HTMLElement)
		fileContent := fileHeader.ParentElement().GetElementsByClassName("file")[0].(dom.HTMLElement)
		fileContent.Style().SetProperty("background-color", "red", "")*/

		lineElement := document.GetElementByID(hash).(dom.HTMLElement)
		lineElement.Class().Add("selected-line")
	}
}

func parseHash(hash string) (file string, line int) {
	parts := strings.Split(hash, "-")
	if file, line, ok := tryParseFileLine(parts); ok {
		return file, line
	}
	return hash, 0
}

func tryParseFileLine(parts []string) (file string, line int, ok bool) {
	if len(parts) <= 1 {
		return "", 0, false
	}
	lastPart := parts[len(parts)-1]
	if len(lastPart) < 2 || lastPart[0] != 'L' {
		return "", 0, false
	}
	line, err := strconv.Atoi(lastPart[1:])
	if err != nil {
		return "", 0, false
	}
	return strings.Join(parts[:len(parts)-1], "-"), line, true
}

func init() {
	js.Global.Set("MustScrollTo", jsutil.Wrap(MustScrollTo))
	js.Global.Set("LineNumber", jsutil.Wrap(LineNumber))

	document.AddEventListener("DOMContentLoaded", false, func(_ dom.Event) {
		// This needs to be in a goroutine or else it "happens too early". See if there's a better event than DOMContentLoaded.
		go func() {
			//time.Sleep(

			// Scroll to hash target.
			hash := strings.TrimPrefix(dom.GetWindow().Location().Hash, "#")
			target, ok := document.GetElementByID(hash).(dom.HTMLElement)
			if ok {
				windowHalfHeight := dom.GetWindow().InnerHeight() * 2 / 5
				dom.GetWindow().ScrollTo(dom.GetWindow().ScrollX(), int(target.OffsetTop()+target.OffsetHeight())-windowHalfHeight)
			}

			processHash(hash, ok)

			fmt.Println("DOMContentLoaded:", hash)
		}()
	})

	// Start watching for hashchange events.
	dom.GetWindow().AddEventListener("hashchange", false, func(event dom.Event) {
		event.PreventDefault()

		// Scroll to hash target.
		hash := strings.TrimPrefix(dom.GetWindow().Location().Hash, "#")
		target, ok := document.GetElementByID(hash).(dom.HTMLElement)
		if ok {
			windowHalfHeight := dom.GetWindow().InnerHeight() * 2 / 5
			dom.GetWindow().ScrollTo(dom.GetWindow().ScrollX(), int(target.OffsetTop()+target.OffsetHeight())-windowHalfHeight)
		}

		processHash(hash, ok)

		fmt.Println("hash changed:", hash)
	})
}

func main() {}

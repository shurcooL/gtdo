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

var state selection

type selection struct {
	valid bool
	file  string
	start int
	end   int
}

// Hash returns a hash encoding of the selection, without '#'.
func (s selection) Hash() string {
	if !s.valid {
		return ""
	}
	hash := s.file
	if s.start != 0 {
		hash += fmt.Sprintf("-L%d", s.start)
	}
	if s.start != s.end {
		hash += fmt.Sprintf("-L%d", s.end)
	}
	return hash
}

// targetId must point to a valid target.
func MustScrollTo(event dom.Event, targetId string) {
	target := document.GetElementByID(targetId).(dom.HTMLElement)

	// dom.GetWindow().History().ReplaceState(nil, nil, href)
	js.Global.Get("window").Get("history").Call("replaceState", nil, nil, "#"+targetId)

	windowHalfHeight := dom.GetWindow().InnerHeight() * 2 / 5
	dom.GetWindow().ScrollTo(dom.GetWindow().ScrollX(), target.OffsetTop()+target.OffsetHeight()-windowHalfHeight)

	processHash(targetId, true)

	fmt.Println("MustScrollTo:", targetId)
}

// expandLineSelection expands line selection if shift was held down when clicking a line number,
// and it's in the same file as already highlighted. Otherwise return original targetId unmodified.
func expandLineSelection(event dom.Event, targetId string) string {
	me, ok := event.(*dom.MouseEvent)
	if !(ok && me.ShiftKey && state.valid && state.start != 0) {
		return targetId
	}
	file, start, end := parseHash(targetId)
	if !(file == state.file && start != 0) {
		return targetId
	}
	switch {
	case start < state.start:
		state.start = start
	case end > state.end:
		state.end = end
	}
	return state.Hash()
}

// targetId must point to a valid target.
func LineNumber(event dom.Event, targetId string) {
	targetId = expandLineSelection(event, targetId)

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
	/*for _, e := range document.GetElementsByClassName("selected-line") {
		e.Class().Remove("selected-line")
	}*/
	for _, e := range document.GetElementsByClassName("background") {
		e.(dom.HTMLElement).Style().SetProperty("display", "none", "")
	}

	if !valid {
		state.valid = false
		return
	}

	file, start, end := parseHash(hash)
	state.file, state.start, state.end, state.valid = file, start, end, true

	if start != 0 {
		// DEBUG: Highlight entire file in red.
		/*fileHeader := document.GetElementByID(file).(dom.HTMLElement)
		fileContent := fileHeader.ParentElement().GetElementsByClassName("file")[0].(dom.HTMLElement)
		fileContent.Style().SetProperty("background-color", "red", "")*/

		//lineElement := document.GetElementByID(fmt.Sprintf("%s-L%d", file, start)).(dom.HTMLElement)
		//lineElement.Class().Add("selected-line")

		startElement := document.GetElementByID(fmt.Sprintf("%s-L%d", file, start)).(dom.HTMLElement)
		var endElement dom.HTMLElement
		if end == start {
			endElement = startElement
		} else {
			endElement = document.GetElementByID(fmt.Sprintf("%s-L%d", file, end)).(dom.HTMLElement)
		}

		fileHeader := document.GetElementByID(file).(dom.HTMLElement)
		fileBackground := fileHeader.ParentElement().GetElementsByClassName("background")[0].(dom.HTMLElement)
		fileBackground.Style().SetProperty("display", "initial", "")
		fileBackground.Style().SetProperty("top", fmt.Sprintf("%vpx", startElement.OffsetTop()), "")
		fileBackground.Style().SetProperty("height", fmt.Sprintf("%vpx", endElement.OffsetTop()-startElement.OffsetTop()+endElement.OffsetHeight()), "")
	}
}

func parseHash(hash string) (file string, start, end int) {
	parts := strings.Split(hash, "-")
	if file, start, end, ok := tryParseFileLineRange(parts); ok {
		fmt.Println("tryParseFileLineRange:", file, start, end, ok)
		return file, start, end
	} else if file, line, ok := tryParseFileLine(parts); ok {
		return file, line, line
	} else {
		return hash, 0, 0
	}
}

func tryParseFileLine(parts []string) (file string, line int, ok bool) {
	if len(parts) < 2 {
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

func tryParseFileLineRange(parts []string) (file string, start, end int, ok bool) {
	if len(parts) < 3 {
		return "", 0, 0, false
	}
	{
		secondLastPart := parts[len(parts)-2]
		if len(secondLastPart) < 2 || secondLastPart[0] != 'L' {
			return "", 0, 0, false
		}
		var err error
		start, err = strconv.Atoi(secondLastPart[1:])
		if err != nil {
			return "", 0, 0, false
		}
	}
	{
		lastPart := parts[len(parts)-1]
		if len(lastPart) < 2 || lastPart[0] != 'L' {
			return "", 0, 0, false
		}
		var err error
		end, err = strconv.Atoi(lastPart[1:])
		if err != nil {
			return "", 0, 0, false
		}
	}
	return strings.Join(parts[:len(parts)-2], "-"), start, end, true
}

// rootOffsetTop returns the offset top of element e relative to root element.
func rootOffsetTop(e dom.HTMLElement) float64 {
	var rootOffsetTop float64
	for ; e != nil; e = e.OffsetParent() {
		rootOffsetTop += e.OffsetTop()
	}
	return rootOffsetTop
}

func init() {
	js.Global.Set("MustScrollTo", jsutil.Wrap(MustScrollTo))
	js.Global.Set("LineNumber", jsutil.Wrap(LineNumber))

	processHashSet := func() {
		// Scroll to hash target.
		hash := strings.TrimPrefix(dom.GetWindow().Location().Hash, "#")
		parts := strings.Split(hash, "-") // TODO: Factor out.
		var targetId string
		if file, start, _, ok := tryParseFileLineRange(parts); ok {
			targetId = fmt.Sprintf("%s-L%d", file, start)
		} else {
			targetId = hash
		}
		target, ok := document.GetElementByID(targetId).(dom.HTMLElement)
		if ok {
			windowHalfHeight := dom.GetWindow().InnerHeight() * 2 / 5
			dom.GetWindow().ScrollTo(dom.GetWindow().ScrollX(), rootOffsetTop(target)+target.OffsetHeight()-windowHalfHeight)
		}

		processHash(hash, ok)
	}
	document.AddEventListener("DOMContentLoaded", false, func(_ dom.Event) {
		// This needs to be in a goroutine or else it "happens too early". See if there's a better event than DOMContentLoaded.
		go func() {
			//time.Sleep(

			processHashSet()

			fmt.Println("DOMContentLoaded:", strings.TrimPrefix(dom.GetWindow().Location().Hash, "#"))
		}()
	})
	// Start watching for hashchange events.
	dom.GetWindow().AddEventListener("hashchange", false, func(event dom.Event) {
		event.PreventDefault()

		processHashSet()

		fmt.Println("hash changed:", strings.TrimPrefix(dom.GetWindow().Location().Hash, "#"))
	})
}

func main() {}

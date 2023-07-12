// +build js

package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/gopherjs/eventsource"
	"github.com/gopherjs/gopherjs/js"
	_ "github.com/shurcooL/frontend/checkbox"
	_ "github.com/shurcooL/frontend/select_menu"
	"github.com/shurcooL/go/gopherjs_http/jsutil"
	"github.com/shurcooL/gtdo/frontend/selectlistview"
	"github.com/shurcooL/gtdo/frontend/tableofcontents"
	"github.com/shurcooL/gtdo/gtdo"
	"github.com/shurcooL/gtdo/page"
	"honnef.co/go/js/dom"
)

var document = dom.GetWindow().Document().(dom.HTMLDocument)

var state selection

type selection struct {
	valid bool
	file  string
	start int // Start line of selection, or 0 if there's no line selection. First line is 1.
	end   int // End line of selection. Same as start if it's one line selection.
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

func main() {
	js.Global.Set("MustScrollTo", jsutil.Wrap(MustScrollTo))
	js.Global.Set("LineNumber", jsutil.Wrap(LineNumber))
	js.Global.Set("HideOutdatedBox", HideOutdatedBox)

	switch readyState := document.ReadyState(); readyState {
	case "loading":
		document.AddEventListener("DOMContentLoaded", false, func(dom.Event) {
			go setup()
		})
	case "interactive", "complete":
		setup()
	default:
		panic(fmt.Errorf("internal error: unexpected document.ReadyState value: %v", readyState))
	}

	var state page.State
	err := json.Unmarshal([]byte(js.Global.Get("StateJSON").String()), &state)
	if err != nil {
		panic(err)
	}

	// EventSource updates.
	if state.RepoSpec.CloneURL != "" {
		u := url.URL{
			Path: "/-/events",
			RawQuery: url.Values{
				"ImportPath":        {state.ImportPath},
				"Branch":            {state.ProcessedRev},
				"RepoSpec.VCSType":  {state.RepoSpec.VCSType},
				"RepoSpec.CloneURL": {state.RepoSpec.CloneURL},
			}.Encode(),
		}

		source := eventsource.New(u.String())
		source.AddEventListener("message", false, func(event *js.Object) {
			data := event.Get("data").String()
			if data == "outdated" {
				showOutdatedBox()
			}
		})
	}
}

func setup() {
	tableofcontents.Setup()
	selectlistview.Setup()

	// Jump to desired hash after page finishes loading (and override browser's default hash jumping).
	go func() {
		// This needs to be in a goroutine or else it "happens too early".
		// TODO: See if there's a better event than DOMContentLoaded.
		processHashSet()
	}()
	// Start watching for hashchange events.
	dom.GetWindow().AddEventListener("hashchange", false, func(event dom.Event) {
		event.PreventDefault()

		processHashSet()
	})

	document.Body().AddEventListener("keydown", false, func(event dom.Event) {
		if event.DefaultPrevented() {
			return
		}
		// Ignore when some element other than body has focus (it means the user is typing elsewhere).
		if !event.Target().IsEqualNode(document.Body()) {
			return
		}

		switch ke := event.(*dom.KeyboardEvent); {
		// 'y' keyboard shortcut to get permalink.
		case ke.KeyCode == 'Y' && !ke.Repeat && !ke.CtrlKey && !ke.AltKey && !ke.MetaKey && !ke.ShiftKey:
			commitIdEl := document.GetElementByID("commit-id")
			if commitIdEl == nil {
				return
			}
			commitId := commitIdEl.GetAttribute("title")
			if commitId == "" {
				return
			}

			// Set revision query parameter to full commit id, if it's not already.
			query, _ := url.ParseQuery(strings.TrimPrefix(dom.GetWindow().Location().Search, "?"))
			if query.Get(gtdo.RevisionQueryParameter) != commitId {
				query.Set(gtdo.RevisionQueryParameter, commitId)
				// TODO: dom.GetWindow().History().PushState(...)
				js.Global.Get("window").Get("history").Call("pushState", nil, nil, "?"+query.Encode()+dom.GetWindow().Location().Hash)
			}

			ke.PreventDefault()

		// Escape.
		case ke.KeyCode == 27 && !ke.Repeat && !ke.CtrlKey && !ke.AltKey && !ke.MetaKey && !ke.ShiftKey:
			url := windowLocation
			url.Fragment = ""
			// TODO: dom.GetWindow().History().ReplaceState(...), blocked on https://github.com/dominikh/go-js-dom/issues/41.
			js.Global.Get("window").Get("history").Call("replaceState", nil, nil, url.String())

			processHashSet()

			ke.PreventDefault()
		}
	})
}

// targetId must point to a valid target.
func MustScrollTo(event dom.Event, targetId string) {
	target := document.GetElementByID(targetId).(dom.HTMLElement)

	// TODO: dom.GetWindow().History().ReplaceState(...)
	js.Global.Get("window").Get("history").Call("replaceState", nil, nil, "#"+targetId)

	windowHalfHeight := dom.GetWindow().InnerHeight() * 2 / 5
	dom.GetWindow().ScrollTo(dom.GetWindow().ScrollX(), int(offsetTopRoot(target)+target.OffsetHeight())-windowHalfHeight)

	processHash(targetId, true)
}

// expandLineSelection expands line selection if shift was held down when clicking a line number,
// and it's in the same file as already highlighted. Otherwise return original targetId unmodified.
func expandLineSelection(event dom.Event, targetId string) string {
	me, isMouseEvent := event.(*dom.MouseEvent)
	if !isMouseEvent {
		if pe, ok := event.(*dom.PointerEvent); ok {
			me, isMouseEvent = pe.MouseEvent, true
		}
	}
	if expand := isMouseEvent && me.ShiftKey && state.valid && state.start != 0; !expand {
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

	// TODO: dom.GetWindow().History().ReplaceState(...)
	js.Global.Get("window").Get("history").Call("replaceState", nil, nil, "#"+targetId)

	processHash(targetId, true)
}

func showOutdatedBox() {
	document.GetElementByID("outdated-box").(dom.HTMLElement).Style().SetProperty("display", "block", "")
}

func HideOutdatedBox() {
	document.GetElementByID("outdated-box").(dom.HTMLElement).Style().SetProperty("display", "none", "")
}

func processHashSet() {
	// Scroll to hash target.
	hash := strings.TrimPrefix(dom.GetWindow().Location().Hash, "#")
	parts := strings.Split(hash, "-")
	var targetId string
	if file, start, _, ok := tryParseFileLineRange(parts); ok {
		targetId = fmt.Sprintf("%s-L%d", file, start)
	} else {
		targetId = hash
	}
	target, ok := document.GetElementByID(targetId).(dom.HTMLElement)
	if ok {
		windowHalfHeight := dom.GetWindow().InnerHeight() * 2 / 5
		dom.GetWindow().ScrollTo(dom.GetWindow().ScrollX(), int(offsetTopRoot(target)+target.OffsetHeight())-windowHalfHeight)
	}

	processHash(hash, ok)
}

// valid is true iff the hash points to a valid target.
func processHash(hash string, valid bool) {
	// Clear everything.
	for _, e := range document.GetElementsByClassName("selection") {
		e.(dom.HTMLElement).Style().SetProperty("display", "none", "")
	}

	if !valid {
		state.valid = false
		return
	}

	file, start, end := parseHash(hash)
	state.file, state.start, state.end, state.valid = file, start, end, true

	if start != 0 {
		startElement := document.GetElementByID(fmt.Sprintf("%s-L%d", file, start)).(dom.HTMLElement)
		var endElement dom.HTMLElement
		if end == start {
			endElement = startElement
		} else {
			endElement = document.GetElementByID(fmt.Sprintf("%s-L%d", file, end)).(dom.HTMLElement)
		}

		fileHeader := document.GetElementByID(file).(dom.HTMLElement)
		fileBackground := fileHeader.ParentElement().GetElementsByClassName("selection")[0].(dom.HTMLElement)
		fileBackground.Style().SetProperty("display", "initial", "")
		fileBackground.Style().SetProperty("top", fmt.Sprintf("%vpx", startElement.OffsetTop()), "")
		fileBackground.Style().SetProperty("height", fmt.Sprintf("%vpx", endElement.OffsetTop()-startElement.OffsetTop()+endElement.OffsetHeight()), "")
	}
}

func parseHash(hash string) (file string, start, end int) {
	parts := strings.Split(hash, "-")
	if file, start, end, ok := tryParseFileLineRange(parts); ok {
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

// offsetTopRoot returns the offset top of element e relative to root element.
func offsetTopRoot(e dom.HTMLElement) float64 {
	var offsetTopRoot float64
	for ; e != nil; e = e.OffsetParent() {
		offsetTopRoot += e.OffsetTop()
	}
	return offsetTopRoot
}

var windowLocation = func() url.URL {
	url, err := url.Parse(dom.GetWindow().Location().Href)
	if err != nil {
		// We don't expect this can ever happen, so treat it as an internal error if it does.
		panic(fmt.Errorf("internal error: parsing window.location.href as URL failed: %v", err))
	}
	return *url
}()

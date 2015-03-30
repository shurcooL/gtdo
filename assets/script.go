// +build js

package main

import (
	"fmt"
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

func ScrollTo(event dom.Event, targetId string) {
	target, ok := document.GetElementByID(targetId).(dom.HTMLElement)
	if !ok {
		return
	}

	// dom.GetWindow().History().ReplaceState(nil, nil, href)
	js.Global.Get("window").Get("history").Call("replaceState", nil, nil, "#"+targetId)

	windowHalfHeight := dom.GetWindow().InnerHeight() * 2 / 5
	dom.GetWindow().ScrollTo(dom.GetWindow().ScrollX(), int(target.OffsetTop()+target.OffsetHeight())-windowHalfHeight)

	fmt.Println("ScrollTo:", targetId)
}

func LineNumber(event dom.Event, object dom.HTMLElement) {
	target := object.(dom.HTMLElement)

	// dom.GetWindow().History().ReplaceState(nil, nil, href)
	js.Global.Get("window").Get("history").Call("replaceState", nil, nil, "#"+target.ID())

	//windowHalfHeight := dom.GetWindow().InnerHeight() * 2 / 5
	//dom.GetWindow().ScrollTo(dom.GetWindow().ScrollX(), int(target.OffsetTop()+target.OffsetHeight())-windowHalfHeight)

	fmt.Println("LineNumber:", target.ID())
}

func init() {
	js.Global.Set("ScrollTo", jsutil.Wrap(ScrollTo))
	js.Global.Set("LineNumber", jsutil.Wrap(LineNumber))

	document.AddEventListener("DOMContentLoaded", false, func(_ dom.Event) {
		// This needs to be in a goroutine or else it "happens too early". See if there's a better event than DOMContentLoaded.
		go func() {
			//time.Sleep(

			// Scroll to hash target.
			hash := strings.TrimPrefix(dom.GetWindow().Location().Hash, "#")
			if target, ok := document.GetElementByID(hash).(dom.HTMLElement); ok {
				windowHalfHeight := dom.GetWindow().InnerHeight() * 2 / 5
				dom.GetWindow().ScrollTo(dom.GetWindow().ScrollX(), int(target.OffsetTop()+target.OffsetHeight())-windowHalfHeight)

				fmt.Println("DOMContentLoaded:", hash, target.OffsetTop())
			}

			//fmt.Println("DOMContentLoaded:", hash, target.OffsetTop())
		}()
	})

	// Start watching for hashchange events.
	dom.GetWindow().AddEventListener("hashchange", false, func(event dom.Event) {
		event.PreventDefault()

		// Scroll to hash target.
		hash := strings.TrimPrefix(dom.GetWindow().Location().Hash, "#")
		if target, ok := document.GetElementByID(hash).(dom.HTMLElement); ok {
			windowHalfHeight := dom.GetWindow().InnerHeight() * 2 / 5
			dom.GetWindow().ScrollTo(dom.GetWindow().ScrollX(), int(target.OffsetTop()+target.OffsetHeight())-windowHalfHeight)
		}

		fmt.Println("hash changed:", hash)
	})
}

func main() {}

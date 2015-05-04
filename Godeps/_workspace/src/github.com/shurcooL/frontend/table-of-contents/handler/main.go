package handler

import (
	"net/http"

	"github.com/shurcooL/go/gopherjs_http"
)

func init() {
	// HACK: Relative path, assuming starting in Conception-go folder.
	// TODO: Need a better way that will work for any package importing this...
	http.Handle("/table-of-contents.go.js", gopherjs_http.GoFiles("../frontend/table-of-contents/main.go"))
	http.HandleFunc("/table-of-contents.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../frontend/table-of-contents/style.css")
	})

	//http.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "main.go") }) // Test of relative path...
}

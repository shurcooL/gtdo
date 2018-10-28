package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"os"
)

// textHandler is a GET-only handler for serving text/plain content.
// It verifies that req.Method is GET, and rejects the request otherwise.
type textHandler func(w io.Writer, req *http.Request) error

func (h textHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		w.Header().Set("Allow", "GET")
		http.Error(w, "405 Method Not Allowed\n\nmethod should be GET", http.StatusMethodNotAllowed)
		return
	}
	var buf bytes.Buffer
	err := h(&buf, req)
	switch {
	case os.IsNotExist(err):
		log.Println(err)
		http.Error(w, "404 Not Found\n\n"+err.Error(), http.StatusNotFound)
	case os.IsPermission(err):
		log.Println(err)
		http.Error(w, "403 Forbidden\n\n"+err.Error(), http.StatusForbidden)
	case err != nil:
		log.Println(err)
		http.Error(w, "500 Internal Server Error\n\n"+err.Error(), http.StatusInternalServerError)
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.Copy(w, &buf)
	}
}

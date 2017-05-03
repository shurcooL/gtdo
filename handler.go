package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"os"
)

// handler is a GET-only handler for serving text/plain content.
// It verifies that req.Method is GET, and rejects the request otherwise.
type handler func(w io.Writer, req *http.Request) error

func (h handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method should be GET", http.StatusMethodNotAllowed)
		return
	}
	var buf bytes.Buffer
	err := h(&buf, req)
	switch {
	case os.IsNotExist(err):
		log.Println(err)
		http.Error(w, err.Error(), http.StatusNotFound)
	case os.IsPermission(err):
		log.Println(err)
		http.Error(w, err.Error(), http.StatusForbidden)
	case err != nil:
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.Copy(w, &buf)
	}
}

// +build dev

package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/shurcooL/go/gopherjs_http"
	"github.com/shurcooL/go/vfs/httpfs/union"
)

var assets = union.New(map[string]http.FileSystem{
	"/assets":                gopherjs_http.NewFS(http.Dir("assets")),
	"/select-list-view.css":  File(filepath.Join("..", "frontend", "select-list-view", "style.css")),
	"/table-of-contents.css": File(filepath.Join("..", "frontend", "table-of-contents", "style.css")),
})

// File implements http.FileSystem using the native file system restricted to a
// specific file served at root.
//
// While the FileSystem.Open method takes '/'-separated paths, a File's string
// value is a filename on the native file system, not a URL, so it is separated
// by filepath.Separator, which isn't necessarily '/'.
type File string

func (f File) Open(name string) (http.File, error) {
	if name != "/" {
		return nil, errors.New(fmt.Sprintf("not found: %v", name))
	}
	return os.Open(string(f))
}

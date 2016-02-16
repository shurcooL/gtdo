// +build dev

package main

import (
	"net/http"
	"path/filepath"

	"github.com/shurcooL/go/gopherjs_http"
	"github.com/shurcooL/httpfs/union"
	"github.com/shurcooL/httpfs/vfsutil"
	"github.com/shurcooL/octicons"
)

var assets = union.New(map[string]http.FileSystem{
	"/assets":                gopherjs_http.NewFS(http.Dir("assets")),
	"/octicons":              octicons.Assets,
	"/select-list-view.css":  vfsutil.File(filepath.Join("..", "frontend", "select-list-view", "style.css")),
	"/table-of-contents.css": vfsutil.File(filepath.Join("..", "frontend", "table-of-contents", "style.css")),
})

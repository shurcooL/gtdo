// +build dev

package assets

import (
	"go/build"
	"log"
	"net/http"
	"path/filepath"

	"github.com/shurcooL/go/gopherjs_http"
	"github.com/shurcooL/httpfs/union"
	"github.com/shurcooL/httpfs/vfsutil"
)

// Assets contains assets for gtdo.
var Assets = union.New(map[string]http.FileSystem{
	"/assets":                gopherjs_http.NewFS(http.Dir(importPathToDir("github.com/shurcooL/gtdo/_data"))),
	"/select-list-view.css":  vfsutil.File(filepath.Join(importPathToDir("github.com/shurcooL/frontend/select-list-view"), "style.css")),
	"/table-of-contents.css": vfsutil.File(filepath.Join(importPathToDir("github.com/shurcooL/frontend/table-of-contents"), "style.css")),
})

func importPathToDir(importPath string) string {
	p, err := build.Import(importPath, "", build.FindOnly)
	if err != nil {
		log.Fatalln(err)
	}
	return p.Dir
}

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
	"/assets":              http.Dir(importPathToDir("github.com/shurcooL/gtdo/_data")),
	"/frontend.js":         gopherjs_http.Package("github.com/shurcooL/gtdo/frontend"),
	"/selectlistview.css":  vfsutil.File(filepath.Join(importPathToDir("github.com/shurcooL/gtdo/frontend/selectlistview"), "style.css")),
	"/tableofcontents.css": vfsutil.File(filepath.Join(importPathToDir("github.com/shurcooL/gtdo/frontend/tableofcontents"), "style.css")),
})

func importPathToDir(importPath string) string {
	p, err := build.Import(importPath, "", build.FindOnly)
	if err != nil {
		log.Fatalln(err)
	}
	return p.Dir
}

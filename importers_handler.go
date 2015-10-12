package main

import (
	"compress/gzip"
	"go/build"
	"html/template"
	"io"
	"log"
	"net/http"

	"github.com/shurcooL/go/u/u5"
	"github.com/shurcooL/gtdo/gtdo"
	"github.com/shurcooL/gtdo/page"
)

func init() {
	switch *productionFlag {
	case true:
		u5.UserAgent = "http://gotools.org/"
	case false:
		u5.UserAgent = "https://github.com/shurcooL/gtdo"
	}
}

func importersHandler(w http.ResponseWriter, req *http.Request) {
	importPath := req.URL.Path[1:]
	rev := req.URL.Query().Get(gtdo.RevisionQueryParameter)

	log.Printf("req: importPath=%q rev=%q.\n", importPath, rev)

	source, bpkg, repoSpec, repoImportPath, _, fs, _, _, err := try(importPath, rev)
	log.Println("using source:", source)
	if err != nil {
		log.Println("try:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		Production         bool
		RawQuery           string
		Tabs               template.HTML
		ImportPath         string
		ImportPathElements template.HTML // Import path with linkified elements.
		RepoImportPath     string
		DirExists          bool
		Bpkg               *build.Package
		Importers          u5.Importers
		Folders            []string
	}{
		Production:         *productionFlag,
		RawQuery:           req.URL.RawQuery,
		Tabs:               page.Tabs(req.URL.Path, req.URL.RawQuery),
		ImportPath:         importPath,
		ImportPathElements: page.ImportPathElementsHTML(repoImportPath, importPath, req.URL.RawQuery),
		RepoImportPath:     repoImportPath,
		DirExists:          fs != nil,
		Bpkg:               bpkg,
	}

	// Folders.
	if fs != nil {
		fis, err := fs.ReadDir("/virtual-go-workspace/src/" + importPath)
		if err != nil {
			log.Println("fs.ReadDir(importPath):", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, fi := range fis {
			if !fi.IsDir() {
				continue
			}
			data.Folders = append(data.Folders, fi.Name())
		}
	}

	if fs != nil && bpkg != nil {
		if importers, err := u5.GetGodocOrgImporters(bpkg.ImportPath); err == nil {
			data.Importers = importers
		} else {
			log.Println(err)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var wr io.Writer = w
	if isGzipEncodingAccepted(req) {
		// Use gzip compression.
		w.Header().Set("Content-Encoding", "gzip")
		gw := gzip.NewWriter(w)
		defer gw.Close()
		wr = gw
	}

	err = t.ExecuteTemplate(wr, "importers.html.tmpl", &data)
	if err != nil {
		log.Printf("t.ExecuteTemplate: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if bpkg != nil {
		sendToTop(bpkg.ImportPath)
	}
	if RepoUpdater != nil && repoSpec != nil {
		RepoUpdater.Enqueue(*repoSpec)
	}
}

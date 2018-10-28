package main

import (
	"compress/gzip"
	"go/build"
	"html/template"
	"io"
	"log"
	"net/http"

	"github.com/shurcooL/go/gddo"
	"github.com/shurcooL/gtdo/page"
	"golang.org/x/net/http/httpguts"
)

var gddoClient gddo.Client

func init() {
	switch *productionFlag {
	case true:
		gddoClient.UserAgent = "https://gotools.org"
	case false:
		gddoClient.UserAgent = "https://github.com/shurcooL/gtdo"
	}
}

func (h *handler) dependentsHandler(w http.ResponseWriter, req *http.Request, importPath, rev string) {
	source, bpkg, repoSpec, repoImportPath, commit, fs, branches, defaultBranch, err := try(importPath, rev)
	log.Println("using source:", source)
	if err != nil {
		log.Println("try:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	frontendState := page.State{
		ImportPath:   importPath,
		ProcessedRev: rev,
	}
	if frontendState.ProcessedRev == "" && len(branches) != 0 {
		frontendState.ProcessedRev = defaultBranch
	}
	if repoSpec != nil {
		frontendState.RepoSpec.VCSType = repoSpec.vcsType
		frontendState.RepoSpec.CloneURL = repoSpec.cloneURL
	}
	if commit != nil {
		frontendState.CommitID = string(commit.ID)
	}

	data := struct {
		FrontendState      page.State // TODO: Maybe move RawQuery, etc., here?
		AnalyticsHTML      template.HTML
		RawQuery           string
		Tabs               template.HTML
		ImportPath         string
		ImportPathElements template.HTML // Import path with linkified elements.
		RepoImportPath     string
		DirExists          bool
		Bpkg               *build.Package
		Dependents         gddo.Importers
		Folders            []string
	}{
		FrontendState:      frontendState,
		AnalyticsHTML:      h.analyticsHTML,
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
		if dependents, err := gddoClient.GetImporters(bpkg.ImportPath); err == nil {
			data.Dependents = dependents
		} else {
			log.Println(err)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var wr io.Writer = w
	if httpguts.HeaderValuesContainsToken(req.Header["Accept-Encoding"], "gzip") {
		// Use gzip compression.
		w.Header().Set("Content-Encoding", "gzip")
		gw := gzip.NewWriter(w)
		defer gw.Close()
		wr = gw
	}

	err = t.ExecuteTemplate(wr, "dependents.html.tmpl", &data)
	if err != nil {
		log.Printf("t.ExecuteTemplate: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sendToTopMaybe(bpkg)
}

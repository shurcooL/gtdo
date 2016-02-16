package main

import (
	"bytes"
	"compress/gzip"
	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/token"
	"html/template"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"sort"

	"github.com/shurcooL/frontend/select_menu"
	"github.com/shurcooL/gtdo/gtdo"
	"github.com/shurcooL/gtdo/page"
	"golang.org/x/tools/godoc/vfs"
	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

func summaryHandler(w http.ResponseWriter, req *http.Request) {
	importPath := req.URL.Path[1:]
	rev := req.URL.Query().Get(gtdo.RevisionQueryParameter)

	log.Printf("req: importPath=%q rev=%q.\n", importPath, rev)

	source, bpkg, repoSpec, repoImportPath, commit, fs, branches, defaultBranch, err := try(importPath, rev)
	log.Println("using source:", source)
	if err != nil {
		log.Println("try:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	frontendState := page.State{
		Production:   *productionFlag,
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
		FrontendState      page.State // TODO: Maybe move Production, RawQuery, etc., here?
		Production         bool
		RawQuery           string
		Tabs               template.HTML
		ImportPath         string
		ImportPathElements template.HTML // Import path with linkified elements.
		RepoImportPath     string
		Commit             *vcs.Commit
		DirExists          bool
		Bpkg               *build.Package
		Dpkg               *doc.Package
		DocHTML            template.HTML
		Folders            []string
		Branches           template.HTML // Select menu for branches.
	}{
		FrontendState:      frontendState,
		Production:         *productionFlag,
		RawQuery:           req.URL.RawQuery,
		Tabs:               page.Tabs(req.URL.Path, req.URL.RawQuery),
		ImportPath:         importPath,
		ImportPathElements: page.ImportPathElementsHTML(repoImportPath, importPath, req.URL.RawQuery),
		RepoImportPath:     repoImportPath,
		Commit:             commit,
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

	// Branches.
	if len(branches) != 0 {
		data.Branches = select_menu.New(branches, defaultBranch, req.URL.Query(), gtdo.RevisionQueryParameter)
	}

	if fs != nil && bpkg != nil {
		if dpkg, err := docPackage(fs, bpkg); err == nil {
			data.Dpkg = dpkg

			var buf bytes.Buffer
			doc.ToHTML(&buf, dpkg.Doc, nil)
			data.DocHTML = template.HTML(buf.String())
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

	err = t.ExecuteTemplate(wr, "summary.html.tmpl", &data)
	if err != nil {
		log.Printf("t.ExecuteTemplate: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	afterPackageVisit(bpkg, repoSpec)
}

func importsHandler(w http.ResponseWriter, req *http.Request) {
	importPath := req.URL.Path[1:]
	rev := req.URL.Query().Get(gtdo.RevisionQueryParameter)

	log.Printf("req: importPath=%q rev=%q.\n", importPath, rev)

	source, bpkg, repoSpec, repoImportPath, commit, fs, branches, defaultBranch, err := try(importPath, rev)
	log.Println("using source:", source)
	if err != nil {
		log.Println("try:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	frontendState := page.State{
		Production:   *productionFlag,
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
		FrontendState      page.State // TODO: Maybe move Production, RawQuery, etc., here?
		Production         bool
		RawQuery           string
		Tabs               template.HTML
		ImportPath         string
		ImportPathElements template.HTML // Import path with linkified elements.
		RepoImportPath     string
		Commit             *vcs.Commit
		DirExists          bool
		Bpkg               *build.Package
		Folders            []string
		Branches           template.HTML // Select menu for branches.

		AdditionalTestImports []string
	}{
		FrontendState:      frontendState,
		Production:         *productionFlag,
		RawQuery:           req.URL.RawQuery,
		Tabs:               page.Tabs(req.URL.Path, req.URL.RawQuery),
		ImportPath:         importPath,
		ImportPathElements: page.ImportPathElementsHTML(repoImportPath, importPath, req.URL.RawQuery),
		RepoImportPath:     repoImportPath,
		Commit:             commit,
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

	// Branches.
	if len(branches) != 0 {
		data.Branches = select_menu.New(branches, defaultBranch, req.URL.Query(), gtdo.RevisionQueryParameter)
	}

	// AdditionalTestImports.
	// It is (bpkg.TestImports + bpkg.XTestImports) - bpkg.Imports.
	if fs != nil && bpkg != nil {
		additionalTestImports := make(map[string]struct{})
		for _, ip := range bpkg.TestImports {
			additionalTestImports[ip] = struct{}{}
		}
		for _, ip := range bpkg.XTestImports {
			additionalTestImports[ip] = struct{}{}
		}
		for _, ip := range bpkg.Imports {
			delete(additionalTestImports, ip)
		}

		for ip := range additionalTestImports {
			data.AdditionalTestImports = append(data.AdditionalTestImports, ip)
		}
		sort.Strings(data.AdditionalTestImports)
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

	err = t.ExecuteTemplate(wr, "imports.html.tmpl", &data)
	if err != nil {
		log.Printf("t.ExecuteTemplate: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	afterPackageVisit(bpkg, repoSpec)
}

func docPackage(fs vfs.FileSystem, bpkg *build.Package) (*doc.Package, error) {
	apkg, err := astPackage(fs, bpkg)
	if err != nil {
		return nil, err
	}
	return doc.New(apkg, bpkg.ImportPath, 0), nil
}

func astPackage(fs vfs.FileSystem, bpkg *build.Package) (*ast.Package, error) {
	// TODO: Either find a way to use golang.org/x/tools/importer directly, or do file AST parsing in parallel like it does
	filenames := append(bpkg.GoFiles, bpkg.CgoFiles...)
	files := make(map[string]*ast.File, len(filenames))
	fset := token.NewFileSet()
	for _, filename := range filenames {
		name := filepath.ToSlash(filepath.Join(bpkg.Dir, filename))
		f, err := fs.Open(name)
		if err != nil {
			return nil, err
		}
		fileAst, err := parser.ParseFile(fset, name, f, parser.ParseComments)
		f.Close()
		if err != nil {
			return nil, err
		}
		files[filename] = fileAst // TODO: Figure out if filename or full path are to be used (the key of this map doesn't seem to be used anywhere!)
	}
	return &ast.Package{Name: bpkg.Name, Files: files}, nil
}

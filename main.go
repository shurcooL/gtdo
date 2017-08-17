// gtdo is the source for gotools.org.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/shurcooL/frontend/checkbox"
	"github.com/shurcooL/frontend/select_menu"
	"github.com/shurcooL/go/printerutil"
	"github.com/shurcooL/gtdo/assets"
	"github.com/shurcooL/gtdo/gtdo"
	"github.com/shurcooL/gtdo/internal/sanitizedanchorname"
	"github.com/shurcooL/gtdo/page"
	"github.com/shurcooL/highlight_go"
	"github.com/shurcooL/httpfs/html/vfstemplate"
	"github.com/shurcooL/httpgzip"
	"github.com/shurcooL/octiconssvg"
	"github.com/shurcooL/vcsstate"
	"github.com/sourcegraph/annotate"
	"golang.org/x/net/html"
	"golang.org/x/net/lex/httplex"
	go_vcs "golang.org/x/tools/go/vcs"
	"golang.org/x/tools/godoc/vfs"
	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	_ "sourcegraph.com/sourcegraph/go-vcs/vcs/git"
	_ "sourcegraph.com/sourcegraph/go-vcs/vcs/hg"
)

var (
	httpFlag        = flag.String("http", ":8080", "Listen for HTTP connections on this address.")
	productionFlag  = flag.Bool("production", false, "Production mode.")
	vcsStoreDirFlag = flag.String("vcs-store-dir", "", "Directory of vcs store (required).")
	stateFileFlag   = flag.String("state-file", "", "File to save/load state.")
)

func main() {
	flag.Parse()
	if *vcsStoreDirFlag == "" {
		fmt.Fprintln(os.Stderr, "-vcs-store-dir flag is required")
		flag.Usage()
		os.Exit(2)
	}

	err := loadTemplates()
	if err != nil {
		log.Fatalln("loadTemplates:", err)
	}

	vs = &localVCSStore{dir: *vcsStoreDirFlag}

	LocalGoVersion, err = localGoVersion()
	if err != nil {
		log.Fatalln("no local Go version available:", err)
	}
	fmt.Printf("using local Go version %q\n", LocalGoVersion)

	http.HandleFunc("/", codeHandler)
	http.Handle("/favicon.ico", http.NotFoundHandler())
	http.HandleFunc("/robots.txt", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, `User-agent: *
Allow: /$
Disallow: /

User-agent: MJ12bot
Disallow: /

User-agent: Baiduspider
Disallow: /
`)
	})
	fileServer := httpgzip.FileServer(assets.Assets, httpgzip.FileServerOptions{ServeError: httpgzip.Detailed})
	http.Handle("/assets/", fileServer)
	http.Handle("/assets/frontend.js", http.StripPrefix("/assets", fileServer))
	http.Handle("/assets/selectlistview.css", http.StripPrefix("/assets", fileServer))
	http.Handle("/assets/tableofcontents.css", http.StripPrefix("/assets", fileServer))

	fontsHandler := httpgzip.FileServer(assets.Fonts, httpgzip.FileServerOptions{ServeError: httpgzip.Detailed})
	http.Handle("/assets/fonts/", http.StripPrefix("/assets/fonts", fontsHandler))

	if *stateFileFlag != "" {
		_ = loadState(*stateFileFlag)
	}

	RepoUpdater = NewRepoUpdater()
	defer RepoUpdater.Close()
	sse = make(map[importPathBranch][]pageViewer)
	http.HandleFunc("/-/events", eventsHandler)
	http.Handle("/-/debug", handler(func(w io.Writer, req *http.Request) error {
		fmt.Fprintln(w, "len(RepoUpdater.queue):", len(RepoUpdater.queue))
		fmt.Fprintln(w)
		fmt.Fprintln(w, "events:")
		sseMu.Lock()
		for importPathBranch, pageViewers := range sse {
			fmt.Fprintf(w, "%#v - %v\n", importPathBranch, len(pageViewers))
		}
		if len(sse) == 0 {
			fmt.Fprintf(w, "-")
		}
		sseMu.Unlock()
		return nil
	}))

	server := &http.Server{Addr: *httpFlag, Handler: topMux{}}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	go func() {
		<-interrupt
		err := server.Close()
		if err != nil {
			log.Println("server.Close:", err)
		}
	}()

	log.Println("Started.")

	err = server.ListenAndServe()
	if err != nil {
		log.Println("server.ListenAndServe:", err)
	}

	if *stateFileFlag != "" {
		_ = saveState(*stateFileFlag)
	}
}

var t *template.Template

func loadTemplates() error {
	var err error
	t = template.New("").Funcs(template.FuncMap{
		"commitId":      func(commitId vcs.CommitID) vcs.CommitID { return commitId[:8] },
		"time":          humanize.Time,
		"fullQuery":     fullQuery,
		"importPathURL": importPathURL,
		"octicon": func(name string) (template.HTML, error) {
			icon := octiconssvg.Icon(name)
			if icon == nil {
				return "", fmt.Errorf("%q is not a valid Octicon symbol name", name)
			}
			var buf bytes.Buffer
			err := html.Render(&buf, icon)
			if err != nil {
				return "", err
			}
			return template.HTML(buf.String()), nil
		},

		"json": func(in interface{}) (string, error) {
			out, err := json.Marshal(in)
			return string(out), err
		},
	})
	t, err = vfstemplate.ParseGlob(assets.Assets, t, "/assets/*.tmpl")
	return err
}

func codeHandler(w http.ResponseWriter, req *http.Request) {
	const testsQueryParameter = "tests"

	if !*productionFlag {
		err := loadTemplates()
		if err != nil {
			log.Println("loadTemplates:", err)
			http.Error(w, fmt.Sprintln("loadTemplates:", err), http.StatusInternalServerError)
			return
		}
	}

	if strings.HasPrefix(req.URL.Path, "/apple-touch-icon") {
		http.NotFound(w, req)
		return
	}

	if req.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		recentlyViewed.mu.RLock()
		recentlyViewed.Production = *productionFlag
		err := t.ExecuteTemplate(w, "index.html.tmpl", recentlyViewed)
		recentlyViewed.mu.RUnlock()
		if err != nil {
			log.Printf("t.ExecuteTemplate: %v\n", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	// Redirect "/import/path/" to "/import/path".
	if req.URL.Path != "/" && req.URL.Path[len(req.URL.Path)-1] == '/' {
		baseURL := req.URL.Path[:len(req.URL.Path)-1]
		if req.URL.RawQuery != "" {
			baseURL += "?" + req.URL.RawQuery
		}
		http.Redirect(w, req, baseURL, http.StatusFound)
		return
	}

	importPath := req.URL.Path[1:]
	rev := req.URL.Query().Get(gtdo.RevisionQueryParameter) // rev is the raw revision query parameter as specified by URL.
	_, includeTestFiles := req.URL.Query()[testsQueryParameter]

	log.Printf("req: importPath=%q rev=%q tab=%v, ref=%q, ua=%q\n", importPath, rev, req.URL.Query().Get("tab"), req.Referer(), req.UserAgent())

	switch req.URL.Query().Get("tab") {
	case "summary":
		summaryHandler(w, req, importPath, rev)
		return
	case "imports":
		importsHandler(w, req, importPath, rev)
		return
	case "dependents":
		dependentsHandler(w, req, importPath, rev)
		return
	}

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
		Commit             *vcs.Commit
		DirExists          bool
		Bpkg               *build.Package
		Folders            []string
		Files              template.HTML
		Branches           template.HTML // Select menu for branches.
		Tests              template.HTML // Checkbox for tests.
	}{
		FrontendState:      frontendState,
		Production:         *productionFlag,
		RawQuery:           req.URL.RawQuery,
		Tabs:               page.Tabs(req.URL.Path, req.URL.RawQuery),
		ImportPath:         importPath,
		ImportPathElements: page.ImportPathElementsHTML(repoImportPath, importPath, req.URL.RawQuery),
		Commit:             commit,
		DirExists:          fs != nil,
		Bpkg:               bpkg,
		Tests:              checkbox.New(false, req.URL.Query(), testsQueryParameter),
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

	if bpkg != nil {
		var buf bytes.Buffer

		// Get all .go files, sort by name.
		goFiles := append(bpkg.GoFiles, bpkg.CgoFiles...)
		if includeTestFiles {
			goFiles = append(goFiles, bpkg.TestGoFiles...)
			goFiles = append(goFiles, bpkg.XTestGoFiles...)
		}
		for _, goFile := range bpkg.IgnoredGoFiles {
			isTest := strings.HasSuffix(goFile, "_test.go") // Logic from go/build.
			// When we care about differentiating test files in/outside package,
			// then need to calculate isXTest correctly, likely by doing
			// parser.ParseFile(..., parser.PackageClauseOnly) again.
			if !isTest || includeTestFiles {
				goFiles = append(goFiles, goFile)
			}
		}
		sort.Strings(goFiles)

		for _, goFile := range goFiles {
			fi, err := fs.Stat(path.Join(bpkg.Dir, goFile))
			if err != nil {
				panic(fmt.Errorf("%v: fs.Stat(%q): %v", fs.String(), path.Join(bpkg.Dir, goFile), err))
			}
			file, err := fs.Open(path.Join(bpkg.Dir, goFile))
			if err != nil {
				panic(fmt.Errorf("%v: fs.Open(%q): %v", fs.String(), path.Join(bpkg.Dir, goFile), err))
			}
			src, err := ioutil.ReadAll(file)
			if err != nil {
				panic(err)
			}
			err = file.Close()
			if err != nil {
				panic(err)
			}

			const maxAnnotateSize = 1000 * 1000

			var (
				annSrc           []byte
				shouldHTMLEscape bool
			)
			switch {
			case fi.Size() <= maxAnnotateSize:
				fset := token.NewFileSet()
				fileAst, err := parser.ParseFile(fset, filepath.Join(bpkg.Dir, goFile), src, parser.ParseComments)
				if err != nil {
					log.Println("parser.ParseFile:", err)
				}
				if fileAst == nil {
					panic(fmt.Errorf("internal error: this shouldn't happen as long as parser.ParseFile is still given []byte as src"))
				}

				anns, err := highlight_go.Annotate(src, htmlAnnotator)
				_ = err // TODO: Deal with returned error.

				for _, decl := range fileAst.Decls {
					switch d := decl.(type) {
					case *ast.FuncDecl:
						name := d.Name.String()
						if d.Recv != nil {
							name = strings.TrimPrefix(printerutil.SprintAstBare(d.Recv.List[0].Type), "*") + "." + name
							anns = append(anns, annotateNodes(fset, d.Recv, d.Name, fmt.Sprintf(`<h3 id="%s">`, name), `</h3>`, 1))
						} else {
							anns = append(anns, annotateNode(fset, d.Name, fmt.Sprintf(`<h3 id="%s">`, name), `</h3>`, 1))
						}
						anns = append(anns, annotateNode(fset, d.Name, fmt.Sprintf(`<a href="%s">`, "#"+name), `</a>`, 2))
					case *ast.GenDecl:
						switch d.Tok {
						case token.IMPORT:
							for _, imp := range d.Specs {
								pathLit := imp.(*ast.ImportSpec).Path
								pathValue, err := strconv.Unquote(pathLit.Value)
								if err != nil {
									continue
								}
								url := importPathURL(pathValue, repoImportPath, req.URL.RawQuery)
								anns = append(anns, annotateNode(fset, pathLit, fmt.Sprintf(`<a href="%s">`, url), `</a>`, 1))
							}
						case token.TYPE:
							for _, spec := range d.Specs {
								ident := spec.(*ast.TypeSpec).Name
								anns = append(anns, annotateNode(fset, ident, fmt.Sprintf(`<h3 id="%s">`, ident.String()), `</h3>`, 1))
								anns = append(anns, annotateNode(fset, ident, fmt.Sprintf(`<a href="%s">`, "#"+ident.String()), `</a>`, 2))
							}
						case token.CONST, token.VAR:
							for _, spec := range d.Specs {
								for _, ident := range spec.(*ast.ValueSpec).Names {
									anns = append(anns, annotateNode(fset, ident, fmt.Sprintf(`<h3 id="%s">`, ident.String()), `</h3>`, 1))
									anns = append(anns, annotateNode(fset, ident, fmt.Sprintf(`<a href="%s">`, "#"+ident.String()), `</a>`, 2))
								}
							}
						}
					}
				}

				sort.Sort(anns)

				annSrc, err = annotate.Annotate(src, anns, template.HTMLEscape)
				if err != nil {
					panic(err)
				}
				shouldHTMLEscape = false
			default:
				// Skip annotation for huge files.
				annSrc = src
				shouldHTMLEscape = true
			}

			lineCount := bytes.Count(src, []byte("\n"))
			fmt.Fprintf(&buf, `<div><h2 id="%s">%s<a class="anchor" onclick="MustScrollTo(event, &#34;\&#34;%s\&#34;&#34;);"><span class="anchor-icon">%s</span></a></h2>`, sanitizedanchorname.Create(goFile), html.EscapeString(goFile), sanitizedanchorname.Create(goFile), octiconsLink) // HACK.
			io.WriteString(&buf, `<div class="highlight">`)
			io.WriteString(&buf, `<div class="background"></div>`)
			io.WriteString(&buf, `<div class="selection"></div>`)
			io.WriteString(&buf, `<table cellspacing=0><tr><td><pre class="ln">`)
			for i := 1; i <= lineCount; i++ {
				fmt.Fprintf(&buf, `<span id="%s-L%d" class="ln" onclick="LineNumber(event, &#34;\&#34;%s-L%d\&#34;&#34;);">%d</span>`, sanitizedanchorname.Create(goFile), i, sanitizedanchorname.Create(goFile), i, i)
				buf.WriteString("\n")
			}
			io.WriteString(&buf, `</pre></td><td><pre class="file">`)
			switch shouldHTMLEscape {
			case false:
				buf.Write(annSrc)
			case true:
				template.HTMLEscape(&buf, annSrc)
			}
			io.WriteString(&buf, `</pre></td></tr></table></div></div>`)
		}

		data.Files = template.HTML(buf.String())
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var wr io.Writer = w
	if httplex.HeaderValuesContainsToken(req.Header["Accept-Encoding"], "gzip") {
		// Use gzip compression.
		w.Header().Set("Content-Encoding", "gzip")
		gw := gzip.NewWriter(w)
		defer gw.Close()
		wr = gw
	}

	err = t.ExecuteTemplate(wr, "code.html.tmpl", &data)
	if err != nil {
		log.Printf("t.ExecuteTemplate: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sendToTopMaybe(bpkg)
}

// sendToTopMaybe sends package to top, if bpkg is not nil and doesn't have a conflicting import comment.
func sendToTopMaybe(bpkg *build.Package) {
	if bpkg == nil {
		return
	}
	conflictingImportComment := bpkg.ImportComment != "" && bpkg.ImportComment != bpkg.ImportPath
	log.Printf("ImportComment = %q, conflicting import comment: %v\n", bpkg.ImportComment, conflictingImportComment)
	if bpkg.Name != "" && !conflictingImportComment {
		sendToTop(bpkg.ImportPath)
	}
	// RepoUpdater.Enqueue(*repoSpec) now happens via SSE path, later on.
	// It needs to happen there, so that by the time we may have a response,
	// it can be directly sent. Otherwise we might have an update before the SSE client connected.
}

// Try local first, if not, try remote, if not, clone/update remote and try one last time.
func try(importPath, rev string) (
	source string,
	bpkg *build.Package,
	repoSpec *repoSpec,
	repoImportPath string,
	commit *vcs.Commit,
	fs vfs.FileSystem,
	branchNames []string,
	defaultBranch string,
	err error,
) {
	var repo vcs.Repository
	var commitId vcs.CommitID
	if isLocal(importPath) {
		repo, repoSpec, commitId, defaultBranch, err = tryRemoteGoroot(rev)
		if err != nil {
			return source, nil, nil, "", nil, nil, nil, "", err
		}
		source = "remote-goroot"
		repoImportPath = strings.Split(importPath, "/")[0]
	} else {
		repo, repoSpec, repoImportPath, commitId, defaultBranch, err = tryRemote(importPath, rev)
		if err != nil {
			return source, nil, nil, "", nil, nil, nil, "", err
		}
		source = "remote"
	}

	branchNames, err = branchesAndTags(repo)
	if err != nil {
		return source, nil, nil, "", nil, nil, nil, "", err
	}

	commit, err = repo.GetCommit(commitId)
	if err != nil {
		return source, nil, nil, "", nil, nil, nil, "", err
	}

	fs, err = repo.FileSystem(commitId)
	if err != nil {
		return source, nil, nil, "", nil, nil, nil, "", err
	}

	switch source {
	case "remote-goroot":
		fs = NewPrefixFS(fs, "/virtual-go-workspace")
	default:
		fs = NewPrefixFS(fs, "/virtual-go-workspace/src/"+repoImportPath)
	}

	// Verify the import path is an existing subdirectory (it may exist on one branch, but not another).
	if fi, err := fs.Stat("/virtual-go-workspace/src/" + importPath); err != nil || !fi.IsDir() {
		return source, nil, repoSpec, repoImportPath, nil, nil, branchNames, defaultBranch, nil
	}

	context := buildContextUsingFS(fs)
	switch source {
	case "remote-goroot":
		context.GOROOT = "/virtual-go-workspace"
	default:
		context.GOPATH = "/virtual-go-workspace"
	}
	bpkg, err = context.Import(importPath, "", build.ImportComment)
	_ = err // TODO: Deal with returned error.
	if bpkg == nil || bpkg.Dir == "" {
		return source, nil, repoSpec, repoImportPath, commit, fs, branchNames, defaultBranch, nil
	}

	return source, bpkg, repoSpec, repoImportPath, commit, fs, branchNames, defaultBranch, nil
}

// isLocal reports whether the import path is a package that can only
// be in a local GOROOT or GOPATH, but not available remotely. It checks
// if the first element (i.e., the domain name) contains a dot.
func isLocal(importPath string) bool {
	return !strings.Contains(strings.Split(importPath, "/")[0], ".")
}

func tryRemoteGoroot(rev string) (
	repo vcs.Repository,
	_ *repoSpec,
	commitId vcs.CommitID,
	defaultBranch string,
	err error,
) {
	u, err := url.Parse("https://go.googlesource.com/go")
	if err != nil {
		return nil, nil, "", "", err
	}
	repo, _, err = vs.Repository("git", u)
	if err != nil {
		return nil, nil, "", "", err
	}

	// Use local Go version as default.
	defaultBranch = LocalGoVersion

	var local bool
	if rev != "" {
		local = rev == LocalGoVersion
		commitId, err = repo.ResolveRevision(rev)
	} else {
		local = true
		commitId, err = repo.ResolveTag(defaultBranch)
	}
	if err != nil {
		_, err1 := repo.(vcs.RemoteUpdater).UpdateEverything(vcs.RemoteOpts{})
		fmt.Println("tryRemote: UpdateEverything:", err1)
		if err1 != nil {
			return nil, nil, "", "", NewMultipleErrors(err, err1)
		}

		if rev != "" {
			commitId, err1 = repo.ResolveRevision(rev)
		} else {
			commitId, err1 = repo.ResolveTag(defaultBranch)
		}
		if err1 != nil {
			return nil, nil, "", "", NewMultipleErrors(err, err1)
		}
		fmt.Println("tryRemote: worked on SECOND try")
	} else {
		fmt.Println("tryRemote: worked on first try")
	}

	if local {
		repo = repoWithLocal{
			Repository:     repo,
			localGoVersion: commitId,
		}
	}

	rs := repoSpec{vcsType: "git", cloneURL: "https://go.googlesource.com/go"} // TODO: Avoid having to return a pointer. It's not optional in this context.
	return repo, &rs, commitId, defaultBranch, nil
}

type repoWithLocal struct {
	vcs.Repository
	localGoVersion vcs.CommitID
}

func (r repoWithLocal) FileSystem(at vcs.CommitID) (vfs.FileSystem, error) {
	if at == r.localGoVersion {
		fmt.Println("using LocalGoVersion:", at)
		return vfs.OS(build.Default.GOROOT), nil
	}
	fmt.Println("using vcs repo", at)
	return r.Repository.FileSystem(at)
}

func tryRemote(importPath, rev string) (
	repo vcs.Repository,
	_ *repoSpec,
	repoImportPath string,
	commitId vcs.CommitID,
	defaultBranch string,
	err error,
) {
	if vs == nil {
		return nil, nil, "", "", "", errors.New("no backing vcsstore specified")
	}

	rr, err := go_vcs.RepoRootForImportPath(importPath, true)
	if err != nil {
		return nil, nil, "", "", "", err
	}
	if rr.VCS.Cmd != "git" && rr.VCS.Cmd != "hg" {
		return nil, nil, "", "", "", fmt.Errorf("unsupported rr.VCS.Cmd: %v", rr.VCS.Cmd)
	}

	vcsRepo, err := vcsstate.NewVCS(rr.VCS)
	if err != nil {
		return nil, nil, "", "", "", err
	}

	u, err := url.Parse(rr.Repo)
	if err != nil {
		return nil, nil, "", "", "", err
	}
	var repoDir string
	repo, repoDir, err = vs.Repository(rr.VCS.Cmd, u)
	if err != nil {
		return nil, nil, "", "", "", err
	}

	// Use remotely checked out branch as the default branch for remote repos.
	defaultBranch, _, err = vcsRepo.RemoteBranchAndRevision(repoDir)
	if err != nil {
		return nil, nil, "", "", "", err
	}

	if rev != "" {
		commitId, err = repo.ResolveRevision(rev)
	} else {
		commitId, err = repo.ResolveBranch(defaultBranch)
	}
	if err != nil {
		_, err1 := repo.(vcs.RemoteUpdater).UpdateEverything(vcs.RemoteOpts{})
		fmt.Println("tryRemote: UpdateEverything:", err1)
		if err1 != nil {
			return nil, nil, "", "", "", NewMultipleErrors(err, err1)
		}

		if rev != "" {
			commitId, err1 = repo.ResolveRevision(rev)
		} else {
			commitId, err1 = repo.ResolveBranch(defaultBranch)
		}
		if err1 != nil {
			return nil, nil, "", "", "", NewMultipleErrors(err, err1)
		}
		fmt.Println("tryRemote: worked on SECOND try")
	} else {
		fmt.Println("tryRemote: worked on first try")
	}

	rs := repoSpec{vcsType: rr.VCS.Cmd, cloneURL: rr.Repo} // TODO: Avoid having to return a pointer. It's not optional in this context.
	return repo, &rs, rr.Root, commitId, defaultBranch, nil
}

func buildContextUsingFS(fs vfs.FileSystem) build.Context {
	var context build.Context = build.Default

	//context.GOROOT = ""
	//context.GOPATH = "/"
	context.JoinPath = path.Join
	context.IsAbsPath = path.IsAbs
	context.SplitPathList = func(list string) []string { return strings.Split(list, ":") }
	context.IsDir = func(path string) bool {
		//fmt.Printf("context.IsDir %q\n", path)
		if fi, err := fs.Stat(path); err == nil && fi.IsDir() {
			return true
		}
		return false
	}
	context.HasSubdir = func(root, dir string) (rel string, ok bool) {
		//fmt.Printf("context.HasSubdir %q %q\n", root, dir)
		if context.IsDir(path.Join(root, dir)) {
			return dir, true
		} else {
			return "", false
		}
	}
	context.ReadDir = func(dir string) (fi []os.FileInfo, err error) {
		//fmt.Printf("context.ReadDir %q\n", dir)
		return fs.ReadDir(dir)
	}
	context.OpenFile = func(path string) (r io.ReadCloser, err error) {
		//fmt.Printf("context.OpenFile %q\n", path)
		return fs.Open(path)
	}

	return context
}

// importPathURL returns a URL to the target importPath, preserving query parameters.
//
// It strips out the revision parameter if the target package lies outside of the current repository.
func importPathURL(importPath, repoImportPath string, rawQuery string) template.URL {
	query, _ := url.ParseQuery(rawQuery)
	// If it crosses the repository boundary, do not persist the revision.
	if !packageInsideRepo(importPath, repoImportPath) {
		query.Del(gtdo.RevisionQueryParameter)
	}
	url := url.URL{
		Path:     "/" + importPath,
		RawQuery: query.Encode(),
	}
	return template.URL(url.String())
}

// packageInsideRepo returns true iff importPath package is inside repository repoImportPath.
func packageInsideRepo(importPath, repoImportPath string) bool {
	return strings.HasPrefix(importPath, repoImportPath)
}

// fullQuery returns rawQuery with a "?" prefix if rawQuery is non-empty.
func fullQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	return "?" + rawQuery
}

// NewMultipleErrors returns an error that consists of 2 or more errors.
func NewMultipleErrors(err0, err1 error, errs ...error) error {
	return append(multipleErrors{err0, err1}, errs...)
}

// multipleErrors should consist of 2 or more errors.
type multipleErrors []error

func (me multipleErrors) Error() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%d errors:\n", len(me))
	for _, err := range me {
		fmt.Fprintln(&buf, err.Error())
	}
	return buf.String()
}

var octiconsLink = func() string {
	var buf bytes.Buffer
	err := html.Render(&buf, octiconssvg.Link())
	if err != nil {
		panic(err)
	}
	return buf.String()
}()

// topMux adds some instrumentation on top of http.DefaultServeMux.
type topMux struct{}

func (topMux) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	started := time.Now()
	rw := &responseWriter{ResponseWriter: w}
	http.DefaultServeMux.ServeHTTP(rw, req)
	fmt.Printf("TIMING: %s: %v\n", path, time.Since(started))
	if path != req.URL.Path {
		log.Printf("warning: req.URL.Path was modified from %v to %v\n", path, req.URL.Path)
	}
	if rw.WroteBytes && !haveType(w) {
		log.Printf("warning: Content-Type header not set for %q\n", path)
	}
}

// haveType reports whether w has the Content-Type header set.
func haveType(w http.ResponseWriter) bool {
	_, ok := w.Header()["Content-Type"]
	return ok
}

// responseWriter wraps a real http.ResponseWriter and captures
// whether any bytes were written.
type responseWriter struct {
	http.ResponseWriter

	WroteBytes bool // Whether non-zero bytes have been written.
}

func (rw *responseWriter) Write(p []byte) (n int, err error) {
	if len(p) > 0 {
		rw.WroteBytes = true
	}
	return rw.ResponseWriter.Write(p)
}

func (rw *responseWriter) Flush() {
	rw.ResponseWriter.(http.Flusher).Flush()
}

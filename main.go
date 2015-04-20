package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
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
	"syscall"

	"go/ast"
	"go/build"
	"go/parser"
	"go/token"

	"github.com/shurcooL/frontend/checkbox"
	"github.com/shurcooL/frontend/select_menu"
	"github.com/shurcooL/go/gists/gist5639599"
	"github.com/shurcooL/go/gists/gist7390843"
	"github.com/shurcooL/go/gists/gist7480523"
	"github.com/shurcooL/go/gopherjs_http"
	"github.com/shurcooL/go/highlight_go"
	vcs2 "github.com/shurcooL/go/vcs"
	"github.com/shurcooL/go/vfs_util"
	"github.com/shurcooL/sanitized_anchor_name"
	"github.com/sourcegraph/annotate"
	"github.com/sourcegraph/httpcache"
	"github.com/sourcegraph/syntaxhighlight"
	"golang.org/x/net/html"
	go_vcs "golang.org/x/tools/go/vcs"
	"golang.org/x/tools/godoc/vfs"
	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	_ "sourcegraph.com/sourcegraph/go-vcs/vcs/gitcmd"
	_ "sourcegraph.com/sourcegraph/go-vcs/vcs/hgcmd"
	"sourcegraph.com/sourcegraph/vcsstore/vcsclient"
)

var httpFlag = flag.String("http", ":8080", "Listen for HTTP connections on this address.")
var productionFlag = flag.Bool("production", false, "Production mode.")
var vcsstoreHostFlag = flag.String("vcsstore-host", "localhost:9090", "Host of backing vcsstore.")
var stateFileFlag = flag.String("state-file", "", "File to save/load state.")

var sg *vcsclient.Client

var t *template.Template

func loadTemplates() error {
	var err error
	t = template.New("").Funcs(template.FuncMap{})
	t, err = t.ParseGlob("./assets/*.tmpl")
	return err
}

func main() {
	flag.Parse()

	err := loadTemplates()
	if err != nil {
		log.Fatalln("loadTemplates:", err)
	}

	// TODO: This likely has room for improvement, investigate this carefully and improve.
	transport := httpcache.NewMemoryCacheTransport()
	cacheClient := &http.Client{Transport: transport}

	sg = vcsclient.New(&url.URL{Scheme: "http", Host: *vcsstoreHostFlag}, cacheClient)
	sg.UserAgent = "gotools.org backend " + sg.UserAgent

	http.HandleFunc("/", codeHandler)
	http.Handle("/assets/", http.FileServer(http.Dir(".")))

	// Dev, hot reload.
	/*http.Handle("/command-r.go.js", gopherjs_http.GoFiles("../frontend/select-list-view/main.go"))
	http.HandleFunc("/command-r.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../frontend/select-list-view/style.css")
	})
	http.Handle("/table-of-contents.go.js", gopherjs_http.GoFiles("../frontend/table-of-contents/main.go"))
	http.HandleFunc("/table-of-contents.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../frontend/table-of-contents/style.css")
	})*/

	// HACK: Prod, static.
	http.Handle("/favicon.ico/", http.NotFoundHandler())
	http.HandleFunc("/robots.txt", func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, `User-agent: *
Disallow: /
`)
	})
	http.HandleFunc("/command-r.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../frontend/select-list-view/style.css")
	})
	http.HandleFunc("/table-of-contents.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../frontend/table-of-contents/style.css")
	})
	http.Handle("/script.go.js", gopherjs_http.StaticGoFiles("./assets/script.go"))

	if *stateFileFlag != "" {
		_ = loadState(*stateFileFlag)
	}

	stopServerChan := make(chan struct{})
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-signalChan
		stopServerChan <- struct{}{}
	}()

	err = gist7390843.ListenAndServeStoppable(*httpFlag, nil, stopServerChan)
	if err != nil {
		log.Println("ListenAndServeStoppable:", err)
	}

	if *stateFileFlag != "" {
		_ = saveState(*stateFileFlag)
	}
}

func codeHandler(w http.ResponseWriter, req *http.Request) {
	const (
		revisionQueryParameter = "rev"
		testsQueryParameter    = "tests"
	)

	if !*productionFlag {
		err := loadTemplates()
		if err != nil {
			log.Println("loadTemplates:", err)
			http.Error(w, fmt.Sprintln("loadTemplates:", err), http.StatusInternalServerError)
			return
		}
	}

	if req.URL.Path != "/" && req.URL.Path[len(req.URL.Path)-1] == '/' {
		http.Redirect(w, req, req.URL.Path[:len(req.URL.Path)-1], http.StatusFound)
		return
	}

	if req.URL.Path == "/" {
		recentlyViewed.lock.RLock()
		recentlyViewed.Production = *productionFlag
		err := t.ExecuteTemplate(w, "index.html.tmpl", recentlyViewed)
		recentlyViewed.lock.RUnlock()
		if err != nil {
			log.Printf("t.ExecuteTemplate: %v\n", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	importPath := req.URL.Path[1:]
	rev := req.URL.Query().Get(revisionQueryParameter)
	_, includeTestFiles := req.URL.Query()[testsQueryParameter]

	log.Printf("req: importPath=%q rev=%q.\n", importPath, rev)

	source, bpkg, repoImportPath, fs, branches, defaultBranch, err := try(importPath, rev)
	log.Println("using source:", source)
	if err != nil {
		log.Println("try:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		Production         bool
		ImportPath         string
		ImportPathElements template.HTML // Import path with linkified elements.
		DirExists          bool
		Bpkg               *build.Package
		Folders            []string
		Files              template.HTML
		Branches           template.HTML // Select menu for branches.
		Tests              template.HTML // Checkbox for tests.
	}{
		Production:         *productionFlag,
		ImportPath:         importPath,
		ImportPathElements: ImportPathElementsHtml(repoImportPath, importPath, req.URL.RawQuery),
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
		data.Branches = select_menu.New(branches, defaultBranch, req.URL.Query(), revisionQueryParameter)
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
			fset := token.NewFileSet()
			file, err := fs.Open(path.Join(bpkg.Dir, goFile))
			if err != nil {
				log.Panicln(fs.String(), "fs.Open:", path.Join(bpkg.Dir, goFile), err)
			}
			src, err := ioutil.ReadAll(file)
			if err != nil {
				panic(err)
			}
			err = file.Close()
			if err != nil {
				panic(err)
			}
			fileAst, err := parser.ParseFile(fset, filepath.Join(bpkg.Dir, goFile), src, parser.ParseComments)
			if err != nil {
				panic(err)
			}

			anns, err := highlight_go.Annotate(src, syntaxhighlight.HTMLAnnotator(syntaxhighlight.DefaultHTMLConfig))

			for _, decl := range fileAst.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					name := d.Name.String()
					if d.Recv != nil {
						name = strings.TrimPrefix(gist5639599.SprintAstBare(d.Recv.List[0].Type), "*") + "." + name
						anns = append(anns, annotateNodes(fset, d.Recv, d.Name, fmt.Sprintf(`<h3 id="%s">`, name), `</h3>`, 1))
					} else {
						anns = append(anns, annotateNode(fset, d.Name, fmt.Sprintf(`<h3 id="%s">`, name), `</h3>`, 1))
					}
					anns = append(anns, annotateNode(fset, d.Name, fmt.Sprintf(`<a href="%s">`, "#"+name), `</a>`, 2))
				case *ast.GenDecl:
					switch d.Tok {
					case token.IMPORT:
						for _, imp := range d.Specs {
							path := imp.(*ast.ImportSpec).Path
							pathValue, err := strconv.Unquote(path.Value)
							if err != nil {
								continue
							}
							values := req.URL.Query()
							// If it crosses the repository boundary, do not persist the revision.
							if !packageInsideRepo(pathValue, repoImportPath) {
								delete(values, revisionQueryParameter)
							}
							url := url.URL{
								Path:     "/" + pathValue,
								RawQuery: values.Encode(),
							}
							anns = append(anns, annotateNode(fset, path, fmt.Sprintf(`<a href="%s">`, url.String()), `</a>`, 1))
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

			b, err := annotate.Annotate(src, anns, func(w io.Writer, b []byte) { template.HTMLEscape(w, b) })
			if err != nil {
				panic(err)
			}

			fmt.Fprintf(&buf, `<h2 id="%s">%s<a class="anchor" href="#%s"><span class="anchor-icon octicon"></span></a></h2>`, sanitized_anchor_name.Create(goFile), html.EscapeString(goFile), sanitized_anchor_name.Create(goFile))
			io.WriteString(&buf, `<div class="highlight highlight-Go"><pre>`)
			buf.Write(b)
			io.WriteString(&buf, `</pre></div>`)
		}

		data.Files = template.HTML(buf.String())
	}

	// Use gzip compression.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Encoding", "gzip") // TODO: Check "Accept-Encoding"?
	gw := gzip.NewWriter(w)
	defer gw.Close()

	err = t.ExecuteTemplate(gw, "code.html.tmpl", &data)
	if err != nil {
		log.Printf("t.ExecuteTemplate: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if bpkg != nil {
		sendToTop(bpkg.ImportPath)
	}
}

// Try local first, if not, try remote, if not, clone/update remote and try one last time.
func try(importPath, rev string) (source string, bpkg *build.Package, repoImportPath string, fs vfs.FileSystem, branchNames []string, defaultBranch string, err error) {
	var repo vcs.Repository
	var commitId vcs.CommitID
	if bpkg, fs, err = tryLocalGoroot(importPath, rev); err == nil {
		// Use local GOROOT package.
		source = "goroot"
		repoImportPath = strings.Split(importPath, "/")[0]
		return source, bpkg, repoImportPath, fs, nil, "", nil
	} else if repo, repoImportPath, commitId, defaultBranch, err = tryLocalGopath(importPath, rev); err == nil {
		// Use local GOPATH package.
		source = "gopath"
	} else if repo, repoImportPath, commitId, defaultBranch, err = tryRemote(importPath, rev); err == nil { // If local didn't work, try remote...
		// Use remote.
		source = "remote"
	} else {
		return source, nil, "", nil, nil, "", err
	}

	branches, err := repo.Branches()
	if err != nil {
		return source, nil, "", nil, nil, "", err
	}
	branchNames = make([]string, len(branches))
	for i, branch := range branches {
		branchNames[i] = branch.Name
	}
	sort.Strings(branchNames)

	fs, err = repo.FileSystem(commitId)
	if err != nil {
		return source, nil, "", nil, nil, "", err
	}

	// This adapter is needed to make fs.Open("/main.go") work, since the local repo's vfs only allows fs.Open("main.go").
	// See https://github.com/sourcegraph/go-vcs/issues/23.
	fs = vfs_util.NewRootedFS(fs)

	fs = vfs_util.NewPrefixFS(fs, "/virtual-go-workspace/src/"+repoImportPath)

	// Verify the import path is an existing subdirectory (it may exist on one branch, but not another).
	if _, err := fs.Stat("/virtual-go-workspace/src/" + importPath); err != nil {
		return source, nil, repoImportPath, nil, branchNames, defaultBranch, nil
	}

	context := buildContextUsingFS(fs)
	context.GOPATH = "/virtual-go-workspace"
	bpkg, err = context.Import(importPath, "", 0)
	if err != nil {
		return source, nil, repoImportPath, fs, branchNames, defaultBranch, nil
	}

	return source, bpkg, repoImportPath, fs, branchNames, defaultBranch, nil
}

func tryLocalGoroot(importPath, rev string) (bpkg *build.Package, fs vfs.FileSystem, err error) {
	fs = vfs.OS(filepath.Join(build.Default.GOROOT, "src"))

	// Verify it's an existing folder in GOROOT.
	if _, err := fs.Stat(importPath); err != nil {
		return nil, nil, errors.New("package is not in GOROOT")
	}

	fs = vfs_util.NewPrefixFS(fs, "/virtual-go-workspace/src")

	context := buildContextUsingFS(fs)
	context.GOROOT = "/virtual-go-workspace"
	bpkg, err1 := context.Import(importPath, "", 0)
	if err1 == nil {
		return bpkg, fs, nil
	}

	return nil, fs, nil
}

func tryLocalGopath(importPath, rev string) (repo vcs.Repository, repoImportPath string, commitId vcs.CommitID, defaultBranch string, err error) {
	if *productionFlag {
		// Disable local for GOPATH packages in production.
		return nil, "", "", "", errors.New("local for GOPATH packages is disabled")
	}

	goPackage := gist7480523.GoPackageFromImportPath(importPath)
	if goPackage == nil {
		return nil, "", "", "", errors.New("no local go package")
	}

	if goPackage.Bpkg.Goroot {
		return nil, "", "", "", errors.New("package in GOROOT, but we're looking in GOPATH only")
	}

	goPackage.UpdateVcs()
	if goPackage.Dir.Repo == nil {
		return nil, "", "", "", errors.New("no local vcs root path")
	}
	rootPath := goPackage.Dir.Repo.Vcs.RootPath()
	repoImportPath = gist7480523.GetRepoImportPath(rootPath, goPackage.Bpkg.SrcRoot)

	repo, err = vcs.Open(goPackage.Dir.Repo.Vcs.Type().VcsType(), rootPath)
	if err != nil {
		return nil, "", "", "", err
	}

	if rev != "" {
		commitId, err = repo.ResolveRevision(rev)
	} else {
		commitId, err = repo.ResolveBranch(goPackage.Dir.Repo.Vcs.GetDefaultBranch())
	}
	if err != nil {
		return nil, "", "", "", err
	}

	// Verify it's an existing revision, etc.
	{
		fs, err := repo.FileSystem(commitId)
		if err != nil {
			return nil, "", "", "", err
		}

		if _, err := fs.Stat("."); err != nil {
			return nil, "", "", "", err
		}
	}

	return repo, repoImportPath, commitId, goPackage.Dir.Repo.Vcs.GetDefaultBranch(), nil
}

func tryRemote(importPath, rev string) (repo vcs.Repository, repoImportPath string, commitId vcs.CommitID, defaultBranch string, err error) {
	repoImportPath, cloneUrl, vcsRepo, err := importPathToRepoRoot(importPath)
	if err != nil {
		return nil, "", "", "", err
	}

	repo, err = sg.Repository(vcsRepo.Type().VcsType(), cloneUrl)
	if err != nil {
		return nil, "", "", "", err
	}

	if rev != "" {
		commitId, err = repo.ResolveRevision(rev)
	} else {
		commitId, err = repo.ResolveBranch(vcsRepo.GetDefaultBranch())
	}
	if err != nil {
		err1 := repo.(vcsclient.RepositoryCloneUpdater).CloneOrUpdate(vcs.RemoteOpts{})
		fmt.Println("tryRemote: CloneOrUpdate:", err1)
		if err1 != nil {
			return nil, "", "", "", MultiError{err, err1}
		}

		if rev != "" {
			commitId, err1 = repo.ResolveRevision(rev)
		} else {
			commitId, err1 = repo.ResolveBranch(vcsRepo.GetDefaultBranch())
		}
		if err1 != nil {
			return nil, "", "", "", MultiError{err, err1}
		}
		fmt.Println("tryRemote: worked on SECOND try")
	} else {
		fmt.Println("tryRemote: worked on first try")
	}

	return repo, repoImportPath, commitId, vcsRepo.GetDefaultBranch(), nil
}

func importPathToRepoRoot(importPath string) (repoImportPath string, cloneUrl *url.URL, vcsRepo vcs2.Vcs, err error) {
	rr, err := go_vcs.RepoRootForImportPath(importPath, true)
	if err != nil {
		return "", nil, nil, err
	}

	repoImportPath = rr.Root

	cloneUrl, err = url.Parse(rr.Repo)
	if err != nil {
		return "", nil, nil, err
	}

	switch rr.VCS.Cmd {
	case "git":
		vcsRepo = vcs2.NewFromType(vcs2.Git)
	case "hg":
		vcsRepo = vcs2.NewFromType(vcs2.Hg)
	default:
		return "", nil, nil, errors.New("unsupported rr.VCS.Cmd: " + rr.VCS.Cmd)
	}

	return repoImportPath, cloneUrl, vcsRepo, nil
}

func buildContextUsingFS(fs vfs.FileSystem) build.Context {
	var context build.Context = build.Default

	//context.GOROOT = ""
	//context.GOPATH = "/"
	context.JoinPath = path.Join
	context.IsAbsPath = path.IsAbs
	context.SplitPathList = func(list string) []string { return strings.Split(list, ":") }
	context.IsDir = func(path string) bool {
		fmt.Printf("context.IsDir %q\n", path)
		if fi, err := fs.Stat(path); err == nil && fi.IsDir() {
			return true
		}
		return false
	}
	context.HasSubdir = func(root, dir string) (rel string, ok bool) {
		fmt.Printf("context.HasSubdir %q %q\n", root, dir)
		if context.IsDir(path.Join(root, dir)) {
			return dir, true
		} else {
			return "", false
		}
	}
	context.ReadDir = func(dir string) (fi []os.FileInfo, err error) {
		fmt.Printf("context.ReadDir %q\n", dir)
		return fs.ReadDir(dir)
	}
	context.OpenFile = func(path string) (r io.ReadCloser, err error) {
		fmt.Printf("context.OpenFile %q\n", path)
		return fs.Open(path)
	}

	return context
}

// packageInsideRepo returns true iff importPath package is inside repository repoImportPath.
func packageInsideRepo(importPath, repoImportPath string) bool {
	return strings.HasPrefix(importPath, repoImportPath)
}

// ---

type MultiError []error

func (me MultiError) Error() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%d errors:\n", len(me))
	for _, err := range me {
		fmt.Fprintln(&buf, err.Error())
	}
	return buf.String()
}

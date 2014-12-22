package main

import (
	"bytes"
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
	"time"

	"go/ast"
	"go/build"
	"go/parser"
	"go/token"

	"github.com/shurcooL/frontend/select_menu"
	"github.com/shurcooL/go/gists/gist5639599"
	"github.com/shurcooL/go/gists/gist7390843"
	"github.com/shurcooL/go/gists/gist7480523"
	"github.com/shurcooL/go/github_flavored_markdown/sanitized_anchor_name"
	"github.com/shurcooL/go/gopherjs_http"
	"github.com/shurcooL/go/highlight_go"
	"github.com/shurcooL/go/html_gen"
	vcs2 "github.com/shurcooL/go/vcs"
	"github.com/shurcooL/go/vfs_util"
	"github.com/sourcegraph/annotate"
	"github.com/sourcegraph/apiproxy"
	"github.com/sourcegraph/apiproxy/service/github"
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

	// TODO: This is probably not completely right as it's GitHub-specific; fix that.
	transport := &apiproxy.RevalidationTransport{
		Transport: httpcache.NewMemoryCacheTransport(),
		Check: (&githubproxy.MaxAge{
			User:         time.Hour * 24,
			Repository:   time.Hour * 24,
			Repositories: time.Hour * 24,
			Activity:     time.Hour * 12,
		}).Validator(),
	}
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

	importPath := req.URL.Path[1:]
	rev := req.URL.Query().Get("rev")
	_, _ = importPath, rev

	log.Printf("req: importPath=%q rev=%q.\n", importPath, rev)

	if importPath == "" {
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

	bpkg, repoImportPath, fs, branches, defaultBranch, err := try(importPath, rev)
	if err != nil {
		log.Println("try:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		Production         bool
		ImportPath         string
		ImportPathElements template.HTML // Import path with linkified elements.
		Bpkg               *build.Package
		Folders            []string
		Files              template.HTML
		Branches           template.HTML // Select menu for branches.
	}{
		Production: *productionFlag,
		ImportPath: importPath,
		Bpkg:       bpkg,
	}

	// For now, don't try to find the subfolders for standard Go packages.
	if bpkg != nil && bpkg.Goroot {
		data.ImportPathElements, err = html_gen.RenderNodes(html_gen.Text(importPath))
		if err != nil {
			panic(err)
		}
	} else {
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

		data.ImportPathElements = ImportPathElementsHtml(repoImportPath, importPath)
	}

	// Branches.
	if len(branches) != 0 {
		data.Branches = select_menu.New(branches, defaultBranch, req.URL.Query(), "rev")
	}

	if bpkg != nil {
		var buf bytes.Buffer

		// Get all .go files, sort by name.
		// For now, do not include any test files.
		goFiles := append(bpkg.GoFiles, bpkg.CgoFiles...)
		for _, goFile := range bpkg.IgnoredGoFiles {
			isTest := strings.HasSuffix(goFile, "_test.go") // Logic from go/build.
			if !isTest {
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
					}
					anns = append(anns, annotateNode(fset, d.Name, fmt.Sprintf(`<h3 id="%s">`, name), `</h3>`, 1))
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
							anns = append(anns, annotateNode(fset, path, fmt.Sprintf(`<a href="%s" target="_blank">`, "/"+pathValue), `</a>`, 1))
						}
					case token.TYPE:
						for _, spec := range d.Specs {
							name := spec.(*ast.TypeSpec).Name.String()
							anns = append(anns, annotateNode(fset, spec.(*ast.TypeSpec).Name, fmt.Sprintf(`<h3 id="%s">`, name), `</h3>`, 1))
						}
					}
				}
			}

			sort.Sort(anns)

			b, err := annotate.Annotate(src, anns, func(w io.Writer, b []byte) { template.HTMLEscape(w, b) })
			if err != nil {
				panic(err)
			}

			fmt.Fprintf(&buf, "<h2 id=\"%s\">%s<a class=\"anchor\" href=\"#%s\"><span class=\"anchor-icon octicon\"></span></a></h2>", sanitized_anchor_name.Create(goFile), html.EscapeString(goFile), sanitized_anchor_name.Create(goFile))
			io.WriteString(&buf, `<div class="highlight highlight-Go"><pre>`)
			buf.Write(b)
			io.WriteString(&buf, `</pre></div>`)
		}

		data.Files = template.HTML(buf.String())
	}

	err = t.ExecuteTemplate(w, "code.html.tmpl", &data)
	if err != nil {
		log.Printf("t.ExecuteTemplate: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if bpkg != nil {
		sendToTop(bpkg.ImportPath)
	}
}

func tryLocal(importPath, rev string) (*build.Package, string, vfs.FileSystem, error) {
	goPackage := gist7480523.GoPackageFromImportPath(importPath)
	if goPackage == nil {
		return nil, "", nil, errors.New("no local go package")
	}

	if goPackage.Bpkg.Goroot {
		if rev != "" {
			return nil, "", nil, errors.New("custom revision not yet supported for GOROOT packages")
		}

		return goPackage.Bpkg, "", vfs.OS(""), nil
	}

	// TESTING: Disable local for non-standard library packages.
	return nil, "", nil, errors.New("TESTING: local for non-standard library packages is disabled")

	/*goPackage.UpdateVcs()
	if goPackage.Dir.Repo == nil {
		return nil, "", nil, errors.New("no local vcs root path")
	}
	rootPath := goPackage.Dir.Repo.Vcs.RootPath()

	repo, err := vcs.Open(goPackage.Dir.Repo.Vcs.Type().VcsType(), rootPath)
	if err != nil {
		return nil, "", nil, err
	}

	var commitId vcs.CommitID
	if rev != "" {
		commitId, err = repo.ResolveRevision(rev)
	} else {
		commitId, err = repo.ResolveBranch(goPackage.Dir.Repo.Vcs.GetDefaultBranch())
	}
	if err != nil {
		return nil, "", nil, err
	}

	fs, err := repo.FileSystem(commitId)
	if err != nil {
		return nil, "", nil, err
	}

	// Verify it's an existing revision, etc.
	_, err = fs.Stat(".")
	if err != nil {
		return nil, "", nil, err
	}

	// This adapter is needed to make fs.Open("/main.go") work, since the repo's vfs only allows fs.Open("main.go").
	// See https://github.com/sourcegraph/go-vcs/issues/23.
	fs = vfs_util.NewRootedFS(fs)

	repoImportPath := gist7480523.GetRepoImportPath(rootPath, goPackage.Bpkg.SrcRoot)
	fs = vfs_util.NewPrefixFS(fs, "/virtual-go-workspace/src/"+repoImportPath)

	context := buildContextUsingFS(fs)
	context.GOPATH = "/virtual-go-workspace"
	bpkg, err := context.Import(importPath, "", 0)
	return bpkg, repoImportPath, fs, err*/
}

// Try local first, if not, try remote, if not, clone/update remote and try one last time.
func try(importPath, rev string) (*build.Package, string, vfs.FileSystem, []string, string, error) {
	bpkg, repoImportPath, fs, err0 := tryLocal(importPath, rev)
	fmt.Println("tryLocal err:", err0)
	if err0 == nil {
		return bpkg, repoImportPath, fs, nil, "", nil
	}

	// If local didn't work, try remote...
	repo, repoImportPath, commitId, defaultBranch, err := repoFromRequest(importPath, rev)
	if err != nil {
		return nil, "", nil, nil, "", err
	}

	branches, err := repo.Branches()
	if err != nil {
		return nil, "", nil, nil, "", err
	}
	// Sort branches?
	branchNames := make([]string, len(branches))
	for i, branch := range branches {
		branchNames[i] = branch.Name
	}

	fs, err = repo.FileSystem(commitId)
	if err != nil {
		return nil, "", nil, nil, "", err
	}

	fs = vfs_util.NewPrefixFS(fs, "/virtual-go-workspace/src/"+repoImportPath)

	context := buildContextUsingFS(fs)
	context.GOPATH = "/virtual-go-workspace"
	bpkg, err1 := context.Import(importPath, "", 0)
	if err1 == nil {
		return bpkg, repoImportPath, fs, branchNames, defaultBranch, nil
	}

	return nil, repoImportPath, fs, branchNames, defaultBranch, nil
}

func importPathToRepoGuess(importPath string) (repoImportPath string, cloneUrl *url.URL, vcsRepo vcs2.Vcs, err error) {
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

func repoFromRequest(importPath, rev string) (repo vcs.Repository, repoImportPath string, commitId vcs.CommitID, defaultBranch string, err error) {
	repoImportPath, cloneUrl, vcsRepo, err := importPathToRepoGuess(importPath)
	if err != nil {
		return nil, "", "", "", err
	}

	//goon.DumpExpr(repoImportPath, cloneUrl, vcsRepo, err)

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
		fmt.Println("repoFromRequest: CloneOrUpdate:", err1)
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
		fmt.Println("repoFromRequest: worked on SECOND try")
	} else {
		fmt.Println("repoFromRequest: worked on first try")
	}

	return repo, repoImportPath, commitId, vcsRepo.GetDefaultBranch(), nil
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

// ---

func ImportPathElementsHtml(repoImportPath, importPath string) template.HTML {
	// Elements of importPath, first element being repoImportPath.
	// E.g., {"github.com/user/repo", "subpath", "package"}.
	elements := []string{repoImportPath}
	elements = append(elements, strings.Split(importPath[len(repoImportPath):], "/")[1:]...)

	var ns []*html.Node
	for i, element := range elements {
		if i != 0 {
			ns = append(ns, html_gen.Text("/"))
		}

		path := path.Join(elements[:i+1]...)

		// Don't link last importPath element, since it's the current page.
		if path != importPath {
			ns = append(ns, html_gen.A(element, template.URL("/"+path)))
		} else {
			ns = append(ns, html_gen.Text(element))
		}
	}

	importPathElements, err := html_gen.RenderNodes(ns...)
	if err != nil {
		panic(err)
	}
	return importPathElements
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

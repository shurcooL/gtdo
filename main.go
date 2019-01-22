// gtdo is the source for gotools.org.
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
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
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/shurcooL/frontend/select_menu"
	"github.com/shurcooL/go/printerutil"
	"github.com/shurcooL/gtdo/assets"
	"github.com/shurcooL/gtdo/gtdo"
	"github.com/shurcooL/gtdo/internal/sanitizedanchorname"
	"github.com/shurcooL/gtdo/page"
	"github.com/shurcooL/highlight_go"
	"github.com/shurcooL/httpfs/html/vfstemplate"
	"github.com/shurcooL/httpgzip"
	"github.com/shurcooL/octicon"
	"github.com/sourcegraph/annotate"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/html"
	"golang.org/x/net/http/httpguts"
	_ "sourcegraph.com/sourcegraph/go-vcs/vcs/git"
	_ "sourcegraph.com/sourcegraph/go-vcs/vcs/hg"
)

var (
	httpFlag          = flag.String("http", ":8080", "Listen for HTTP connections on this address.")
	autocertFlag      = flag.String("autocert", "", `If non-empty, use autocert with the specified domain (e.g., -autocert="example.com").`)
	productionFlag    = flag.Bool("production", false, "Production mode.")
	analyticsFileFlag = flag.String("analytics-file", "", "Optional path to file containing analytics HTML to insert at the beginning of <head>.")
)

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		cancel()
	}()

	err := run(ctx, *analyticsFileFlag)
	if err != nil {
		log.Fatalln(err)
	}
}

func run(ctx context.Context, analyticsFile string) error {
	var analyticsHTML []byte
	if analyticsFile != "" {
		var err error
		analyticsHTML, err = ioutil.ReadFile(analyticsFile)
		if err != nil {
			return err
		}
	}

	err := loadTemplates()
	if err != nil {
		return fmt.Errorf("loadTemplates: %v", err)
	}

	h := &handler{
		analyticsHTML: template.HTML(analyticsHTML),
	}
	http.HandleFunc("/", h.codeHandler)
	http.Handle("/favicon.ico", http.NotFoundHandler())
	http.HandleFunc("/robots.txt", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, `User-agent: *
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

	server := &http.Server{Addr: *httpFlag, Handler: topMux{}}

	go func() {
		<-ctx.Done()
		err := server.Close()
		if err != nil {
			log.Println("server.Close:", err)
		}
	}()

	log.Println("Starting HTTP server.")

	switch *autocertFlag {
	case "":
		err := server.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Println("server.ListenAndServe:", err)
		}
	default:
		err := server.Serve(autocert.NewListener(*autocertFlag))
		if err != http.ErrServerClosed {
			log.Println("server.Serve:", err)
		}
	}

	log.Println("Ended HTTP server.")

	return nil
}

var t *template.Template

func loadTemplates() error {
	var err error
	t = template.New("").Funcs(template.FuncMap{
		"time":      humanize.Time,
		"fullQuery": fullQuery,
		//"importPathURL": importPathURL,
		"octicon": func(name string) (template.HTML, error) {
			icon := octicon.Icon(name)
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

type handler struct {
	analyticsHTML template.HTML
}

func (h *handler) codeHandler(w http.ResponseWriter, req *http.Request) {
	if strings.HasPrefix(req.URL.Path, "/apple-touch-icon") {
		http.NotFound(w, req)
		return
	}

	switch req.UserAgent() {
	case "Mozilla/5.0 (compatible; Baiduspider/2.0; +http://www.baidu.com/search/spider.html)":
		log.Printf("blocked request to %q from Baiduspider\n", req.URL.String())
		http.Error(w, "403 Forbidden\n\nsee robots.txt", http.StatusForbidden)
		return
	case "Mozilla/5.0 (compatible; AlphaBot/3.2; +http://alphaseobot.com/bot.html)":
		log.Printf("blocked request to %q from AlphaBot\n", req.URL.String())
		http.Error(w, "403 Forbidden\n\nsee robots.txt", http.StatusForbidden)
		return
	}

	if !*productionFlag {
		err := loadTemplates()
		if err != nil {
			log.Println("loadTemplates:", err)
			http.Error(w, fmt.Sprintln("loadTemplates:", err), http.StatusInternalServerError)
			return
		}
	}

	if req.URL.Path == "/" {
		http.Error(w, "404 Not Found", http.StatusNotFound)
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
	const testsQueryParameter = "tests"
	_, includeTestFiles := req.URL.Query()[testsQueryParameter]

	log.Printf("req: importPath=%q rev=%q tab=%v, ref=%q, ua=%q\n", importPath, rev, req.URL.Query().Get("tab"), req.Referer(), req.UserAgent())

	/*_, bpkg, _, repoImportPath, commit, fs, branches, defaultBranch, err := try(importPath, rev)
	if err != nil {
		log.Println("try:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}*/

	// Make a temporary directory.
	tempDir, err := ioutil.TempDir("", "gtdo_space_")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Printf("temp dir: %q\n", tempDir)
	//defer os.RemoveAll(tempDir)

	// Initialize an empty module in the temporary directory.
	err = ioutil.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module temp\n"), 0600)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Resolve package@query -> module@version.
	importPathRev := importPath
	if rev != "" {
		importPathRev += "@" + rev
	}
	cmd := exec.CommandContext(req.Context(), "go", "get", "-d", importPathRev)
	cmd.Dir = tempDir
	err = cmd.Run()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get module@version.
	cmd = exec.CommandContext(req.Context(), "go", "list", "-json", importPath)
	cmd.Dir = tempDir
	out, err := cmd.Output()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var pkgInfo struct {
		Dir    string
		Module struct {
			Path    string
			Version string
			Time    time.Time
		}
	}
	err = json.Unmarshal(out, &pkgInfo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// List module versions.
	cmd = exec.CommandContext(req.Context(), "go", "list", "-json", "-m", "-versions", pkgInfo.Module.Path)
	cmd.Dir = tempDir
	out, err = cmd.Output()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var modInfo struct {
		Versions []string
	}
	err = json.Unmarshal(out, &modInfo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Load package.
	bpkg, err := build.Default.ImportDir(pkgInfo.Dir, build.ImportComment)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	//var fs http.FileSystem = http.Dir(bpkg.Dir)

	frontendState := page.State{
		ImportPath: importPath,
	}

	type version struct {
		Name string
		Time time.Time
	}
	data := struct {
		FrontendState      page.State // TODO: Maybe move RawQuery, etc., here?
		AnalyticsHTML      template.HTML
		RawQuery           string
		Tabs               template.HTML
		ImportPath         string
		ImportPathElements template.HTML // Import path with linkified elements.
		Version            version
		DirExists          bool
		Bpkg               *build.Package
		Folders            []string
		Files              template.HTML
		Branches           template.HTML // Select menu for branches.
		Tests              template.HTML // Checkbox for tests.
	}{
		FrontendState: frontendState,
		AnalyticsHTML: h.analyticsHTML,
		RawQuery:      req.URL.RawQuery,
		//Tabs:               page.Tabs(req.URL.Path, req.URL.RawQuery),
		ImportPath:         importPath,
		ImportPathElements: page.ImportPathElementsHTML(pkgInfo.Module.Path, importPath, req.URL.RawQuery),
		Version: version{
			Name: pkgInfo.Module.Version,
			Time: pkgInfo.Module.Time,
		},
		DirExists: true, //fs != nil,
		Bpkg:      bpkg,
		//Tests:     checkbox.New(false, req.URL.Query(), testsQueryParameter),
	}

	// List subdirectories.
	{
		fis, err := ioutil.ReadDir(bpkg.Dir)
		if err != nil {
			log.Println("ReadDir(bpkg.Dir):", err)
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
	if branches := modInfo.Versions; len(branches) != 0 {
		defaultBranch := pkgInfo.Module.Version // HACK, TODO: Compute better, if possible.
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
			file, err := os.Open(path.Join(bpkg.Dir, goFile))
			if err != nil {
				panic(fmt.Errorf("Open(%q): %v", path.Join(bpkg.Dir, goFile), err))
			}
			fi, err := file.Stat()
			if err != nil {
				panic(fmt.Errorf("Stat(%q): %v", path.Join(bpkg.Dir, goFile), err))
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
								url := importPathURL(pathValue, req.URL.RawQuery)
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
			fmt.Fprintf(&buf, `<div><h2 id="%s">%s<a class="anchor" onclick="MustScrollTo(event, &#34;\&#34;%s\&#34;&#34;);"><span class="anchor-icon">%s</span></a></h2>`, sanitizedanchorname.Create(goFile), html.EscapeString(goFile), sanitizedanchorname.Create(goFile), linkOcticon) // HACK.
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
	if httpguts.HeaderValuesContainsToken(req.Header["Accept-Encoding"], "gzip") {
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
}

// importPathURL returns a URL to the target importPath, preserving query parameters.
//
// It strips out the revision parameter.
func importPathURL(importPath string, rawQuery string) template.URL {
	query, _ := url.ParseQuery(rawQuery)
	// Do not persist the revision.
	// TODO: Read go.mod, use appropriate revision, etc.
	query.Del(gtdo.RevisionQueryParameter)
	url := url.URL{
		Path:     "/" + importPath,
		RawQuery: query.Encode(),
	}
	return template.URL(url.String())
}

// fullQuery returns rawQuery with a "?" prefix if rawQuery is non-empty.
func fullQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	return "?" + rawQuery
}

var linkOcticon = func() string {
	var buf bytes.Buffer
	err := html.Render(&buf, octicon.Link())
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

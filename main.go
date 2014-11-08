package main

import (
	"bytes"
	"flag"
	"fmt"
	"html"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"go/ast"
	"go/build"
	"go/parser"
	"go/token"

	"code.google.com/p/go.tools/godoc/vfs"
	"github.com/shurcooL/go-goon"
	"github.com/shurcooL/go/gists/gist5639599"
	"github.com/shurcooL/go/github_flavored_markdown/sanitized_anchor_name"
	"github.com/shurcooL/go/gopherjs_http"
	vcs2 "github.com/shurcooL/go/vcs"
	"github.com/shurcooL/go/vfs_util"
	"github.com/sourcegraph/annotate"
	"github.com/sourcegraph/apiproxy"
	"github.com/sourcegraph/apiproxy/service/github"
	"github.com/sourcegraph/go-vcs/vcs"
	"github.com/sourcegraph/httpcache"
	"github.com/sourcegraph/syntaxhighlight"
	"github.com/sourcegraph/vcsstore/vcsclient"
)

var httpFlag = flag.String("http", ":8080", "Listen for HTTP connections on this address.")

var sg *vcsclient.Client

func main() {
	flag.Parse()

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

	sg = vcsclient.New(&url.URL{Scheme: "http", Host: "localhost:26203"}, cacheClient)
	sg.UserAgent = "gotools.org backend " + sg.UserAgent

	http.HandleFunc("/", codeHandler)

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
	http.Handle("/command-r.go.js", gopherjs_http.StaticGoFiles("../frontend/select-list-view/main.go"))
	http.HandleFunc("/command-r.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../frontend/select-list-view/style.css")
	})
	http.Handle("/table-of-contents.go.js", gopherjs_http.StaticGoFiles("../frontend/table-of-contents/main.go"))
	http.HandleFunc("/table-of-contents.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../frontend/table-of-contents/style.css")
	})

	panic(http.ListenAndServe(*httpFlag, nil))
}

// TODO: Dedup.
var gfmHtmlConfig = syntaxhighlight.HTMLConfig{
	String:        "s",
	Keyword:       "k",
	Comment:       "c",
	Type:          "n",
	Literal:       "lit",
	Punctuation:   "p",
	Plaintext:     "n",
	Tag:           "tag",
	HTMLTag:       "htm",
	HTMLAttrName:  "atn",
	HTMLAttrValue: "atv",
	Decimal:       "m",
}

func codeHandler(w http.ResponseWriter, req *http.Request) {
	importPath := req.URL.Path[1:]
	rev := req.URL.Query().Get("rev")
	_, _ = importPath, rev

	log.Printf("req: importPath=%q rev=%q.\n", importPath, rev)

	if importPath == "" {
		http.ServeFile(w, req, "./index.html")
		return
	}

	bpkg, fs, err := try(req)
	if err != nil {
		panic(err)
	}

	fmt.Fprintf(w, `<html>
	<head>
		<title>%s - Go Code</title>`, html.EscapeString(importPath))
	io.WriteString(w, `
		<link href="https://dl.dropboxusercontent.com/u/8554242/temp/github-flavored-markdown.css" media="all" rel="stylesheet" type="text/css" />
		<link href="/command-r.css" media="all" rel="stylesheet" type="text/css" />
		<link href="/table-of-contents.css" media="all" rel="stylesheet" type="text/css" />
		<style>
			.highlight h3 {
				display: inline;
				font-size: inherit;
				margin-top: 0;
				margin-bottom: 0;
				font-weight: normal;
			}
			.highlight h3.highlighted {
				background: rgb(243, 136, 73);
			}
			.highlight h3.highlighted-fade {
				background: rgba(243, 136, 73, 0.0);
				transition: background 0.5s ease-in-out;
			}
		</style>
<script>
  (function(i,s,o,g,r,a,m){i['GoogleAnalyticsObject']=r;i[r]=i[r]||function(){
  (i[r].q=i[r].q||[]).push(arguments)},i[r].l=1*new Date();a=s.createElement(o),
  m=s.getElementsByTagName(o)[0];a.async=1;a.src=g;m.parentNode.insertBefore(a,m)
  })(window,document,'script','//www.google-analytics.com/analytics.js','ga');

  ga('create', 'UA-56541369-1', 'auto');
  ga('send', 'pageview');

</script>
	</head>
	<body>
		<div style="position: relative; min-height: 100%;">
			<div style="width: 100%; background-color: hsl(209, 51%, 92%);">
				<span style="margin-left: 30px; padding: 15px; display: inline-block;"><strong>Go Tools</strong></span>
				<span style="background-color: hsl(209, 51%, 88%); padding: 15px; display: inline-block;">Code</span>
				<span style="padding: 15px; display: inline-block;"><strong>Cmd+R</strong>: Go To Symbol...</span>
			</div>
			<div style="padding-bottom: 50px;">
				<article class="markdown-body entry-content" style="padding: 30px;">`)

	fmt.Fprintf(w, "<h1>%s</h1>", html.EscapeString(importPath))

	for _, goFile := range append(bpkg.GoFiles, bpkg.CgoFiles...) {
		fset := token.NewFileSet()
		file, err := fs.Open(path.Join(bpkg.Dir, goFile))
		if err != nil {
			panic(err)
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

		anns, err := syntaxhighlight.Annotate(src, syntaxhighlight.HTMLAnnotator(gfmHtmlConfig))

		for _, decl := range fileAst.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				pos := fset.File(d.Pos()).Offset(d.Pos())
				funcDeclSignature := &ast.FuncDecl{Recv: d.Recv, Name: d.Name, Type: d.Type}
				name := d.Name.String()
				if d.Recv != nil {
					name = strings.TrimPrefix(gist5639599.SprintAstBare(d.Recv.List[0].Type), "*") + "." + name
				}
				//fmt.Fprintln(w, pos, d.Name.String(), gist5639599.SprintAstBare(funcDeclSignature))
				ann := &annotate.Annotation{
					Start: pos,
					End:   pos + len(gist5639599.SprintAstBare(funcDeclSignature)),

					Left:  []byte(fmt.Sprintf(`<h3 id="%s">`, name)),
					Right: []byte(`</h3>`),
				}
				anns = append(anns, ann)
			case *ast.GenDecl:
				if d.Tok != token.IMPORT {
					continue
				}
				for _, imp := range d.Specs {
					path := imp.(*ast.ImportSpec).Path
					pos := fset.File(path.Pos()).Offset(path.Pos())
					end := fset.File(path.End()).Offset(path.End())
					pathValue, err := strconv.Unquote(path.Value)
					if err != nil {
						continue
					}
					ann := &annotate.Annotation{
						Start: pos + 1, // Don't include quote characters.
						End:   end - 1,

						Left:  []byte(fmt.Sprintf(`<a href="%s" target="_blank">`, "/"+pathValue)),
						Right: []byte(`</a>`),
					}
					anns = append(anns, ann)
				}
			}
		}

		/*var buf bytes.Buffer
		err = (&printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}).Fprint(&buf, fset, fileAst)
		if err != nil {
			panic(err)
		}*/

		sort.Sort(anns)

		b, err := annotate.Annotate(src, anns, func(w io.Writer, b []byte) { template.HTMLEscape(w, b) })
		if err != nil {
			panic(err)
		}

		fmt.Fprintf(w, "<h2 id=\"%s\">%s</h2>", sanitized_anchor_name.Create(goFile), html.EscapeString(goFile))
		io.WriteString(w, `<div class="highlight highlight-Go"><pre>`)
		w.Write(b)
		io.WriteString(w, `</pre></div>`)
	}

	io.WriteString(w, `</article>
			</div>
			<div style="position: absolute; bottom: 0; left: 0; width: 100%; text-align: right; background-color: hsl(209, 51%, 92%);">
				<span style="margin-right: 15px; padding: 15px; display: inline-block;"><a href="https://github.com/shurcooL/gtdo/issues" target="_blank">Report an issue</a></span>
			</div>
		</div>
		<script type="text/javascript" src="/command-r.go.js"></script>
		<script type="text/javascript" src="/table-of-contents.go.js"></script>
	</body>
</html>
`)
}

func try(req *http.Request) (*build.Package, vfs.FileSystem, error) {
	importPath := req.URL.Path[1:]

	fs := vfs.OS("")

	context := buildContextUsingFS(fs)
	bpkg, err0 := context.Import(importPath, "", 0)
	if err0 == nil {
		return bpkg, fs, nil
	}

	repo, repoImportPath, commitId, err := repoFromRequest(req)
	if err != nil {
		return nil, nil, err
	}

	fs, err = repo.FileSystem(commitId)
	if err != nil {
		return nil, nil, err
	}

	fs = vfs_util.NewPrefixFS(fs, "/virtual-go-workspace/src/"+repoImportPath)

	context = buildContextUsingFS(fs)
	context.GOPATH = "/virtual-go-workspace"
	bpkg, err1 := context.Import(importPath, "", 0)
	if err1 == nil {
		return bpkg, fs, nil
	}

	return nil, nil, MultiError{err0, err1}
}

func importPathToRepoGuess(importPath string) (repoImportPath string, cloneUrl *url.URL, vcsRepo vcs2.Vcs, err error) {
	switch {
	case strings.HasPrefix(importPath, "github.com/"):
		importPathElements := strings.Split(importPath, "/")
		if len(importPathElements) < 3 {
			return "", nil, nil, err
		}

		repoImportPath = path.Join(importPathElements[:3]...)

		cloneUrl, err = url.Parse("https://" + repoImportPath)
		if err != nil {
			return "", nil, nil, err
		}

		return repoImportPath, cloneUrl, vcs2.NewFromType(vcs2.Git), nil
	case strings.HasPrefix(importPath, "code.google.com/p/"):
		importPathElements := strings.Split(importPath, "/")
		if len(importPathElements) < 3 {
			return "", nil, nil, err
		}

		repoImportPath = path.Join(importPathElements[:3]...)

		cloneUrl, err = url.Parse("https://" + repoImportPath)
		if err != nil {
			return "", nil, nil, err
		}

		return repoImportPath, cloneUrl, vcs2.NewFromType(vcs2.Hg), nil
	default:
		return "", nil, nil, err
	}
}

func repoFromRequest(req *http.Request) (repo vcs.Repository, repoImportPath string, commitId vcs.CommitID, err error) {
	importPath := req.URL.Path[1:]
	rev := req.URL.Query().Get("rev")

	repoImportPath, cloneUrl, vcsRepo, err := importPathToRepoGuess(importPath)
	if err != nil {
		return nil, "", "", err
	}

	goon.DumpExpr(cloneUrl, vcsRepo, err)

TryGitInstead:
	repo, err = sg.Repository(vcsRepo.Type().VcsType(), cloneUrl)
	if err != nil {
		return nil, "", "", err
	}

	if rev != "" {
		commitId, err = repo.ResolveRevision(rev)
	} else {
		commitId, err = repo.ResolveBranch(vcsRepo.GetDefaultBranch())
	}
	if err != nil && vcsRepo.Type() == vcs2.Hg && strings.HasPrefix(importPath, "code.google.com/p/") {
		vcsRepo = vcs2.NewFromType(vcs2.Git)
		goto TryGitInstead
	}
	if err != nil {
		err1 := repo.(vcsclient.RepositoryCloneUpdater).CloneOrUpdate(vcs.RemoteOpts{})
		fmt.Println("repoFromRequest: CloneOrUpdate:", err1)
		if err1 != nil {
			return nil, "", "", MultiError{err, err1}
		}

		if rev != "" {
			commitId, err1 = repo.ResolveRevision(rev)
		} else {
			commitId, err1 = repo.ResolveBranch(vcsRepo.GetDefaultBranch())
		}
		if err1 != nil {
			return nil, "", "", MultiError{err, err1}
		}
		fmt.Println("repoFromRequest: worked on SECOND try")
	} else {
		fmt.Println("repoFromRequest: worked on first try")
	}

	return repo, repoImportPath, commitId, nil
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
		return "", false
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

type MultiError []error

func (me MultiError) Error() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%d errors:\n", len(me))
	for _, err := range me {
		fmt.Fprintln(&buf, err.Error())
	}
	return buf.String()
}

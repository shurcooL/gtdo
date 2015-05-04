package main

/*
#cgo pkg-config: --static --define-variable=libdir=../../Godeps/_workspace/src/github.com/libgit2/git2go/vendor/libgit2/build --define-variable=includedir=../../Godeps/_workspace/src/github.com/libgit2/git2go/vendor/libgit2/include ../../Godeps/_workspace/src/github.com/libgit2/git2go/vendor/libgit2/build/libgit2.pc
// #cgo LDFLAGS: -lgit2
#cgo LDFLAGS: -L../../Godeps/_workspace/src/github.com/libgit2/git2go/vendor/libgit2/build -lgit2
*/
import "C"

import (
	"crypto/subtle"
	"encoding/base64"
	_ "expvar"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/coreos/go-etcd/etcd"
	"github.com/gorilla/handlers"
	"github.com/lox/httpcache"
	"sourcegraph.com/sourcegraph/datad"
	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	_ "sourcegraph.com/sourcegraph/go-vcs/vcs/git"
	_ "sourcegraph.com/sourcegraph/go-vcs/vcs/hg"
	"sourcegraph.com/sourcegraph/vcsstore"
	"sourcegraph.com/sourcegraph/vcsstore/cluster"
	"sourcegraph.com/sourcegraph/vcsstore/server"
	"sourcegraph.com/sourcegraph/vcsstore/vcsclient"
)

var (
	storageDir = flag.String("s", "/tmp/vcsstore", "storage root dir for VCS repos")
	verbose    = flag.Bool("v", true, "show verbose output")

	etcdEndpoint  = flag.String("etcd", "http://127.0.0.1:4001", "etcd endpoint")
	etcdKeyPrefix = flag.String("etcd-key-prefix", filepath.Join(datad.DefaultKeyPrefix, "vcs"), "keyspace for datad registry and provider list in etcd")

	defaultPort = "9090"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, `vcsstore caches and serves information about VCS repositories.

Usage:

        vcsstore [options] command [arg...]

The commands are:
`)
		for _, c := range subcommands {
			fmt.Fprintf(os.Stderr, "    %-14s %s\n", c.Name, c.Description)
		}
		fmt.Fprintln(os.Stderr, `
Use "vcsstore command -h" for more information about a command.

The global options are:
`)
		flag.PrintDefaults()
		os.Exit(1)
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
	}

	subcmd := flag.Arg(0)
	extraArgs := flag.Args()[1:]
	for _, c := range subcommands {
		if c.Name == subcmd {
			c.Run(extraArgs)
			return
		}

	}

	fmt.Fprintf(os.Stderr, "vcsstore: unknown subcommand %q\n", subcmd)
	fmt.Fprintln(os.Stderr, `Run "vcsstore -h" for usage.`)
	os.Exit(1)
}

type subcommand struct {
	Name        string
	Description string
	Run         func(args []string)
}

var subcommands = []subcommand{
	{"serve", "start an HTTP server to serve VCS repository data", serveCmd},
	{"repo", "display information about a repository", repoCmd},
	{"clone", "clones a repository on the server", cloneCmd},
	{"get", "gets a path from the server (or datad cluster)", getCmd},
}

func etcdBackend() datad.Backend {
	return datad.NewEtcdBackend(*etcdKeyPrefix, etcd.NewClient([]string{*etcdEndpoint}))
}

func serveCmd(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	debug := fs.Bool("d", false, "debug mode (don't use on publicly available servers)")
	bindAddr := fs.String("http", ":"+defaultPort, "HTTP listen address")
	datadNode := fs.Bool("datad", false, "participate as a node in a datad cluster")
	datadNodeName := fs.String("datad-node-name", "127.0.0.1:"+defaultPort, "datad node name (must be accessible to datad clients & other nodes)")
	tlsCert := fs.String("tls.cert", "", "TLS certificate file (if set, server uses TLS)")
	tlsKey := fs.String("tls.key", "", "TLS key file (if set, server uses TLS)")
	basicAuth := fs.String("http.basicauth", "", "if set to 'user:passwd', require HTTP Basic Auth")
	cache := fs.String("cache", "", "HTTP cache (either 'mem' or 'disk:/path/to/cache/dir')")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: vcsstore serve [options]

Starts an HTTP server that serves information about VCS repositories.

The options are:
`)
		fs.PrintDefaults()
		os.Exit(1)
	}
	fs.Parse(args)

	if fs.NArg() != 0 {
		fs.Usage()
	}

	err := os.MkdirAll(*storageDir, 0700)
	if err != nil {
		log.Fatalf("Error creating directory %q: %s.", *storageDir, err)
	}

	var logw io.Writer
	if *verbose {
		logw = os.Stderr
	} else {
		logw = ioutil.Discard
	}

	conf := &vcsstore.Config{
		StorageDir: *storageDir,
		Log:        log.New(logw, "vcsstore: ", log.LstdFlags),
	}
	if *debug {
		conf.DebugLog = log.New(logw, "vcsstore DEBUG: ", log.LstdFlags)
	}

	vh := server.NewHandler(vcsstore.NewService(conf), nil, nil)
	vh.Log = log.New(logw, "server: ", log.LstdFlags)
	vh.Debug = *debug

	if *datadNode {
		node := datad.NewNode(*datadNodeName, etcdBackend(), cluster.NewProvider(conf, vh.Service))
		node.Updaters = runtime.GOMAXPROCS(0)
		err := node.Start()
		if err != nil {
			log.Fatal("Failed to start datad node: ", err)
		}
		log.Printf("Started datad node %s.", *datadNodeName)
	}

	var h http.Handler
	if *basicAuth != "" {
		parts := strings.SplitN(*basicAuth, ":", 2)
		if len(parts) != 2 {
			log.Fatalf("Basic auth must be specified as 'user:passwd'.")
		}
		user, passwd := parts[0], parts[1]
		if user == "" || passwd == "" {
			log.Fatalf("Basic auth user and passwd must both be nonempty.")
		}
		log.Printf("Requiring HTTP Basic auth")
		h = newBasicAuthHandler(user, passwd, vh)
	} else {
		h = vh
	}
	h = cacheHandler(*cache, h)
	http.Handle("/", handlers.CombinedLoggingHandler(os.Stderr, h))

	if *tlsCert != "" || *tlsKey != "" {
		fmt.Fprintf(os.Stderr, "Starting HTTPS server on %s (cert %s, key %s)\n", *bindAddr, *tlsCert, *tlsKey)
		log.Fatal(http.ListenAndServeTLS(*bindAddr, *tlsCert, *tlsKey, nil))
	} else {
		fmt.Fprintf(os.Stderr, "Starting HTTP server on %s\n", *bindAddr)
		log.Fatal(http.ListenAndServe(*bindAddr, nil))
	}
}

func cacheHandler(cacheOpt string, h http.Handler) http.Handler {
	if cacheOpt == "" {
		return h
	}
	var cache *httpcache.Cache
	if cacheOpt == "mem" {
		cache = httpcache.NewMemoryCache()
		log.Printf("Using in-memory HTTP cache.")
	} else if strings.HasPrefix(cacheOpt, "disk:") {
		dir := cacheOpt[len("disk:"):]
		log.Printf("Using on-disk HTTP cache at %q.", dir)
		var err error
		cache, err = httpcache.NewDiskCache(dir)
		if err != nil {
			log.Fatalf("Error creating HTTP disk cache at dir %q: %s.", dir, err)
		}
	} else {
		log.Fatalf("Invalid -cache option: %q.", cacheOpt)
	}
	ch := httpcache.NewHandler(cache, h)
	httpcache.DebugLogging, _ = strconv.ParseBool(os.Getenv("LOG_CACHE"))
	return ch
}

func newBasicAuthHandler(user, passwd string, h http.Handler) http.Handler {
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", user, passwd)))
	return &basicAuthHandler{h, []byte(want)}
}

type basicAuthHandler struct {
	http.Handler
	want []byte // = "Basic " base64(user ":" passwd) [precomputed]
}

func (h *basicAuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Constant time comparison to avoid timing attack.
	authHdr := r.Header.Get("authorization")
	if len(h.want) == len(authHdr) && subtle.ConstantTimeCompare(h.want, []byte(authHdr)) == 1 {
		h.Handler.ServeHTTP(w, r)
		return
	}
	w.Header().Set("WWW-Authenticate", `Basic realm="vcsstore"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func repoCmd(args []string) {
	fs := flag.NewFlagSet("repo", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: vcsstore repo [options] vcs-type clone-url

Displays the directory to which a repository would be cloned.

The options are:
`)
		fs.PrintDefaults()
		os.Exit(1)
	}
	fs.Parse(args)

	if fs.NArg() != 2 {
		fs.Usage()
	}

	vcsType := fs.Arg(0)
	cloneURL, err := url.Parse(fs.Arg(1))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("RepositoryPath:      ", filepath.Join(*storageDir, vcsstore.EncodeRepositoryPath(vcsType, cloneURL)))
	fmt.Println("URL:                 ", vcsclient.NewRouter(nil).URLToRepo(vcsType, cloneURL))
}

func cloneCmd(args []string) {
	fs := flag.NewFlagSet("clone", flag.ExitOnError)
	urlStr := fs.String("url", "http://localhost:"+defaultPort, "base URL to a running vcsstore API server")
	datadClient := fs.Bool("datad", false, "use datad cluster client")
	sshKeyFile := fs.String("i", "", "ssh private key file for clone remote")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: vcsstore clone [options] vcs-type clone-url

Clones a repository on the server. Once finished, the repository will be
available to the client via the vcsstore API.

The options are:
`)
		fs.PrintDefaults()
		os.Exit(1)
	}
	fs.Parse(args)

	if fs.NArg() != 2 {
		fs.Usage()
	}

	baseURL, err := url.Parse(*urlStr)
	if err != nil {
		log.Fatal(err)
	}

	vcsType := fs.Arg(0)
	cloneURL, err := url.Parse(fs.Arg(1))
	if err != nil {
		log.Fatal(err)
	}

	var repo vcs.Repository
	if *datadClient {
		cc := cluster.NewClient(datad.NewClient(etcdBackend()), nil)
		repo, err = cc.Repository(vcsType, cloneURL)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		c := vcsclient.New(baseURL, nil)
		repo, err = c.Repository(vcsType, cloneURL)
		if err != nil {
			log.Fatal("Open repository: ", err)
		}
	}

	var opt vcs.RemoteOpts
	if *sshKeyFile != "" {
		key, err := ioutil.ReadFile(*sshKeyFile)
		if err != nil {
			log.Fatal(err)
		}
		opt.SSH = &vcs.SSHConfig{PrivateKey: key}
	}

	if repo, ok := repo.(vcsclient.RepositoryCloneUpdater); ok {
		err := repo.CloneOrUpdate(opt)
		if err != nil {
			log.Fatal("Clone: ", err)
		}
	} else {
		log.Fatalf("Remote cloning is not implemented for %T.", repo)
	}

	fmt.Printf("%-5s %-45s cloned OK\n", vcsType, cloneURL)
}

func getCmd(args []string) {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	urlStr := fs.String("url", "http://localhost:"+defaultPort, "base URL to a running vcsstore API server")
	datadClient := fs.Bool("datad", false, "route request using datad (specify etcd backend in global options)")
	method := fs.String("method", "GET", "HTTP request method")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: vcsstore get [options] vcs-type clone-url [extra-path]

Gets a URL path from the server (optionally routing the request using datad).

The options are:
`)
		fs.PrintDefaults()
		os.Exit(1)
	}
	fs.Parse(args)

	if n := fs.NArg(); n != 2 && n != 3 {
		fs.Usage()
	}
	vcsType, cloneURLStr := fs.Arg(0), fs.Arg(1)
	var extraPath string
	if fs.NArg() == 3 {
		extraPath = fs.Arg(2)
	}

	baseURL, err := url.Parse(*urlStr)
	if err != nil {
		log.Fatal(err)
	}

	cloneURL, err := url.Parse(cloneURLStr)
	if err != nil {
		log.Fatal(err)
	}

	router := vcsclient.NewRouter(nil)
	url := router.URLToRepo(vcsType, cloneURL)
	url.Path = strings.TrimPrefix(url.Path, "/")
	url = baseURL.ResolveReference(url)
	url.Path = filepath.Join(url.Path, extraPath)

	if *datadClient {
		datadGet(*method, vcsType, cloneURL, url)
	} else {
		normalGet(*method, nil, url)
	}
}

func datadGet(method string, vcsType string, cloneURL *url.URL, reqURL *url.URL) {
	cc := cluster.NewClient(datad.NewClient(etcdBackend()), nil)
	t, err := cc.TransportForRepository(vcsType, cloneURL)
	if err != nil {
		log.Fatal(err)
	}

	reqURL.Host = "$(DATAD_NODE)"
	normalGet(method, &http.Client{Transport: t}, reqURL)
}

func normalGet(method string, c *http.Client, url *url.URL) {
	if c == nil {
		c = http.DefaultClient
	}

	if *verbose {
		log.Printf("%s %s", method, url)
	}

	req, err := http.NewRequest(method, url.String(), nil)
	if err != nil {
		log.Fatal(err)
	}
	resp, err := c.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	if !(resp.StatusCode >= 200 && resp.StatusCode <= 399) {
		log.Fatalf("Error: HTTP %d: %s.", resp.StatusCode, body)
	}

	fmt.Println(string(body))
}

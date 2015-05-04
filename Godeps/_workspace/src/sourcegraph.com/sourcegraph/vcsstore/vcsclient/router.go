package vcsclient

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/google/go-querystring/query"
	muxpkg "github.com/sourcegraph/mux"
	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	"sourcegraph.com/sourcegraph/vcsstore"
)

const (
	// Route names
	RouteRepo                   = "vcs:repo"
	RouteRepoBlameFile          = "vcs:repo.blame-file"
	RouteRepoBranch             = "vcs:repo.branch"
	RouteRepoBranches           = "vcs:repo.branches"
	RouteRepoCommit             = "vcs:repo.commit"
	RouteRepoCommits            = "vcs:repo.commits"
	RouteRepoCreateOrUpdate     = "vcs:repo.create-or-update"
	RouteRepoDiff               = "vcs:repo.diff"
	RouteRepoCrossRepoDiff      = "vcs:repo.cross-repo-diff"
	RouteRepoMergeBase          = "vcs:repo.merge-base"
	RouteRepoCrossRepoMergeBase = "vcs:repo.cross-repo-merge-base"
	RouteRepoRevision           = "vcs:repo.rev"
	RouteRepoSearch             = "vcs:repo.search"
	RouteRepoTag                = "vcs:repo.tag"
	RouteRepoTags               = "vcs:repo.tags"
	RouteRepoTreeEntry          = "vcs:repo.tree-entry"
	RouteRoot                   = "vcs:root"
)

type Router muxpkg.Router

// NewRouter creates a new router that matches and generates URLs that the HTTP
// handler recognizes.
func NewRouter(parent *muxpkg.Router) *Router {
	if parent == nil {
		parent = muxpkg.NewRouter()
	}

	parent.Path("/").Methods("GET").Name(RouteRoot)

	// Encode the repository VCS type and clone URL using
	// vcsstore.{Encode,Decode}RepositoryPath.
	//
	// Because these are used for both EncodedRepo and
	// EncodedHeadRepo, they require a string label ("" and "Head",
	// respectively).
	const encodedRepoPattern = "(?:[^/]+)(?:/[^./][^/]*){2,}"
	unescapeRepoVars := func(label string) func(req *http.Request, match *muxpkg.RouteMatch, r *muxpkg.Route) {
		return func(req *http.Request, match *muxpkg.RouteMatch, r *muxpkg.Route) {
			vcsType, cloneURL, err := vcsstore.DecodeRepositoryPath(match.Vars["Encoded"+label+"Repo"])
			if err != nil {
				return
			}
			match.Vars[label+"VCS"] = vcsType
			match.Vars[label+"CloneURL"] = cloneURL.String()
			delete(match.Vars, "Encoded"+label+"Repo")
		}
	}
	escapeRepoVars := func(label string) func(vars map[string]string) map[string]string {
		return func(vars map[string]string) map[string]string {
			cloneURL, err := url.Parse(vars[label+"CloneURL"])
			if err != nil {
				return vars
			}
			vars["Encoded"+label+"Repo"] = vcsstore.EncodeRepositoryPath(vars[label+"VCS"], cloneURL)
			delete(vars, label+"CloneURL")
			delete(vars, label+"VCS")
			return vars
		}
	}

	repoPath := "/{EncodedRepo:" + encodedRepoPattern + "}"
	parent.Path(repoPath).Methods("GET").PostMatchFunc(unescapeRepoVars("")).BuildVarsFunc(escapeRepoVars("")).Name(RouteRepo)
	parent.Path(repoPath).Methods("POST").PostMatchFunc(unescapeRepoVars("")).BuildVarsFunc(escapeRepoVars("")).Name(RouteRepoCreateOrUpdate)
	repo := parent.PathPrefix(repoPath).PostMatchFunc(unescapeRepoVars("")).BuildVarsFunc(escapeRepoVars("")).Subrouter()
	repo.Path("/.blame/{Path:.+}").Methods("GET").Name(RouteRepoBlameFile)
	repo.Path("/.diff/{Base}..{Head}").Methods("GET").Name(RouteRepoDiff)
	repo.Path("/.cross-repo-diff/{Base}..{EncodedHeadRepo:" + encodedRepoPattern + "}:{Head}").Methods("GET").PostMatchFunc(unescapeRepoVars("Head")).BuildVarsFunc(escapeRepoVars("Head")).Name(RouteRepoCrossRepoDiff)
	repo.Path("/.branches").Methods("GET").Name(RouteRepoBranches)
	repo.Path("/.branches/{Branch:.+}").Methods("GET").Name(RouteRepoBranch)
	repo.Path("/.revs/{RevSpec:.+}").Methods("GET").Name(RouteRepoRevision)
	repo.Path("/.tags").Methods("GET").Name(RouteRepoTags)
	repo.Path("/.tags/{Tag:.+}").Methods("GET").Name(RouteRepoTag)
	repo.Path("/.merge-base/{CommitIDA}/{CommitIDB}").Methods("GET").Name(RouteRepoMergeBase)
	repo.Path("/.cross-repo-merge-base/{CommitIDA}/{EncodedBRepo:" + encodedRepoPattern + "}/{CommitIDB}").Methods("GET").PostMatchFunc(unescapeRepoVars("B")).BuildVarsFunc(escapeRepoVars("B")).Name(RouteRepoCrossRepoMergeBase)
	repo.Path("/.commits").Methods("GET").Name(RouteRepoCommits)
	commitPath := "/.commits/{CommitID}"
	repo.Path(commitPath).Methods("GET").Name(RouteRepoCommit)
	commit := repo.PathPrefix(commitPath).Subrouter()

	// cleanTreeVars modifies the Path route var to be a clean filepath. If it
	// is empty, it is changed to ".".
	cleanTreeVars := func(req *http.Request, match *muxpkg.RouteMatch, r *muxpkg.Route) {
		path := filepath.Clean(strings.TrimPrefix(match.Vars["Path"], "/"))
		if path == "" || path == "." {
			match.Vars["Path"] = "."
		} else {
			match.Vars["Path"] = path
		}
	}
	// prepareTreeVars prepares the Path route var to generate a clean URL.
	prepareTreeVars := func(vars map[string]string) map[string]string {
		if path := vars["Path"]; path == "." {
			vars["Path"] = ""
		} else {
			vars["Path"] = "/" + filepath.Clean(path)
		}
		return vars
	}
	commit.Path("/tree{Path:(?:/.*)*}").Methods("GET").PostMatchFunc(cleanTreeVars).BuildVarsFunc(prepareTreeVars).Name(RouteRepoTreeEntry)
	commit.Path("/search").Methods("GET").Name(RouteRepoSearch)

	return (*Router)(parent)
}

func (r *Router) URLToRepo(vcsType string, cloneURL *url.URL) *url.URL {
	return r.URLTo(RouteRepo, "VCS", vcsType, "CloneURL", cloneURL.String())
}

func (r *Router) URLToRepoBlameFile(vcsType string, cloneURL *url.URL, path string, opt *vcs.BlameOptions) *url.URL {
	u := r.URLTo(RouteRepoBlameFile, "VCS", vcsType, "CloneURL", cloneURL.String(), "Path", path)
	if opt != nil {
		q, err := query.Values(opt)
		if err != nil {
			panic(err.Error())
		}
		u.RawQuery = q.Encode()
	}
	return u
}

func (r *Router) URLToRepoDiff(vcsType string, cloneURL *url.URL, base, head vcs.CommitID, opt *vcs.DiffOptions) *url.URL {
	u := r.URLTo(RouteRepoDiff, "VCS", vcsType, "CloneURL", cloneURL.String(), "Base", string(base), "Head", string(head))
	if opt != nil {
		q, err := query.Values(opt)
		if err != nil {
			panic(err.Error())
		}
		u.RawQuery = q.Encode()
	}
	return u
}

func (r *Router) URLToRepoCrossRepoDiff(baseVCS string, baseCloneURL *url.URL, base vcs.CommitID, headVCS string, headCloneURL *url.URL, head vcs.CommitID, opt *vcs.DiffOptions) *url.URL {
	u := r.URLTo(RouteRepoCrossRepoDiff, "VCS", baseVCS, "CloneURL", baseCloneURL.String(), "Base", string(base), "HeadVCS", headVCS, "HeadCloneURL", headCloneURL.String(), "Head", string(head))
	if opt != nil {
		q, err := query.Values(opt)
		if err != nil {
			panic(err.Error())
		}
		u.RawQuery = q.Encode()
	}
	return u
}

func (r *Router) URLToRepoBranch(vcsType string, cloneURL *url.URL, branch string) *url.URL {
	return r.URLTo(RouteRepoBranch, "VCS", vcsType, "CloneURL", cloneURL.String(), "Branch", branch)
}

func (r *Router) URLToRepoBranches(vcsType string, cloneURL *url.URL) *url.URL {
	return r.URLTo(RouteRepoBranches, "VCS", vcsType, "CloneURL", cloneURL.String())
}

func (r *Router) URLToRepoRevision(vcsType string, cloneURL *url.URL, revSpec string) *url.URL {
	return r.URLTo(RouteRepoRevision, "VCS", vcsType, "CloneURL", cloneURL.String(), "RevSpec", revSpec)
}

func (r *Router) URLToRepoTag(vcsType string, cloneURL *url.URL, tag string) *url.URL {
	return r.URLTo(RouteRepoTag, "VCS", vcsType, "CloneURL", cloneURL.String(), "Tag", tag)
}

func (r *Router) URLToRepoTags(vcsType string, cloneURL *url.URL) *url.URL {
	return r.URLTo(RouteRepoTags, "VCS", vcsType, "CloneURL", cloneURL.String())
}

func (r *Router) URLToRepoCommit(vcsType string, cloneURL *url.URL, commitID vcs.CommitID) *url.URL {
	return r.URLTo(RouteRepoCommit, "VCS", vcsType, "CloneURL", cloneURL.String(), "CommitID", string(commitID))
}

func (r *Router) URLToRepoCommits(vcsType string, cloneURL *url.URL, opt vcs.CommitsOptions) *url.URL {
	u := r.URLTo(RouteRepoCommits, "VCS", vcsType, "CloneURL", cloneURL.String())
	q, err := query.Values(opt)
	if err != nil {
		panic(err.Error())
	}
	u.RawQuery = q.Encode()
	return u
}

func (r *Router) URLToRepoTreeEntry(vcsType string, cloneURL *url.URL, commitID vcs.CommitID, path string) *url.URL {
	return r.URLTo(RouteRepoTreeEntry, "VCS", vcsType, "CloneURL", cloneURL.String(), "CommitID", string(commitID), "Path", path)
}

func (r *Router) URLToRepoSearch(vcsType string, cloneURL *url.URL, at vcs.CommitID, opt vcs.SearchOptions) *url.URL {
	u := r.URLTo(RouteRepoSearch, "VCS", vcsType, "CloneURL", cloneURL.String(), "CommitID", string(at))
	q, err := query.Values(opt)
	if err != nil {
		panic(err.Error())
	}
	u.RawQuery = q.Encode()
	return u
}

func (r *Router) URLToRepoMergeBase(vcsType string, cloneURL *url.URL, a, b vcs.CommitID) *url.URL {
	return r.URLTo(RouteRepoMergeBase, "VCS", vcsType, "CloneURL", cloneURL.String(), "CommitIDA", string(a), "CommitIDB", string(b))
}

func (r *Router) URLToRepoCrossRepoMergeBase(vcsType string, cloneURL *url.URL, a vcs.CommitID, bVCS string, bCloneURL *url.URL, b vcs.CommitID) *url.URL {
	return r.URLTo(RouteRepoCrossRepoMergeBase, "VCS", vcsType, "CloneURL", cloneURL.String(), "CommitIDA", string(a), "BVCS", bVCS, "BCloneURL", bCloneURL.String(), "CommitIDB", string(b))
}

func (r *Router) URLTo(route string, vars ...string) *url.URL {
	url, err := (*muxpkg.Router)(r).Get(route).URL(vars...)
	if err != nil {
		panic(err.Error())
	}
	return url
}

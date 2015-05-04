package cluster

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"code.google.com/p/rog-go/parallel"

	"github.com/coreos/etcd/config"
	etcd_client "github.com/coreos/go-etcd/etcd"
	"sourcegraph.com/sourcegraph/datad"
	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	"sourcegraph.com/sourcegraph/vcsstore/vcsclient"
)

var (
	repoOwner      = flag.String("it.repo-owner", "", "(integration test) GitHub user whose repos should be listed and tested against")
	numNodes       = flag.Int("it.nodes", 5, "number of nodes (and corresponding vcsstore servers) to launch")
	etcdDebugLog   = flag.Bool("it.etcd-debug", false, "print etcd client debug log")
	waitBeforeExit = flag.Bool("it.wait", false, "don't exit after finished (keep nodes and etcd up)")
)

func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("long-running integration test")
	}

	repoInfos := getUserRepos(t, *repoOwner)

	type nodeInfo struct {
		cmd     *exec.Cmd
		baseURL string
	}
	nodes := map[string]*nodeInfo{}

	repos := map[repoInfo]vcs.Repository{}
	var reposMu sync.Mutex

	if *etcdDebugLog {
		etcd_client.SetLogger(log.New(os.Stderr, "etcd: ", 0))
	}

	withEtcd(t, func(etcdConfig *config.Config, ec *etcd_client.Client) {
		defer func() {
			if *waitBeforeExit {
				log.Printf("\n\nTest run ended. Ctrl-C to exit.")
				select {}
			}
		}()

		b := datad.NewEtcdBackend("/datad/vcs", ec)
		cc := NewClient(datad.NewClient(b), nil)

		if err := exec.Command("go", "install", "sourcegraph.com/sourcegraph/vcsstore/cmd/vcsstore").Run(); err != nil {
			t.Fatal(err)
		}

		killNode := func(name string, ni *nodeInfo) {
			if ni != nil && ni.cmd != nil {
				ni.cmd.Process.Kill()
				ni.cmd = nil
				delete(nodes, name)
			}
		}

		// Start the nodes and vcsstore servers.
		for i := 0; i < *numNodes; i++ {
			n := 6000 + i
			nodeName := fmt.Sprintf("127.0.0.1:%d", n)
			storageDir := fmt.Sprintf("/tmp/test-vcsstore%d", n)
			cmd := exec.Command("vcsstore", "-v", "-etcd="+etcdConfig.Addr, "-s="+storageDir, "serve", "-datad", "-d", fmt.Sprintf("-http=:%d", n), "-datad-node-name="+nodeName)
			nodes[nodeName] = &nodeInfo{cmd: cmd, baseURL: "http://" + nodeName}
			cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
			err := cmd.Start()
			if err != nil {
				t.Fatalf("error starting %v: %s", cmd.Args, err)
			}
			log.Printf("Launched node %s with storage dir %s (%v).", nodeName, storageDir, cmd.Args)
			defer func() {
				killNode(nodeName, nodes[nodeName])
			}()
		}

		// Wait for servers.
		time.Sleep(400 * time.Millisecond)

		// Clone the repositories.
		cloneStart := time.Now()
		var wg sync.WaitGroup
		for _, ri_ := range repoInfos {
			ri := ri_
			wg.Add(1)
			go func() {
				defer wg.Done()
				log.Printf("cloning %v...", ri)
				repo, err := cc.Repository(ri.vcsType, mustParseURL(ri.cloneURL))
				if err != nil {
					t.Errorf("clone %v failed: %s", ri, err)
					return
				}
				err = repo.(vcsclient.RepositoryCloneUpdater).CloneOrUpdate(vcs.RemoteOpts{})
				if err != nil {
					t.Errorf("remote clone %v failed: %s", ri, err)
					return
				}

				reposMu.Lock()
				defer reposMu.Unlock()
				repos[ri] = repo
			}()
		}
		wg.Wait()
		t.Logf("Cloned %d repositories in %s.", len(repoInfos), time.Since(cloneStart))

		performRepoOps := func() error {
			par := parallel.NewRun(1) // keep at 1, libgit2 has concurrency segfaults :(
			// Perform some operations on the repos.
			for ri_, repo_ := range repos {
				par.Do(func() error {
					ri, repo := ri_, repo_
					commitID, err := repo.ResolveBranch("master")
					if err != nil {
						return fmt.Errorf("repo %v: resolve branch 'master' failed: %s", ri, err)
					}
					commits, _, err := repo.Commits(vcs.CommitsOptions{Head: commitID})
					if err != nil {
						return fmt.Errorf("repo %v: commit log of 'master' failed: %s", ri, err)
					}
					if len(commits) == 0 {
						return fmt.Errorf("repo %v: commit log has 0 entries", ri)
					}
					fs, err := repo.FileSystem(commitID)
					if err != nil {
						return fmt.Errorf("repo %v: filesystem at 'master' failed: %s", ri, err)
					}
					entries, err := fs.ReadDir("/")
					if err != nil {
						return fmt.Errorf("repo %v: readdir '/' at 'master' failed: %s", ri, err)
					}
					_ = entries
					return nil
				})
			}
			return par.Wait()
		}

		opsStart := time.Now()
		err := performRepoOps()
		if err != nil {
			t.Fatalf("before killing any nodes, got error in repo ops: %s", err)
		}
		log.Printf("\n\n\nPerformed various operations on %d repositories in %s.\n\n\n", len(repos), time.Since(opsStart))

		if *numNodes <= 1 {
			t.Fatal("can't test cluster resilience with only 1 node")
		}

		// Kill half of all nodes to test resilience.
		for i := 0; i < *numNodes/2; i++ {
			for name, ni := range nodes {
				reg := datad.NewRegistry(b)
				regKeys, err := reg.KeysForNode(name)
				if err != nil {
					t.Fatal(err)
				}
				killNode(name, ni)
				log.Printf("\n\n\nKilled node %s. Before killing, it had registered keys: %v. Expect to see failures related to these keys in the next set of operations we perform.\n\n\n", name, regKeys)
				break
			}
		}
		time.Sleep(time.Millisecond * 300)
		killedTime := time.Now()

		// After killing nodes, run the same set of operations. We expect some
		// KeyTransportErrors here because the cluster detects that some nodes
		// are down but hasn't had time yet to fetch the data to other nodes.
		//
		// Keep running the operations until we get no more errors.

		log.Printf("\n\n\nAfter killing some nodes, we're going to keep performing VCS ops on the cluster until it fully heals itself and we see no more errors. (We expect some errors until it heals itself.)\n\n\n")

		try := 0
		for {
			err := performRepoOps()
			if err != nil {
				log.Printf("\n\n\nTry #%d (%s): got error %+v\n\n\n", try, time.Since(killedTime), err)
				try++
				time.Sleep(2000 * time.Millisecond)
				continue
			}

			log.Printf("\n\n\nTry #%d: SUCCESS. The cluster healed itself after %s.\n\n\n", try, time.Since(killedTime))
			break
		}
	})
}

func mustParseURL(urlStr string) *url.URL {
	u, err := url.Parse(urlStr)
	if err != nil {
		panic(err.Error())
	}
	return u
}

type repoInfo struct{ vcsType, cloneURL string }

func getUserRepos(t *testing.T, user string) []repoInfo {
	if user == "" {
		// hardcode for speed
		return []repoInfo{
			{"git", "git://github.com/sgtest/python-sample-1.git"},
			{"git", "git://github.com/sgtest/utf8_test.git"},
			{"git", "git://github.com/sgtest/python-sample-0.git"},
			{"git", "git://github.com/sgtest/javascript-nodejs-xrefs-0.git"},
			{"git", "git://github.com/sqs/spans.git"},
			{"git", "git://github.com/sgtest/go-sample-0.git"},
			{"git", "git://github.com/sgtest/ruby-sample-0.git"},
			{"git", "git://github.com/sgtest/rails-sample.git"},
			{"git", "git://github.com/sgtest/js-alias.git"},
			{"git", "git://github.com/sgtest/config1.git"},
			{"git", "git://github.com/sgtest/config0.git"},
			{"git", "git://github.com/sgtest/go-interface-impl.git"},
			{"git", "git://github.com/sgtest/go-interface-def.git"},
			{"git", "git://github.com/atom/welcome.git"},
			{"git", "git://github.com/atom/atom-shell.git"},
			{"git", "git://github.com/atom/snippets.git"},
			{"git", "git://github.com/atom/timecop.git"},
			{"git", "git://github.com/atom/biscotto.git"},
			{"git", "git://github.com/atom/git-utils.git"},
			{"git", "git://github.com/atom/markdown-preview.git"},
			{"git", "git://github.com/atom/language-ruby.git"},
			{"git", "git://github.com/atom/git-diff.git"},
			{"git", "git://github.com/atom/underscore-plus.git"},
			{"git", "git://github.com/atom/text-buffer.git"},
			{"git", "git://github.com/atom/find-and-replace.git"},
		}
	}
	t.Fatal("TODO: fetching other users' repos is not yet implemented")
	panic("unreachable")
}

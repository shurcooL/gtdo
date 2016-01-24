package main

import (
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

// RepoUpdater is a background repository update worker. Repo update requests can be enqueued,
// with debouncing taken care of.
var RepoUpdater *repoUpdater

type repoUpdater struct {
	mu     sync.Mutex
	queue  chan importPathRepoSpec
	recent map[repoSpec]time.Time // Repo spec -> last updated time.
	closed bool                   // After it's set to true, future Enqueue calls will do nothing.

	wg sync.WaitGroup
}

// NewRepoUpdater starts a repository update worker.
func NewRepoUpdater() *repoUpdater {
	ru := &repoUpdater{
		queue:  make(chan importPathRepoSpec, 10),
		recent: make(map[repoSpec]time.Time),
	}
	ru.wg.Add(1)
	go ru.worker()
	return ru
}

// Close disables future Enqueue requests, shuts down all workers, waiting for them to finish.
func (ru *repoUpdater) Close() error {
	ru.mu.Lock()
	close(ru.queue)
	ru.closed = true
	ru.mu.Unlock()

	ru.wg.Wait()

	return nil
}

// Enqueue a request to update the specified repository.
// It's safe to call this concurrently.
// After Close is called, Enqueue will return without doing anything.
func (ru *repoUpdater) Enqueue(repo importPathRepoSpec) {
	ru.mu.Lock()
	defer ru.mu.Unlock()

	if ru.closed {
		return
	}

	now := time.Now()

	// Clear repos that were updated long ago from recent map.
	for rs, lastUpdated := range ru.recent {
		if lastUpdated.Before(now.Add(-20 * time.Second)) {
			delete(ru.recent, rs)
		}
	}

	// Skip if recently updated.
	if _, recent := ru.recent[repo.repoSpec]; recent {
		return
	}

	select {
	case ru.queue <- repo:
		ru.recent[repo.repoSpec] = now
	default:
		// Skip since queue is full.
	}
}

func (ru *repoUpdater) worker() {
	defer ru.wg.Done()

	for rs := range ru.queue {
		started := time.Now()
		fmt.Println("repoUpdater: updating repo", rs)

		u, err := url.Parse(rs.cloneURL)
		if err != nil {
			log.Println(err)
			continue
		}
		repo, err := vs.Repository(rs.vcsType, u)
		if err != nil {
			log.Println(err)
			continue
		}

		result, err := repo.(vcs.RemoteUpdater).UpdateEverything(vcs.RemoteOpts{})
		if err != nil {
			log.Println("repoUpdater: UpdateEverything:", err)
			continue
		}

		fmt.Println("taken:", time.Since(started))

		if result == nil {
			continue
		}
		for _, change := range result.Changes {
			importPathBranch := importPathBranch{
				importPath: rs.importPath,
				branch:     change.Branch,
			}
			fmt.Println("notifying of update all:", importPathBranch)
			sseMu.Lock()
			for _, pv := range sse[importPathBranch] {
				pv.NotifyOutdated()
			}
			sseMu.Unlock()
		}
	}
}

// repoSpec identifies a repository for go-vcs purposes.
type repoSpec struct {
	vcsType  string
	cloneURL string
}

// importPathRepoSpec tracks of repoSpec and importPath. The importPath is needed for frontend
// notifications (while repoSpec is used for go-vcs repo updates.
// TODO: Ideally, in future, might want to change vcs store to be importPath-based instead, then repoSpec can go away
//       and everything can use importPath only. But think about it, maybe importPath alone doesn't carry enough info (like type of VCS).
type importPathRepoSpec struct {
	importPath string
	repoSpec
}

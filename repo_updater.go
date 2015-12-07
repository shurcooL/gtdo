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
	recent map[repoSpec]time.Time // Repo spec -> last updated time.

	queue chan repoSpec

	wg sync.WaitGroup
}

// NewRepoUpdater starts a repository update worker.
func NewRepoUpdater() *repoUpdater {
	ru := &repoUpdater{
		recent: make(map[repoSpec]time.Time),
		queue:  make(chan repoSpec, 10),
	}
	ru.wg.Add(1)
	go ru.worker()
	return ru
}

// Close shuts down all workers, waiting for them to finish.
//
// It should only be called after the last Enqueue call has been made.
func (ru *repoUpdater) Close() error {
	close(ru.queue)
	ru.wg.Wait()
	return nil
}

// Enqueue a request to update the specified repository.
func (ru *repoUpdater) Enqueue(repo repoSpec) {
	ru.mu.Lock()
	defer ru.mu.Unlock()

	now := time.Now()

	// Clear repos that were updated long ago from recent map.
	for rs, lastUpdated := range ru.recent {
		if lastUpdated.Before(now.Add(-20 * time.Second)) {
			delete(ru.recent, rs)
		}
	}

	// Skip if recently updated.
	if _, recent := ru.recent[repo]; recent {
		return
	}

	select {
	case ru.queue <- repo:
		ru.recent[repo] = now
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

		_, err = repo.(vcs.RemoteUpdater).UpdateEverything(vcs.RemoteOpts{})
		if err != nil {
			fmt.Println("repoUpdater: UpdateEverything:", err)
		}

		fmt.Println("taken:", time.Since(started))
	}
}

// repoSpec identifies a repository.
type repoSpec struct {
	vcsType  string
	cloneURL string
}

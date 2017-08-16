package main

import (
	"sort"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

// branchesAndTags fetches branches and tags from repository repo.
// It returns a sorted, de-duplicated list of their names.
func branchesAndTags(repo vcs.Repository) ([]string, error) {
	branches, err := repo.Branches(vcs.BranchesOptions{})
	if err != nil {
		return nil, err
	}
	tags, err := repo.Tags()
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}) // Set of names, for de-duplication.
	for _, branch := range branches {
		set[branch.Name] = struct{}{}
	}
	for _, tag := range tags {
		set[tag.Name] = struct{}{}
	}
	var list []string // List of names, for sorting.
	for name := range set {
		list = append(list, name)
	}
	sort.Strings(list)
	return list, nil
}

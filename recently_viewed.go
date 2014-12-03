package main

import "sync"

var recentlyViewed struct {
	Packages [10]string // Index 0 is the top (most recently viewed Go package).
	lock     sync.RWMutex

	Production bool
}

// sendToTop sends importPath to top of recentlyViewed.Packages.
func sendToTop(importPath string) {
	recentlyViewed.lock.Lock()
	var target int // Index of package that will disappear.
	for i, p := range recentlyViewed.Packages {
		target = i
		if p == importPath {
			break
		}
	}
	// Shift all packages from top to target down by one.
	for ; target > 0; target-- {
		recentlyViewed.Packages[target] = recentlyViewed.Packages[target-1]
	}
	recentlyViewed.Packages[0] = importPath
	recentlyViewed.lock.Unlock()
}

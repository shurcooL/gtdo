package main

import (
	"encoding/gob"
	"os"
	"sync"
)

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

func loadState(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	dec := gob.NewDecoder(file)

	err = dec.Decode(&recentlyViewed.Packages)

	return err
}

func saveState(filename string) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := gob.NewEncoder(file)

	recentlyViewed.lock.RLock()
	err = enc.Encode(recentlyViewed.Packages)
	recentlyViewed.lock.RUnlock()

	return err
}

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

// sendToTop sends importPath to top of recentlyViewed.Packages if it's not already present.
func sendToTop(importPath string) {
	recentlyViewed.lock.Lock()
	defer recentlyViewed.lock.Unlock()
	// Check if package is already present, then do nothing.
	for _, p := range recentlyViewed.Packages {
		if p == importPath {
			return
		}
	}
	// Shift all packages down by one.
	for i := len(recentlyViewed.Packages) - 1; i > 0; i-- {
		recentlyViewed.Packages[i] = recentlyViewed.Packages[i-1]
	}
	recentlyViewed.Packages[0] = importPath
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

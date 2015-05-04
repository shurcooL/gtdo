package datad

import (
	"log"
	"os"
	"strings"
)

const (
	version          = "0.0.1"
	DefaultKeyPrefix = "/datad/"

	nodesPrefix = "/nodes"
	keysPrefix  = "/data"
)

var (
	Log = log.New(os.Stderr, "datad: ", log.Ltime|log.Lmicroseconds|log.Lshortfile)
)

// slash adds a leading slash if path does not contain one.
func slash(path string) string {
	if len(path) == 0 {
		return "/"
	} else if path[0] == '/' {
		return path
	}
	return "/" + path
}

// unslash removes a leading slash from path if it contains one.
func unslash(path string) string {
	return strings.TrimPrefix(path, "/")
}

func trailingSlash(path string) string {
	if strings.HasSuffix(path, "/") {
		return path
	}
	return path + "/"
}

// keyPathJoin removes all slashes on either side of each component and returns
// the components joined by slashes with a leading slash.
func keyPathJoin(components ...string) string {
	var c2 []string
	for _, c := range components {
		c = strings.Trim(c, "/")
		if c != "" {
			c2 = append(c2, c)
		}
	}
	return "/" + strings.Join(c2, "/")
}

// A KeyFunc maps path-space onto key-space.
//
// In other words, it returns the key (a string) of the data stored at path. The
// key, in datad terms, is the unit of storage.
//
// Depending on the type of data, keys and paths may be a 1-to-1 mapping, or
// paths may point to resources inside of a key. For example, you might key on
// repositories clone URLs and allow paths that refer to specific files or
// commits inside of a repository.
type KeyFunc func(path string) (key string, err error)

// IdentityKey is a KeyFunc that treats each path as a key.
func IdentityKey(path string) (string, error) { return path, nil }

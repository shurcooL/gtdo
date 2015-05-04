package datad

import (
	"log"
	"strings"
	"time"
)

var RegistrationTTL = 60 * time.Second

// A Registry contains a bidirectional mapping between data keys and nodes: (1)
// for a given data key, a list of cluster nodes that have the underlying data
// on disk; and (2) for a given node, a list of data keys that it should
// fetch/compute and store on disk.
type Registry struct {
	backend Backend
}

func NewRegistry(b Backend) *Registry {
	return &Registry{b}
}

func (r *Registry) KeysForNode(node string) ([]string, error) {
	return r.backend.ListKeys(keysForNodeDir(node), true)
}

func (r *Registry) NodesForKey(key string) ([]string, error) {
	return r.backend.ListKeys(nodesForKeyDir(key), true)
}

// KeyMap returns a map of keys to a list of their registered nodes.
func (r *Registry) KeyMap() (map[string][]string, error) {
	bkeys, err := r.backend.List(keyPathJoin(registryPrefix, keysPrefix), true)
	if err != nil {
		return nil, err
	}

	// TODO(sqs): divide len(bkeys) by replica count once replicas exist (just an
	// optimization)
	km := make(map[string][]string, len(bkeys))
	for _, bk := range bkeys {
		suffix := "/" + keyNodesSubdir
		if !strings.Contains(bk, suffix) {
			// This backend key is some parent node that is returned in the
			// results but is unneeded for this operation.
			continue
		}

		// Parse the backend key to determine the data key and the registered
		// node.
		parts := strings.Split(bk, suffix)
		if len(parts) != 2 {
			log.Printf("In KeyMap, skipping bad (unparseable) backend key in registry: %q.", bk)
			continue
		}

		key, node := parts[0], strings.TrimPrefix(parts[1], "/")

		if node != "" {
			km[key] = append(km[key], node)
		} else {
			// Record keys that have no nodes.
			km[key] = nil
		}
	}

	return km, nil
}

func (r *Registry) Add(key, node string) error {
	err := r.backend.Set(nodesForKeyDir(key)+"/"+node, "")
	if err != nil {
		return err
	}

	err = r.backend.Set(keysForNodeDir(node)+"/"+key, "")
	if err != nil {
		return err
	}

	return nil
}

func (r *Registry) Remove(key, node string) error {
	err := r.backend.Delete(nodesForKeyDir(key) + "/" + node)
	if err != nil {
		return err
	}

	err = r.backend.Delete(keysForNodeDir(node) + "/" + key)
	if err != nil {
		return err
	}

	return nil
}

const (
	registryPrefix = "registry"
	keyNodesSubdir = "$$nodes"
	nodeKeysSubdir = "$$keys"
)

func nodesForKeyDir(key string) string {
	return keyPathJoin(registryPrefix, keysPrefix, key, keyNodesSubdir)
}

func keysForNodeDir(node string) string {
	return keyPathJoin(registryPrefix, nodesPrefix, node, nodeKeysSubdir)
}

func keyBucket(key string, n int) int {
	var x uint8
	for i := 0; i < len(key); i++ {
		x += key[i]
	}
	return int(x) % n
}

package datad

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-etcd/etcd"
)

var (
	// NodeMembershipTTL is the time-to-live of the etcd key that denotes a
	// node's membership in the cluster.
	NodeMembershipTTL = 10 * time.Second

	// BalanceInterval is the time interval for starting a balancing job on the
	// whole keyspace on each node.
	BalanceInterval = 5 * time.Minute
)

// A Node ensures that the provider's keys are registered and coordinates
// distribution of data among the other nodes in the cluster.
type Node struct {
	Name     string
	Provider Provider

	// Updaters is the maximum number of concurrent calls to Provider.Update
	// that may be executing at any given time on this node.
	Updaters  int
	updateQ   chan string
	updateQMu sync.Mutex

	backend  Backend
	registry *Registry

	Log *log.Logger

	stopChan chan struct{}
}

// NewNode creates a new node to publish data from a provider to the cluster.
// The name ("host:port") is advertised to the cluster and therefore must be
// accessible by the other clients and nodes in the cluster. The name should be
// the host and port where the data on this machine is accessible.
//
// Call Start on this node to begin publishing its keys to the cluster.
func NewNode(name string, b Backend, p Provider) *Node {
	name = cleanNodeName(name)
	return &Node{
		Name:     name,
		Provider: p,
		Updaters: 1,
		updateQ:  make(chan string),
		backend:  b,
		registry: NewRegistry(b),
		Log:      log.New(os.Stderr, "", log.Ltime|log.Lmicroseconds|log.Lshortfile),
		stopChan: make(chan struct{}),
	}
}

func cleanNodeName(name string) string {
	name = strings.TrimPrefix(name, "http://")
	parseName := name
	if !strings.Contains(parseName, ":") {
		parseName += ":80"
	}
	_, _, err := net.SplitHostPort(parseName)
	if err != nil {
		panic("NewNode: bad name '" + name + "': " + err.Error() + " (name should be 'host:port')")
	}
	return name
}

// Start begins advertising this node's provider's keys to the
// cluster.
func (n *Node) Start() error {
	n.logf("Starting node %s.", n.Name)

	err := n.joinCluster()
	if err != nil {
		n.logf("Failed to join cluster: %s", err)
		return err
	}

	go func() {
		err = n.registerExistingKeys()
		if err != nil {
			n.logf("Failed to register existing keys: %s", err)
		}
	}()

	go n.watchRegisteredKeys()
	go n.balancePeriodically()
	go n.startUpdater()

	return nil
}

// Stop deregisters this node's keys and stops background processes
// for this node.
func (n *Node) Stop() error {
	close(n.stopChan)
	return nil
}

// joinCluster adds this node's provider to the cluster, making it available to
// receive requests for and be assigned keys. It then periodically re-adds this
// node to the cluster before the TTL on the etcd cluster membership key
// elapses.
func (n *Node) joinCluster() error {
	err := n.refreshClusterMembership()
	if err != nil {
		return err
	}

	if NodeMembershipTTL < time.Second {
		panic("NodeMembershipTTL must be at least 2 seconds")
	}

	go func() {
		t := time.NewTicker(NodeMembershipTTL - 800*time.Millisecond)
		for {
			select {
			case <-t.C:
				err := n.refreshClusterMembership()
				if err != nil {
					n.logf("Error refreshing node %s cluster membership: %s.", n.Name, err)
				}
			case <-n.stopChan:
				t.Stop()
				return
			}
		}
	}()

	return nil
}

func (n *Node) refreshClusterMembership() error {
	err := n.backend.SetDir(keyPathJoin(nodesPrefix, n.Name), uint64(NodeMembershipTTL/time.Second))
	if isEtcdErrorCode(err, 102) {
		err = n.backend.UpdateDir(keyPathJoin(nodesPrefix, n.Name), uint64(NodeMembershipTTL/time.Second))
	}
	return err
}

// watchRegisteredKeys watches the registry for changes to the list of keys that
// this node is registered for, or for modifications of existing registrations
// (e.g., updates requested).
func (n *Node) watchRegisteredKeys() error {
	watchKey := keysForNodeDir(n.Name)
	fullKey := n.backend.(*EtcdBackend).fullKey(watchKey)

	recv := make(chan *etcd.Response, 10)
	stopWatch := make(chan bool, 1)

	// Receive watched changes.
	go func() {
		for {
			select {
			case resp, ok := <-recv:
				if !ok {
					return
				}
				key := strings.TrimPrefix(resp.Node.Key, fullKey+"/")
				n.logf("Registry changed: %s on key %q.", resp.Action, key)
				if !strings.Contains(strings.ToLower(resp.Action), "delete") {
					n.logf("Queueing update for key %q in data source (in response to registry %s).", key, resp.Action)
					n.updateQ <- key
				}
			case <-n.stopChan:
				n.logf("Stopping registry watcher.")
				stopWatch <- true
				return
			}
		}
	}()

	_, err := n.backend.(*EtcdBackend).etcd.Watch(fullKey, 0, true, recv, stopWatch)
	if err != etcd.ErrWatchStoppedByUser {
		return err
	}
	return nil
}

// registerExistingKeys examines this node's provider's local storage for data
// and registers each data key it finds. This means that when the node starts
// up, it's immediately able to receive requests for the data it already has on
// disk. Without this, the cluster would not know that this node's provider has
// these keys.
func (n *Node) registerExistingKeys() error {
	n.logf("Finding existing keys to register... (this may take a while)")

	keys, err := n.Provider.Keys("")
	if err != nil {
		return err
	}

	if len(keys) == 0 {
		return nil
	}

	n.logf("Found %d existing keys in provider: %v. Registering existing keys to this node...", len(keys), keys)
	for _, key := range keys {
		err := n.registry.Add(key, n.Name)
		if err != nil {
			return err
		}
	}
	n.logf("Finished registering existing %d keys to this node.", len(keys))

	return nil
}

func (n *Node) startUpdater() {
	keyToUpdate := make(chan string)

	// Use a map to avoid updating the same key concurrently. If a key's value
	// is false, it's queued; if true, its update is in progress.
	pending := make(map[string]bool)

	type keyStatus struct {
		key       string
		completed bool
	}

	status := make(chan keyStatus)

	// Consume queue and distribute keys to updaters.
	go func() {
		for {
			select {
			case s := <-status:
				if s.completed {
					delete(pending, s.key)
				} else {
					pending[s.key] = true
				}
			case key := <-n.updateQ:
				if _, isPending := pending[key]; !isPending {
					pending[key] = false
					go func() { keyToUpdate <- key }() // TODO(sqs): hacky
					if len(pending) > n.Updaters {
						n.logf("%d key updates pending: %v (false=queued, true=update in progress).", len(pending), pending)
					}
				}
			case <-n.stopChan:
				return
			}
		}
	}()

	if n.Updaters <= 0 {
		panic("invalid Updaters value for node (Updaters <= 0)")
	}

	// Updaters.
	for i := 0; i < n.Updaters; i++ {
		go func() {
			for {
				select {
				case key := <-keyToUpdate:
					status <- keyStatus{key, false}
					err := n.Provider.Update(key)
					if err == nil {
						n.logf("Update succeeded for key %q.", key)
					} else {
						n.logf("Update failed for key %q: %s.", key, err)
					}
					status <- keyStatus{key, true}
				case <-n.stopChan:
					return
				}
			}
		}()
	}
}

// startBalancer starts a periodic process that balances the distribution of
// keys to nodes.
func (n *Node) balancePeriodically() {
	// TODO(sqs): allow tweaking the balance interval
	t := time.NewTicker(BalanceInterval)
	for {
		select {
		case <-t.C:
			err := n.balance()
			if err != nil {
				n.logf("Error balancing: %s. Will retry next balance interval.", err)
			}
		case <-n.stopChan:
			t.Stop()
			return
		}
	}
}

// balance examines all keys and ensures each key has a registered node. If not,
// it registers a node for the key. This lets the cluster heal itself after a
// node goes down (which causes keys to be orphaned).
func (n *Node) balance() error {
	keyMap, err := n.registry.KeyMap()
	if err != nil {
		return err
	}

	if len(keyMap) == 0 {
		return nil
	}

	c := NewClient(n.backend)
	clusterNodes, err := c.NodesInCluster()
	if err != nil {
		return err
	}

	// TODO(sqs): allow tweaking this parameter
	x := rand.Intn(10)
	start := time.Now()

	n.logf("Balancer: starting on %d keys, with known cluster nodes %v.", len(keyMap), clusterNodes)
	actions := 0
	iterations := 0
	for key, nodes := range keyMap {
		if maxDuration := BalanceInterval / 2; time.Since(start) > maxDuration {
			// Rough heuristic that the KeyMap is stale, so let's end this
			// balance run. The next balance run won't have to redo the work we
			// did.
			n.logf("Balancer: ending before complete because max balance duration %s elapsed. Checked %d/%d entries in KeyMap, %d non-read ops. Will resume in next balance run.", maxDuration, iterations, len(keyMap), actions)
			return nil
		}

		iterations++

		if len(nodes) == 0 {
			regNode := clusterNodes[keyBucket(key, len(clusterNodes))]

			n.logf("Balancer: found unregistered key %q; registering it to node %s.", key, regNode)

			// TODO(sqs): optimize this by only adding if not exists, and then
			// seeing if it exists (to avoid potentially duplicating work).
			err := c.registry.Add(key, regNode)
			if err != nil {
				return err
			}

			actions++
			continue
		}

		// Check liveness of key on each node.
		for _, node := range nodes {
			t, err := c.transportForKey(key, nil, []string{node})
			if err != nil {
				return err
			}

			hc := &http.Client{Transport: t, Timeout: 2 * time.Second}
			resp, err := hc.Get(slash(key))
			if err != nil {
				actions++
				n.logf("Balancer: liveness check failed for key %q on node %s: %s. Client deregistered key from node.", key, node, err)
			}
			if resp != nil {
				resp.Body.Close()
			}
		}

		// Update keys on this node (but not each time, to avoid overloading the
		// origin servers).
		if x == 0 {
			for _, node := range nodes {
				if node == n.Name {
					n.logf("Balancer: queueing update for key %q in data source on current node.", key)
					n.updateQ <- key
					actions++
				}
			}
		}
	}
	n.logf("Balancer: completed in %s for %d keys (%d non-read actions performed).", time.Since(start), len(keyMap), actions)

	return nil
}

func (n *Node) logf(format string, a ...interface{}) {
	if n.Log != nil {
		n.Log.Printf(fmt.Sprintf("Node %s: ", n.Name)+format, a...)
	}
}

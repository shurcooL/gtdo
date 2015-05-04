package datad

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

// A Client routes requests for data.
type Client struct {
	// KeyURLPrefix, if set, is prepended to all HTTP request URL paths using
	// the transport from TransportForKey. It is useful when your keys refer to
	// data hosted on a HTTP server at somewhere other than the root path. For
	// example, if the datad key "/foo" refers to "http://example.com/api/foo",
	// then KeyURLPrefix would be "/api/".
	KeyURLPrefix string

	backend Backend

	registry *Registry

	Log *log.Logger
}

func NewClient(b Backend) *Client {
	return &Client{
		backend:  b,
		registry: NewRegistry(b),
		Log:      log.New(os.Stderr, "datad client: ", log.Ltime|log.Lmicroseconds|log.Lshortfile),
	}
}

var ErrNoNodesForKey = errors.New("key has no nodes")

// NodesInCluster returns a list of all nodes in the cluster.
func (c *Client) NodesInCluster() ([]string, error) {
	return c.backend.List(nodesPrefix, false)
}

// NodesForKey returns a list of nodes that, according to the registry, hold the
// data specified by key.
func (c *Client) NodesForKey(key string) ([]string, error) {
	return c.registry.NodesForKey(key)
}

// Update updates key from the data source on the nodes that are registered to
// it. If key is not registered to any nodes, a node is registered for it and
// the key is created on that node.
func (c *Client) Update(key string) (nodes []string, err error) {
	return c.update(key, nil, nil, nil)
}

var ErrNoAvailableNodesForRegistration = errors.New("no available nodes to register key with")

// update is like Update, but takes nodesForKey and clusterNodes params as an
// optimization for callers who already know their values. Also, the key will
// not be registered to any nodes in excludeNodes (if no other nodes are
// available, an error is returned).
func (c *Client) update(key string, nodesForKey []string, clusterNodes []string, excludeNodes map[string]struct{}) (nodes []string, err error) {
	if nodesForKey == nil {
		nodesForKey, err = c.NodesForKey(key)
		if err != nil {
			return nil, err
		}
	}

	if len(nodesForKey) == 0 {
		// Register a node for the key.
		if clusterNodes == nil {
			clusterNodes, err = c.NodesInCluster()
			if err != nil {
				return nil, err
			}
		}

		// Exclude nodes.
		if len(excludeNodes) > 0 {
			var clusterNodes2 []string
			for _, cnode := range clusterNodes {
				if _, exclude := excludeNodes[cnode]; !exclude {
					clusterNodes2 = append(clusterNodes2, cnode)
				}
			}
			clusterNodes = clusterNodes2
		}

		if len(clusterNodes) == 0 {
			return nil, ErrNoAvailableNodesForRegistration
		}

		// Try to choose the same node as other clients that might be calling Update on the same key concurrently.
		regNode := clusterNodes[keyBucket(key, len(clusterNodes))]

		c.logf("Key to update does not exist yet: %q; registering key to node %s (will trigger update).", key, regNode)

		// TODO(sqs): optimize this by only adding if not exists, and then
		// seeing if it exists (to avoid potentially duplicating work).
		err = c.registry.Add(key, regNode)
		if err != nil {
			return nil, err
		}

		// The call to Add will trigger the update on the node, so we're done.
		return []string{regNode}, nil
	}

	for i, node := range nodesForKey {
		c.logf("Triggering update of key %q on node %s (%d/%d)...", key, node, i+1, len(nodesForKey))
		// Each node watches its list of registered keys, so just re-adding it
		// to the registry will trigger an update.
		err = c.registry.Add(key, node)
		if err != nil {
			return nil, err
		}
	}
	c.logf("Finished triggering updates of key %q on %d nodes (%v).", key, len(nodesForKey), nodesForKey)

	return nodesForKey, nil
}

// TransportForKey returns a HTTP transport (http.RoundTripper) optimized for
// accessing the data specified by key.
//
// If key is not registered to any nodes, ErrNoNodesForKey is returned.
func (c *Client) TransportForKey(key string, underlying http.RoundTripper) (*KeyTransport, error) {
	nodes, err := c.NodesForKey(key)
	if err != nil {
		return nil, err
	}

	return c.transportForKey(key, underlying, nodes)
}

// transportForkey is like TransportForKey but is optimized for callers who
// already know the nodes that are registered to key.
func (c *Client) transportForKey(key string, underlying http.RoundTripper, nodes []string) (*KeyTransport, error) {
	if underlying == nil {
		underlying = http.DefaultTransport
	}
	return &KeyTransport{key: key, nodes: nodes, c: c, transport: underlying}, nil
}

type KeyTransport struct {
	key       string
	nodes     []string
	c         *Client
	transport http.RoundTripper

	// nodesMu synchronizes access to nodes.
	nodesMu sync.Mutex
}

// KeyTransportError denotes that the key transport's RoundTrip failed. It
// records the individual errors for each node it attempted to contact.
type KeyTransportError struct {
	URL        string
	NodeErrors map[string]error

	// OtherError is an error encountered while trying to register key with other nodes.
	OtherError error
}

func (e *KeyTransportError) Error() string {
	summary := make([]string, len(e.NodeErrors))
	i := 0
	for node, err := range e.NodeErrors {
		summary[i] = fmt.Sprintf("%s [node %s]", truncate(err.Error(), 75, "..."), node)
		i++
	}
	return fmt.Sprintf("no nodes responded successfully for %q (node errors: %s) (other error: %v)", e.URL, strings.Join(summary, "; "), e.OtherError)
}

// RoundTrip implements http.RoundTripper. If at least one node responds
// successfully, no error is returned. If all nodes fail to respond
// successfully, a *KeyTransportError is returned with the errors from each
// node.
func (t *KeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request so we can modify the URL.
	req2 := *req

	// Copy over everything important but the URL host (because we'll try different hosts).
	req2.URL = &url.URL{
		Scheme:   req.URL.Scheme,
		Path:     t.c.KeyURLPrefix + req.URL.Path,
		RawQuery: req.URL.RawQuery,
		Fragment: req.URL.Fragment,
	}
	if req2.URL.Scheme == "" {
		req2.URL.Scheme = "http"
	}

	t.nodesMu.Lock()
	nodes := t.nodes
	t.nodesMu.Unlock()

	// Track failed nodes so we don't reregister this key to them.
	failedNodes := make(map[string]struct{})

	// Track the errors we saw from each node's response.
	nodeErrors := make(map[string]error)

	for i, node := range nodes {
		// TODO(sqs): this code assumes the node is a "host:port".
		req2.URL.Host = node

		resp, err := t.transport.RoundTrip(&req2)
		if err == nil && (resp.StatusCode >= 200 && resp.StatusCode <= 399) {
			return resp, nil
		}

		if err == nil {
			defer resp.Body.Close()
			var body []byte
			body, err = ioutil.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}
			err = &HTTPError{resp.StatusCode, string(bytes.TrimSpace(body))}
		}

		// Remove this node from the registry and from t.nodes.
		t.c.logf("Transport for key %q: HTTP request for %q failed (%s); deregistering node %q from key.", t.key, req.URL, err, node)
		if err := t.c.registry.Remove(t.key, node); err != nil && !isEtcdKeyNotExist(err) {
			return nil, err
		}
		t.nodesMu.Lock()
		t.nodes = append(t.nodes[:i], t.nodes[i:]...)
		t.nodesMu.Unlock()

		failedNodes[node] = struct{}{}
		nodeErrors[node] = err
	}

	kte := &KeyTransportError{URL: req2.URL.String(), NodeErrors: nodeErrors}

	if len(nodes) == 0 {
		kte.OtherError = ErrNoNodesForKey
	}

	// If we get here, then no nodes responded successfully.
	t.c.logf("Transport for key %q: No nodes' data sources responded successfully to request for %q. Registering key to a new node and triggering an update.", t.key, req.URL)

	// Register this key with a new node and trigger an update.
	regNodes, err := t.c.update(t.key, []string{}, nil, failedNodes)
	if err != nil {
		kte.OtherError = err
		return nil, kte
	}

	// Use the newly registered node(s) as the new destinations for this transport.
	t.c.logf("Transport for key %q: Registered key to new nodes %v and triggered an update.", t.key, regNodes)
	t.nodesMu.Lock()
	t.nodes = regNodes
	t.nodesMu.Unlock()

	return nil, kte
}

// CancelRequest is to allow a nonzero Timeout on the http.Client. TODO(sqs):
// check this.
func (t *KeyTransport) CancelRequest(req *http.Request) {
	// no-op
}

// SyncWithRegistry updates the list of nodes that this transport attempts to
// make HTTP requests to. The new nodes are looked up in the registry.
func (t *KeyTransport) SyncWithRegistry() error {
	nodes, err := t.c.NodesForKey(t.key)
	if err != nil {
		return err
	}

	t.c.logf("Transport for key %q: Synced nodes with registry. New nodes: %v. Old nodes: %v.", t.key, nodes, t.nodes)
	t.nodesMu.Lock()
	t.nodes = nodes
	t.nodesMu.Unlock()

	return nil
}

func (c *Client) logf(format string, a ...interface{}) {
	if c.Log != nil {
		c.Log.Printf(format, a...)
	}
}

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("http %d: %s", e.StatusCode, e.Body) }

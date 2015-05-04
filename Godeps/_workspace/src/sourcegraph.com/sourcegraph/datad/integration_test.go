package datad

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	etcd_client "github.com/coreos/go-etcd/etcd"
)

func TestIntegration_Simple(t *testing.T) {
	withEtcd(t, func(ec *etcd_client.Client) {
		b := NewEtcdBackend("/", ec)

		data := data{"/key": {"val"}}

		n := NewNode("n", b, noopUpdateProvider{data})

		c := NewClient(b)

		// Check that the key is unroutable because although it exists on the
		// provider, the provider has not yet synced to the registry (this
		// occurs when we start the node below).
		nodes, err := c.NodesForKey("/key")
		if err != nil {
			t.Fatal(err)
		}
		if len(nodes) != 0 {
			t.Errorf("got NodesForKey == %v, want empty", nodes)
		}

		// Start the node.
		n.Start()
		defer n.Stop()

		// Check that the node has added itself to the cluster.
		nodes, err = c.NodesInCluster()
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{n.Name}; !reflect.DeepEqual(nodes, want) {
			t.Errorf("got NodesInCluster == %v, want %v", nodes, want)
		}

		// After calling n.Start(), the key should be routable (since it is
		// persisted locally on the node).
		err = n.registerExistingKeys()
		if err != nil {
			t.Fatal(err)
		}
		nodes, err = c.NodesForKey("/key")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{n.Name}; !reflect.DeepEqual(nodes, want) {
			t.Errorf("got NodesForKey == %v, want %v", nodes, want)
		}
	})
}

func TestIntegration_NodeTTL(t *testing.T) {
	if testing.Short() {
		t.Skip("requires at least 1s sleep")
	}

	withEtcd(t, func(ec *etcd_client.Client) {
		b := NewEtcdBackend("/", ec)

		data := data{"/key": {"val"}}

		c := NewClient(b)

		// Shorten the NodeMembershipTTL.
		origTTL := NodeMembershipTTL
		NodeMembershipTTL = time.Second // minimum for etcd
		defer func() { NodeMembershipTTL = origTTL }()

		n := NewNode("n", b, noopUpdateProvider{data})
		must(t, n.Start())

		// Ensure that the node is in the cluster.
		nodes, err := c.NodesInCluster()
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{n.Name}; !reflect.DeepEqual(nodes, want) {
			t.Errorf("got NodesInCluster == %v, want %v", nodes, want)
		}

		// Unjoin the cluster.
		must(t, n.Stop())

		// Sleep so that the TTL elapses.
		time.Sleep(NodeMembershipTTL + time.Millisecond*500)

		// Ensure that the node is no longer in the cluster.
		nodes, err = c.NodesInCluster()
		if err != nil {
			t.Fatal(err)
		}
		if len(nodes) != 0 {
			t.Errorf("got NodesForKey == %v, want empty", nodes)
		}
	})
}

// Test that a key is created upon demand (in the data source) if it does not exist yet.
func TestIntegration_Update_CreateKey(t *testing.T) {
	withEtcd(t, func(ec *etcd_client.Client) {
		b := NewEtcdBackend("/", ec)

		data := data{}

		ds := httptest.NewServer(dataHandler(data))
		defer ds.Close()

		n := NewNode(ds.URL, b, fakeUpdateProvider{data: data})
		n.Start()
		defer n.Stop()

		c := NewClient(b)

		// Check that no nodes are registered for "/newkey".
		nodes, err := c.NodesForKey("/newkey")
		if err != nil {
			t.Fatal(err)
		}
		if len(nodes) != 0 {
			t.Errorf("got NodesForKey == %v, want empty", nodes)
		}

		// Test that calling Update will update the key and register it to the
		// existing node.
		nodes, err = c.Update("/newkey")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{n.Name}; !reflect.DeepEqual(nodes, want) {
			t.Errorf("got NodesForKey == %v, want %v", nodes, want)
		}

		time.Sleep(100 * time.Millisecond)

		// Test that the data source for the key now contains the key's value
		// (i.e., test that it was indeed created).
		transport, err := c.TransportForKey("/newkey", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp := httpGet("", t, transport, "/newkey")
		if want := "val0"; resp != want {
			t.Errorf("got response == %q, want %q", resp, want)
		}
	})
}

// Test that a key is updated.
func TestIntegration_Update_UpdateKey(t *testing.T) {
	withEtcd(t, func(ec *etcd_client.Client) {
		b := NewEtcdBackend("/", ec)

		data := data{"key": {"initialVal"}}

		ds := httptest.NewServer(dataHandler(data))
		defer ds.Close()

		n := NewNode(ds.URL, b, fakeUpdateProvider{data: data})
		n.Start()
		defer n.Stop()

		c := NewClient(b)

		// Test that calling Update will update the key and return the node that
		// it was already registered to.
		nodes, err := c.Update("/key")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{n.Name}; !reflect.DeepEqual(nodes, want) {
			t.Errorf("got NodesForKey == %v, want %v", nodes, want)
		}

		time.Sleep(100 * time.Millisecond)

		// Test that the data source for the key now contains the key's updated
		// value (i.e., test that it was indeed updated).
		transport, err := c.TransportForKey("/key", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp := httpGet("", t, transport, "/key")
		if want := "val0"; resp != want {
			t.Errorf("got response == %q, want %q", resp, want)
		}
	})
}

// Test that a key is deregistered from a node if the node's data source
// returns HTTP errors.
func TestIntegration_DeregisterFailingDataSources(t *testing.T) {
	withEtcd(t, func(ec *etcd_client.Client) {
		b := NewEtcdBackend("/", ec)

		// The "/key" key will be registered to a provider that's no longer up.
		data := data{"/key": {"val"}}

		ds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "dummy error", http.StatusInternalServerError)
		}))
		defer ds.Close()

		badN := NewNode(ds.URL, b, noopUpdateProvider{data})
		badN.Start()
		defer badN.Stop()

		c := NewClient(b)

		// Check that badN is registered for "/key".
		err := badN.registerExistingKeys()
		if err != nil {
			t.Fatal(err)
		}
		nodes, err := c.NodesForKey("/key")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{badN.Name}; !reflect.DeepEqual(nodes, want) {
			t.Errorf("got NodesForKey == %v, want %v", nodes, want)
		}

		// Test that the KeyTransport will deregister "/key" from badN when it
		// notices that the HTTP request fails.
		transport, err := c.TransportForKey("/key", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = (&http.Client{Transport: transport}).Get("/key")
		if err == nil || !strings.Contains(err.Error(), "dummy error") {
			t.Errorf(`got DataTransport get error %v, want "dummy error"`, err)
		}

		// Test that the "/key" key is unregistered from the failing server.
		nodes, err = c.NodesForKey("/key")
		if err != nil {
			t.Fatal(err)
		}
		if len(nodes) != 0 {
			t.Errorf("got NodesForKey == %v, want empty", nodes)
		}
	})
}

// Test that accessing a key using the KeyTransport succeeds as long as any of
// the nodes respond (i.e., KeyTransport tries all nodes).
func TestIntegration_KeyTransportHandleUnreachableDataSources(t *testing.T) {
	withEtcd(t, func(ec *etcd_client.Client) {
		b := NewEtcdBackend("/", ec)

		// The "/key" key will be registered to a provider that's no longer up.
		data := data{"/key": {"val"}}

		// To ensure we *first* try to access the bad node, keep creating test
		// servers until we the bad server's URL sorts first lexicographically.
		calledBadDSHandler := false
		var badDSURL, goodDSURL string
		for {
			// Start a data source that fails to respond to HTTP requests.
			badDS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calledBadDSHandler = true
				http.Error(w, "dummy error", http.StatusInternalServerError)
			}))

			// Start a data source that successfully responds to HTTP requests.
			goodDS := httptest.NewServer(dataHandler(data))

			if badDS.URL < goodDS.URL {
				defer badDS.Close()
				defer goodDS.Close()
				badDSURL, goodDSURL = badDS.URL, goodDS.URL
				break
			} else {
				badDS.Close()
				goodDS.Close()
			}
		}

		badN := NewNode(badDSURL, b, noopUpdateProvider{data})
		badN.Start()
		defer badN.Stop()

		goodN := NewNode(goodDSURL, b, noopUpdateProvider{data})
		goodN.Start()
		defer goodN.Stop()

		c := NewClient(b)

		// Check that both badN and goodN are registered for "/key".
		err := badN.registerExistingKeys()
		if err != nil {
			t.Fatal(err)
		}
		err = goodN.registerExistingKeys()
		if err != nil {
			t.Fatal(err)
		}
		nodes, err := c.NodesForKey("/key")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{badN.Name, goodN.Name}; !reflect.DeepEqual(nodes, want) {
			t.Errorf("got NodesForKey == %v, want %v", nodes, want)
		}

		// Test that the KeyTransport will deregister "/key" from badN
		// when it notices that the HTTP request fails.
		transport, err := c.TransportForKey("/key", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp := httpGet("", t, transport, "/key")
		if want := "val"; resp != want {
			t.Errorf("got response == %q, want %q", resp, want)
		}

		if !calledBadDSHandler {
			t.Error("!calledBadDSHandler")
		}
	})
}

// Test that keys with no registered nodes are periodically re-registered to new nodes.
func TestIntegration_Balance_RegisterUnregisteredKeys(t *testing.T) {
	withEtcd(t, func(ec *etcd_client.Client) {
		b := NewEtcdBackend("/", ec)

		data := data{}

		ds := httptest.NewServer(dataHandler(data))
		defer ds.Close()

		n := NewNode(ds.URL, b, fakeUpdateProvider{data: data})
		n.Start()
		defer n.Stop()

		c := NewClient(b)

		// Simulate what would happen when a key's node dies: previously it was
		// registered, then it becomes unregistered, which means the key is
		// still in the registry as a directory with no subdirectories (i.e., no
		// nodes registered to it).
		err := c.registry.Add("key", "deadnode")
		if err != nil {
			t.Fatal(err)
		}
		err = c.registry.Remove("key", "deadnode")
		if err != nil {
			t.Fatal(err)
		}

		// Check that the key has no registered nodes.
		nodes, err := c.NodesForKey("/key")
		if err != nil {
			t.Fatal(err)
		}
		if len(nodes) != 0 {
			t.Errorf("got NodesForKey == %v, want empty", nodes)
		}

		// Trigger a balance on the live node.
		err = n.balance()
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(100 * time.Millisecond)

		// Test that there's now a node registered for the key.
		nodes, err = c.NodesForKey("/key")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{n.Name}; !reflect.DeepEqual(nodes, want) {
			t.Errorf("got NodesForKey == %v, want %v", nodes, want)
		}

		// Test that the data source for the key contains its value.
		transport, err := c.TransportForKey("/key", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp := httpGet("", t, transport, "/key")
		if want := "val0"; resp != want {
			t.Errorf("got response == %q, want %q", resp, want)
		}
	})
}

// func TestIntegration_TwoNodes(t *testing.T) {
// 	data1 := map[string]datum{"/key": {"valA", "0"}}
// 	fakeServer1 := NewFakeServer(data1)
// 	dataServer1 := httptest.NewServer(fakeServer1)
// 	defer dataServer1.Close()
// 	providerServer1 := httptest.NewServer(NewProviderHandler(fakeServer1))
// 	defer providerServer1.Close()

// 	data2 := map[string]datum{"/bob": {"valB", "1"}}
// 	fakeServer2 := NewFakeServer(data2)
// 	dataServer2 := httptest.NewServer(fakeServer2)
// 	defer dataServer2.Close()
// 	providerServer2 := httptest.NewServer(NewProviderHandler(fakeServer2))
// 	defer providerServer2.Close()

// 	c := NewClient(NewInMemoryBackend(nil))

// 	// Add the servers.
// 	err := c.AddProvider(providerServer1.URL, dataServer1.URL)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	err = c.AddProvider(providerServer2.URL, dataServer2.URL)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	// Check that they were added.
// 	nodes, err := c.NodesInCluster()
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	sort.Strings(nodes)
// 	wantNodes := []string{providerServer1.URL, providerServer2.URL}
// 	sort.Strings(wantNodes)
// 	if !reflect.DeepEqual(nodes, wantNodes) {
// 		t.Errorf("got nodes == %v, want %v", nodes, wantNodes)
// 	}

// 	// Register the servers' existing data.
// 	err = c.RegisterKeysOnProvider(providerServer1.URL)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	err = c.RegisterKeysOnProvider(providerServer2.URL)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	// After calling RegisterKeysOnProvider, the keys should be routable.

// 	// "/key" is on server 1.
// 	dataURL, err := c.DataURL("/key")
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	if want := dataServer1.URL; dataURL.String() != want {
// 		t.Errorf("got DataURL == %q, want %q", dataURL, want)
// 	}

// 	// "/bob" is on server 2.
// 	dataURL, err = c.DataURL("/bob")
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	if want := dataServer2.URL; dataURL.String() != want {
// 		t.Errorf("got DataURL == %q, want %q", dataURL, want)
// 	}
// }

// func TestIntegration_TwoNodes_DifferentVersions(t *testing.T) {
// 	t.Skip("not yet implemented")

// 	data1 := map[string]datum{"/key": {"valA", "0"}}
// 	fakeServer1 := NewFakeServer(data1)
// 	dataServer1 := httptest.NewServer(fakeServer1)
// 	defer dataServer1.Close()
// 	providerServer1 := httptest.NewServer(NewProviderHandler(fakeServer1))
// 	defer providerServer1.Close()

// 	data2 := map[string]datum{"/key": {"valB", "1"}}
// 	fakeServer2 := NewFakeServer(data2)
// 	dataServer2 := httptest.NewServer(fakeServer2)
// 	defer dataServer2.Close()
// 	providerServer2 := httptest.NewServer(NewProviderHandler(fakeServer2))
// 	defer providerServer2.Close()

// 	c := NewClient(NewInMemoryBackend(nil))

// 	// Add the servers.
// 	err := c.AddProvider(providerServer1.URL, dataServer1.URL)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	err = c.AddProvider(providerServer2.URL, dataServer2.URL)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	// Register the servers' existing data.
// 	err = c.RegisterKeysOnProvider(providerServer1.URL)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	err = c.RegisterKeysOnProvider(providerServer2.URL)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	// Server 1 has version 0 of "/key" and server 2 has version 1 of
// 	// "/key". TODO(sqs): The second call to RegisterKeysOnProvider recognize
// 	// this and trigger an update on server.

// 	// After the updates, they should both be at version 1 (TODO(sqs): or maybe they both
// 	// update from the source, since it's hard to know which is the newer one).
// 	dvs, err := c.DataURLVersions("/key")
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	for dataURL, ver := range dvs {
// 		if want := "1"; ver != want {
// 			t.Errorf("got dataURL %q version == %q, want %q", dataURL, ver, want)
// 		}
// 	}
// }

func httpGet(label string, t *testing.T, transport http.RoundTripper, url string) string {
	c := &http.Client{Transport: transport}
	resp, err := c.Get(url)
	if err != nil {
		t.Fatalf("%s (%s): %s", label, url, err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s (%s): %s", label, url, err)
	}
	return string(body)
}

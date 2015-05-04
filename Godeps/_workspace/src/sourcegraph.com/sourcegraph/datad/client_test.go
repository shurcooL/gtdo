package datad

import (
	"reflect"
	"testing"

	etcd_client "github.com/coreos/go-etcd/etcd"
)

func TestClient_NodesInCluster(t *testing.T) {
	withEtcd(t, func(ec *etcd_client.Client) {
		b := NewEtcdBackend("/", ec)
		c := NewClient(b)
		n := NewNode("example.com", b, NoopProvider{})

		nodes, err := c.NodesInCluster()
		if err != nil {
			t.Fatal(err)
		}
		if len(nodes) != 0 {
			t.Errorf("got NodesInCluster == %v, want 0", nodes)
		}

		n.Start()
		defer n.Stop()

		nodes, err = c.NodesInCluster()
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{n.Name}; !reflect.DeepEqual(nodes, want) {
			t.Errorf("got NodesInCluster == %v, want %v", nodes, want)
		}
	})
}

package datad

import (
	"reflect"
	"testing"

	etcd_client "github.com/coreos/go-etcd/etcd"
)

func TestRegistry(t *testing.T) {
	withEtcd(t, func(ec *etcd_client.Client) {
		b := NewEtcdBackend("/", ec)
		r := NewRegistry(b)

		keys, err := r.KeysForNode("n")
		if err != nil {
			t.Fatal(err)
		}
		if len(keys) != 0 {
			t.Errorf("got KeysForNode == %v, want empty", keys)
		}

		nodes, err := r.NodesForKey("k")
		if err != nil {
			t.Fatal(err)
		}
		if len(nodes) != 0 {
			t.Errorf("got NodesForKey == %v, want empty", nodes)
		}

		// Add some mappings.
		err = r.Add("k", "n")
		if err != nil {
			t.Fatal(err)
		}
		err = r.Add("l/m", "n")
		if err != nil {
			t.Fatal(err)
		}

		// Test that the mappings are set.
		keys, err = r.KeysForNode("n")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"k", "l/m"}; !reflect.DeepEqual(keys, want) {
			t.Errorf("got KeysForNode == %v, want %v", keys, want)
		}

		nodes, err = r.NodesForKey("k")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"n"}; !reflect.DeepEqual(nodes, want) {
			t.Errorf("got NodesForKey == %v, want %v", nodes, want)
		}

		nodes, err = r.NodesForKey("l/m")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"n"}; !reflect.DeepEqual(nodes, want) {
			t.Errorf("got NodesForKey == %v, want %v", nodes, want)
		}

		// Remove the mapping for l/m.
		err = r.Remove("l/m", "n")
		if err != nil {
			t.Fatal(err)
		}

		// Test that the mapping was removed.

		keys, err = r.KeysForNode("n")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"k"}; !reflect.DeepEqual(keys, want) {
			t.Errorf("got KeysForNode == %v, want %v", keys, want)
		}

		keyMap, err := r.KeyMap()
		if err != nil {
			t.Fatal(err)
		}
		if want := map[string][]string{"k": []string{"n"}, "l/m": nil}; !reflect.DeepEqual(keyMap, want) {
			t.Errorf("got KeyMap == %v, want %v", keyMap, want)
		}

		nodes, err = r.NodesForKey("l/m")
		if err != nil {
			t.Fatal(err)
		}
		if len(nodes) != 0 {
			t.Errorf("got NodesForKey == %v, want empty", nodes)
		}
	})
}

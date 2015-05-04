package datad

import (
	"reflect"
	"testing"
)

func testBackend(t *testing.T, b Backend) {
	v, err := b.Get("dir/key")
	if err != ErrKeyNotExist {
		t.Fatal(err)
	}
	if v != "" {
		t.Errorf("got v == %q, want empty", v)
	}

	// List (empty)
	keys, err := b.List("dir", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Errorf("got keys == %v, want empty", keys)
	}

	// Set
	wantV := "v"
	err = b.Set("dir/key", wantV)
	if err != nil {
		t.Error(err)
	}

	v, err = b.Get("dir/key")
	if err != nil {
		t.Fatal(err)
	}
	if v != wantV {
		t.Errorf("got v == %q, want %q", v, wantV)
	}

	// Recursive list
	keys, err = b.List("", true)
	if err != nil {
		t.Fatal(err)
	}
	if wantKeys := []string{"dir", "dir/key"}; !reflect.DeepEqual(keys, wantKeys) {
		t.Errorf("got keys == %v, want %v", keys, wantKeys)
	}

	// Non-recursive list
	keys, err = b.List("", false)
	if err != nil {
		t.Fatal(err)
	}
	if wantKeys := []string{"dir"}; !reflect.DeepEqual(keys, wantKeys) {
		t.Errorf("got keys == %v, want %v", keys, wantKeys)
	}

	// Delete
	err = b.Delete("dir/key")
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.Get("dir/key")
	if err != ErrKeyNotExist {
		t.Error(err)
	}
}

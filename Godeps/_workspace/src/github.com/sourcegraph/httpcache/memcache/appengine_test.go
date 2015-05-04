// +build appengine

package memcache

import (
	"bytes"
	"testing"

	"appengine/aetest"
)

func TestAppEngine(t *testing.T) {
	ctx, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()

	cache := New(ctx)

	key := "testKey"
	_, ok := cache.Get(key)

	if ok != false {
		t.Fatal("could retrieve non existing key")
	}

	val := []byte("some bytes")
	cache.Set(key, val)

	retVal, ok := cache.Get(key)
	if ok != true {
		t.Fatal("could not retrieve key i just added")
	}
	if bytes.Equal(retVal, val) != true {
		t.Fatal("retrieved something different from what i put in")
	}

	cache.Delete(key)

	_, ok = cache.Get(key)
	if ok != false {
		t.Fatal("retrieved deleted key")
	}
}

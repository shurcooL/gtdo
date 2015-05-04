package diskcache

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
)

func TestDiskCache(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "httpcache")
	if err != nil {
		t.Fatalf("TempDir,: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cache := New(tempDir)

	key := "testKey"
	_, ok := cache.Get(key)

	if ok != false {
		t.Fatal("Get() without Add()")
	}

	val := []byte("some bytes")
	cache.Set(key, val)

	retVal, ok := cache.Get(key)
	if ok != true {
		t.Fatal("did not retrieve the key i just set")
	}
	if bytes.Equal(retVal, val) != true {
		t.Fatal("retrieved value not equal to the stored one")
	}

	cache.Delete(key)

	_, ok = cache.Get(key)
	if ok != false {
		t.Fatal("Delete() key still present")
	}
}

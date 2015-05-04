package datad

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type datum struct{ value string }

type data map[string]datum

func newData(m map[string]datum) data {
	if m == nil {
		m = make(map[string]datum)
	} else {
		// Ensure all paths begin with '/'.
		for k, d := range m {
			if !strings.HasPrefix(k, "/") {
				delete(m, k)
				m["/"+k] = d
			}
		}
	}
	return data(m)
}

func (m data) HasKey(key string) (bool, error) {
	if !strings.HasPrefix(key, "/") {
		key = "/" + key
	}
	_, present := m[key]
	if !present {
		return false, ErrKeyNotExist
	}
	return true, nil
}

func (m data) Keys(keyPrefix string) ([]string, error) {
	if !strings.HasPrefix(keyPrefix, "/") {
		keyPrefix = "/" + keyPrefix
	}
	if !strings.HasSuffix(keyPrefix, "/") {
		keyPrefix += "/"
	}
	var subkeys []string
	for k, _ := range m {
		if strings.HasPrefix(k, keyPrefix) {
			subkeys = append(subkeys, strings.TrimPrefix(k, keyPrefix))
		}
	}
	return subkeys, nil
}

type noopUpdateProvider struct{ data }

func (p noopUpdateProvider) Update(key string) error {
	if _, err := p.HasKey(key); err != nil {
		return err
	}
	return nil
}

type fakeUpdateProvider struct {
	data

	updateCount int
	mu          sync.Mutex
}

func (p fakeUpdateProvider) Update(key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data[slash(key)] = datum{value: fmt.Sprintf("val%d", p.updateCount)}
	p.updateCount++
	return nil
}

type dataHandler map[string]datum

func (h dataHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d, present := h[slash(r.URL.Path)]
	if !present {
		http.Error(w, ErrKeyNotExist.Error(), http.StatusNotFound)
		return
	}
	w.Write([]byte(d.value))
}

type NoopProvider struct{}

func (_ NoopProvider) HasKey(key string) (bool, error)         { return false, nil }
func (_ NoopProvider) Keys(keyPrefix string) ([]string, error) { return nil, nil }
func (_ NoopProvider) Update(key string) error                 { return nil }

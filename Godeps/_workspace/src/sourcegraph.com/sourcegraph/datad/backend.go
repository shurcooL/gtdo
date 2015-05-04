package datad

import (
	"errors"
	"strings"

	"github.com/coreos/go-etcd/etcd"
)

type Backend interface {
	Get(key string) (string, error)
	List(key string, recursive bool) ([]string, error)

	// ListKeys lists only keys (not directories).
	ListKeys(key string, recursive bool) ([]string, error)

	Set(key, value string) error
	SetDir(key string, ttl uint64) error
	UpdateDir(key string, ttl uint64) error
	Delete(key string) error
}

var ErrKeyNotExist = errors.New("key does not exist")

type EtcdBackend struct {
	keyPrefix string
	etcd      *etcd.Client
}

func NewEtcdBackend(keyPrefix string, c *etcd.Client) Backend {
	keyPrefix = slash(strings.TrimSuffix(keyPrefix, "/"))
	return &EtcdBackend{keyPrefix, c}
}

func (c *EtcdBackend) Get(key string) (string, error) {
	key = c.fullKey(key)
	resp, err := c.etcd.Get(key, false, false)
	if isEtcdKeyNotExist(err) {
		return "", ErrKeyNotExist
	} else if err != nil {
		return "", err
	}
	return resp.Node.Value, nil
}

func (c *EtcdBackend) ListKeys(key string, recursive bool) ([]string, error) {
	return c.listNames(key, recursive, true)
}

func (c *EtcdBackend) List(key string, recursive bool) ([]string, error) {
	return c.listNames(key, recursive, false)
}

func (c *EtcdBackend) listNames(key string, recursive, keysOnly bool) ([]string, error) {
	nodes, err := c.list(key, recursive)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, node := range nodes {
		if !keysOnly || !node.Dir {
			names = append(names, node.Key)
		}
	}

	return names, nil
}

func (c *EtcdBackend) list(key string, recursive bool) ([]*etcd.Node, error) {
	key = c.fullKey(key)
	resp, err := c.etcd.Get(key, true, recursive)
	if isEtcdKeyNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	rmPrefix := strings.TrimSuffix(key, "/") + "/"

	var subnodes []*etcd.Node
	var add func(nodes etcd.Nodes)
	add = func(nodes etcd.Nodes) {
		for _, node := range nodes {
			node.Key = strings.TrimPrefix(node.Key, rmPrefix)
			subnodes = append(subnodes, node)
			if len(node.Nodes) > 0 {
				add(node.Nodes)
			}
		}
	}
	add(resp.Node.Nodes)
	return subnodes, nil
}

func (c *EtcdBackend) Set(key, value string) error {
	key = c.fullKey(key)
	_, err := c.etcd.Set(key, value, 0)
	return err
}

func (c *EtcdBackend) SetDir(key string, ttl uint64) error {
	key = c.fullKey(key)
	_, err := c.etcd.SetDir(key, ttl)
	return err
}

func (c *EtcdBackend) UpdateDir(key string, ttl uint64) error {
	key = c.fullKey(key)
	_, err := c.etcd.UpdateDir(key, ttl)
	return err
}

func (c *EtcdBackend) Delete(key string) error {
	key = c.fullKey(key)
	_, err := c.etcd.Delete(key, false)
	return err
}

func (c *EtcdBackend) fullKey(keyWithoutPrefix string) string {
	return keyPathJoin(c.keyPrefix, keyWithoutPrefix)
}

func isEtcdKeyNotExist(err error) bool { return isEtcdErrorCode(err, 100) }

func isEtcdErrorCode(err error, code int) bool {
	if err, ok := err.(*etcd.EtcdError); ok && err != nil && err.ErrorCode == code {
		return true
	}
	return false
}

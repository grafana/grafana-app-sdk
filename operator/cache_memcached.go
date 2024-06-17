package operator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/grafana/grafana-app-sdk/resource"
	"k8s.io/client-go/tools/cache"
)

var _ cache.Store = &MemcachedStore{}

type MemcachedStore struct {
	client  *memcache.Client
	keyFunc func(any) (string, error)
	kind    resource.Kind
}

func NewMemcachedStore(kind resource.Kind, keyFunc func(any) (string, error), addrs ...string) *MemcachedStore {
	return &MemcachedStore{
		client:  memcache.New(addrs...),
		keyFunc: keyFunc,
		kind:    kind,
	}
}

func (m *MemcachedStore) Add(obj interface{}) error {
	key, err := m.getKey(obj)
	if err != nil {
		return err
	}
	o, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	return m.client.Add(&memcache.Item{
		Key:   key,
		Value: o,
	})
}
func (m *MemcachedStore) Update(obj interface{}) error {
	key, err := m.getKey(obj)
	if err != nil {
		return err
	}
	o, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	return m.client.Replace(&memcache.Item{
		Key:   key,
		Value: o,
	})
}
func (m *MemcachedStore) Delete(obj interface{}) error {
	key, err := m.getKey(obj)
	if err != nil {
		return err
	}
	return m.client.Delete(key)
}
func (m *MemcachedStore) List() []interface{} {
	// TODO: Not supported by memcached
	return nil
}
func (m *MemcachedStore) ListKeys() []string {
	// TODO: Not supported by memcached
	return nil
}
func (m *MemcachedStore) Get(obj interface{}) (item interface{}, exists bool, err error) {
	key, err := m.getKey(obj)
	if err != nil {
		return nil, false, err
	}
	return m.GetByKey(key)
}
func (m *MemcachedStore) GetByKey(key string) (item interface{}, exists bool, err error) {
	fromCache, err := m.client.Get(key)
	if err != nil && !errors.Is(err, memcache.ErrCacheMiss) {
		return nil, false, err
	}
	if fromCache == nil {
		return nil, false, nil
	}
	item, err = m.kind.Read(bytes.NewReader(fromCache.Value), resource.KindEncodingJSON)
	if err != nil {
		return nil, true, err
	}
	return item, true, nil
}
func (m *MemcachedStore) Replace([]interface{}, string) error {
	return nil
}
func (m *MemcachedStore) Resync() error {
	return nil
}

func (m *MemcachedStore) getKey(obj any) (string, error) {
	if m.keyFunc == nil {
		return "", fmt.Errorf("no KeyFunc defined")
	}
	key, err := m.keyFunc(obj)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", m.kind.Plural(), key), nil
}

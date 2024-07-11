package operator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/tools/cache"
)

var _ cache.Store = &MemcachedStore{}

// MemcachedStore implements cache.Store using memcached as the store for objects.
// It should be instantiated with NewMemcachedStore.
type MemcachedStore struct {
	client       *memcache.Client
	keyFunc      func(any) (string, error)
	kind         resource.Kind
	readLatency  *prometheus.HistogramVec
	writeLatency *prometheus.HistogramVec
	keys         sync.Map
	trackKeys    bool
}

// MemcachedStoreConfig is a collection of config values for a MemcachedStore
type MemcachedStoreConfig struct {
	// KeyFunc is the function used to determine the key for an object
	KeyFunc func(any) (string, error)
	// Addrs is a list of addresses (including ports) to connect to
	Addrs []string
	// Metrics is metrics configuration
	Metrics metrics.Config
	// TrackKeys is a flag which, when set to true, allow the MemcachedStore to track what keys exist in the store
	// using an in-memory map. This allows for supporting the ListKeys() and List() functions of cache.Store,
	// which are not natively supported by memcached (and this additionally restricts the keys listed by ListKeys()
	// and List() to ones for objects inserted into this particular instance of MemcachedStore, rather than
	// all keys in the memcached cache, which may be shared across multiple kinds).
	// If this is false, these functions return nil.
	TrackKeys bool
}

// NewMemcachedStore returns a new MemcachedStore for the specified Kind using the provided config.
func NewMemcachedStore(kind resource.Kind, cfg MemcachedStoreConfig) *MemcachedStore {
	keyFunc := cache.DeletionHandlingMetaNamespaceKeyFunc
	if cfg.KeyFunc != nil {
		keyFunc = cfg.KeyFunc
	}
	return &MemcachedStore{
		client:  memcache.New(cfg.Addrs...),
		keyFunc: keyFunc,
		kind:    kind,
		readLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                       cfg.Metrics.Namespace,
			Subsystem:                       "informer",
			Name:                            "cache_read_duration_ms",
			Help:                            "Time (in milliseconds) spent on cache read operations",
			Buckets:                         metrics.LatencyBuckets,
			NativeHistogramBucketFactor:     cfg.Metrics.NativeHistogramBucketFactor,
			NativeHistogramMaxBucketNumber:  cfg.Metrics.NativeHistogramMaxBucketNumber,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"event_type", "kind"}),
		trackKeys: cfg.TrackKeys,
		keys:      sync.Map{},
	}
}

func (m *MemcachedStore) Add(obj interface{}) error {
	key, trackKey, err := m.getKey(obj)
	if err != nil {
		return err
	}
	o, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	err = m.client.Add(&memcache.Item{
		Key:   key,
		Value: o,
	})
	if m.trackKeys && err == nil {
		m.keys.Store(trackKey, struct{}{})
	}
	return err
}
func (m *MemcachedStore) Update(obj interface{}) error {
	key, _, err := m.getKey(obj)
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
	key, trackKey, err := m.getKey(obj)
	if err != nil {
		return err
	}
	err = m.client.Delete(key)
	if err == nil && m.trackKeys {
		m.keys.Delete(trackKey)
	}
	return err
}
func (m *MemcachedStore) List() []interface{} {
	// TODO: do we want to support this even with the trackKeys feature turned on?
	if !m.trackKeys {
		return nil
	}
	items := make([]interface{}, 0)
	m.keys.Range(func(key, value any) bool {
		item, exists, err := m.GetByKey(key.(string))
		if !exists || err != nil {
			return true
		}
		items = append(items, item)
		return true
	})
	return items
}
func (m *MemcachedStore) ListKeys() []string {
	if !m.trackKeys {
		// Not natively supported by memcached, so if the user didn't configure in-mem key tracking, we can't return a list of keys
		return nil
	}
	keys := make([]string, 0)
	m.keys.Range(func(key, value any) bool {
		keys = append(keys, key.(string))
		return true
	})
	return keys
}
func (m *MemcachedStore) Get(obj interface{}) (item interface{}, exists bool, err error) {
	key, trackKey, err := m.getKey(obj)
	if err != nil {
		return nil, false, err
	}
	item, exists, err = m.getByKey(key)
	if m.trackKeys && err == nil && exists {
		m.keys.LoadOrStore(trackKey, struct{}{})
	}
	return item, exists, err
}
func (m *MemcachedStore) GetByKey(key string) (item interface{}, exists bool, err error) {
	item, exists, err = m.getByKey(fmt.Sprintf("%s/%s", m.kind.Plural(), key))
	if m.trackKeys && exists && err == nil {
		m.keys.LoadOrStore(key, struct{}{})
	}
	return item, exists, err
}

func (m *MemcachedStore) getByKey(key string) (item interface{}, exists bool, err error) {
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

func (m *MemcachedStore) getKey(obj any) (prefixedKey string, externalKey string, err error) {
	if m.keyFunc == nil {
		return "", "", fmt.Errorf("no KeyFunc defined")
	}
	externalKey, err = m.keyFunc(obj)
	if err != nil {
		return "", externalKey, err
	}
	return fmt.Sprintf("%s/%s", m.kind.Plural(), externalKey), externalKey, nil
}

package operator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/tools/cache"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

var _ cache.Store = &MemcachedStore{}

const (
	keysCacheKey = "%s-keys"
)

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
	syncTicker   *time.Ticker
}

// MemcachedStoreConfig is a collection of config values for a MemcachedStore
type MemcachedStoreConfig struct {
	// KeyFunc is the function used to determine the key for an object
	KeyFunc func(any) (string, error)
	// Addrs is a list of addresses (including ports) to connect to
	Addrs []string
	// Metrics is metrics configuration
	Metrics metrics.Config
	// KeySyncInterval is the interval at which keys stored in the in-memory map will be pushed to memcached.
	// Set to 0 to disable key tracking. It is advisable to disable this functionality unless you need ListKeys() and/or
	// List() functionality in MemcachedStore (this is required by an informer if you set the CacheResyncInterval).
	// If disabled (0), ListKeys() and List() will return nil.
	// Since a key list cannot be exported from memcached, the keys are tracked in-memory (from Add, Delete, and successful
	// Get operations), and periodically written to a known key in memcached. NewMemcachedStore loads the existing
	// value from the "known keys" key in memcached into the in-memory key tracking, and then will run a process
	// to push this list of keys to memcached every KeySyncInterval. If the data in memcached is cleared,
	// The in-memory list of keys will also be cleared, though this can result in some state synchronization errors,
	// as any Add operations that happen between the time the memcached was cleared and the next sync run will not
	// be known by the key tracker anymore.
	KeySyncInterval time.Duration
	// Timeout is the timeout on memcached connections. Leave 0 to default.
	Timeout time.Duration
	// MaxIdleConns is the max number of idle memcached connections. Leave 0 to default.
	MaxIdleConns int
}

// NewMemcachedStore returns a new MemcachedStore for the specified Kind using the provided config.
func NewMemcachedStore(kind resource.Kind, cfg MemcachedStoreConfig) (*MemcachedStore, error) {
	keyFunc := cache.DeletionHandlingMetaNamespaceKeyFunc
	if cfg.KeyFunc != nil {
		keyFunc = cfg.KeyFunc
	}
	client := memcache.New(cfg.Addrs...)
	client.Timeout = cfg.Timeout
	client.MaxIdleConns = cfg.MaxIdleConns
	store := &MemcachedStore{
		client:  client,
		keyFunc: keyFunc,
		kind:    kind,
		readLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                       cfg.Metrics.Namespace,
			Subsystem:                       "informer",
			Name:                            "cache_read_duration_seconds",
			Help:                            "Time (in seconds) spent on cache read operations",
			Buckets:                         metrics.LatencyBuckets,
			NativeHistogramBucketFactor:     cfg.Metrics.NativeHistogramBucketFactor,
			NativeHistogramMaxBucketNumber:  cfg.Metrics.NativeHistogramMaxBucketNumber,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"kind"}),
		writeLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                       cfg.Metrics.Namespace,
			Subsystem:                       "informer",
			Name:                            "cache_write_duration_seconds",
			Help:                            "Time (in seconds) spent on cache write operations",
			Buckets:                         metrics.LatencyBuckets,
			NativeHistogramBucketFactor:     cfg.Metrics.NativeHistogramBucketFactor,
			NativeHistogramMaxBucketNumber:  cfg.Metrics.NativeHistogramMaxBucketNumber,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"kind"}),
		trackKeys: cfg.KeySyncInterval != 0,
		keys:      sync.Map{},
	}
	if store.trackKeys {
		err := store.setKeysFromCache()
		if err != nil {
			return nil, err
		}
		store.syncTicker = time.NewTicker(cfg.KeySyncInterval)
		go func() {
			for range store.syncTicker.C {
				err := store.syncKeys()
				if err != nil {
					// TODO: better logging?
					logging.DefaultLogger.Error("error syncing memcached keys", "error", err.Error())
				}
			}
		}()
	}
	return store, nil
}

// PrometheusCollectors returns a list of prometheus collectors used by the MemcachedStore
func (m *MemcachedStore) PrometheusCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.readLatency, m.writeLatency,
	}
}

func (m *MemcachedStore) Add(obj any) error {
	key, trackKey, err := m.getKey(obj)
	if err != nil {
		return err
	}
	o, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	start := time.Now()
	err = m.client.Add(&memcache.Item{
		Key:   key,
		Value: o,
	})
	m.writeLatency.WithLabelValues(m.kind.Kind()).Observe(time.Since(start).Seconds())
	if m.trackKeys && err == nil {
		m.keys.Store(trackKey, struct{}{})
	}
	return err
}
func (m *MemcachedStore) Update(obj any) error {
	key, _, err := m.getKey(obj)
	if err != nil {
		return err
	}
	o, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	start := time.Now()
	err = m.client.Replace(&memcache.Item{
		Key:   key,
		Value: o,
	})
	m.writeLatency.WithLabelValues(m.kind.Kind()).Observe(time.Since(start).Seconds())
	return err
}
func (m *MemcachedStore) Delete(obj any) error {
	key, trackKey, err := m.getKey(obj)
	if err != nil {
		return err
	}
	start := time.Now()
	err = m.client.Delete(key)
	m.writeLatency.WithLabelValues(m.kind.Kind()).Observe(time.Since(start).Seconds())
	if err == nil && m.trackKeys {
		m.keys.Delete(trackKey)
	}
	return err
}
func (m *MemcachedStore) List() []any {
	if !m.trackKeys {
		return nil
	}
	items := make([]any, 0)
	keys := m.ListKeys()
	pageSize := 500
	for i := 0; i < len(keys); i += pageSize {
		var fetchKeys []string
		if i+pageSize > len(keys) {
			fetchKeys = keys[i:]
		} else {
			fetchKeys = keys[i : i+pageSize]
		}
		for j := 0; j < len(fetchKeys); j++ {
			fetchKeys[j] = fmt.Sprintf("%s/%s", m.kind.Plural(), fetchKeys[j])
		}
		res, err := m.client.GetMulti(fetchKeys)
		if err != nil {
			// TODO: ???
			return nil
		}
		for _, val := range res {
			items = append(items, val)
		}
	}
	return items
}
func (m *MemcachedStore) ListKeys() []string {
	if !m.trackKeys {
		// Not natively supported by memcached, so if the user didn't configure in-mem key tracking, we can't return a list of keys
		return nil
	}
	keys := make([]string, 0)
	item, err := m.client.Get(fmt.Sprintf(keysCacheKey, m.kind.Plural()))
	if err != nil {
		if errors.Is(err, memcache.ErrCacheMiss) {
			return []string{}
		}
		// error getting from cache, fall back to local in-memory store
		m.keys.Range(func(key, value any) bool {
			keys = append(keys, key.(string))
			return true
		})
	} else {
		err = json.Unmarshal(item.Value, &keys)
		if err != nil {
			return nil
		}
	}
	return keys
}
func (m *MemcachedStore) Get(obj any) (item any, exists bool, err error) {
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
func (m *MemcachedStore) GetByKey(key string) (item any, exists bool, err error) {
	item, exists, err = m.getByKey(fmt.Sprintf("%s/%s", m.kind.Plural(), key))
	if m.trackKeys && exists && err == nil {
		m.keys.LoadOrStore(key, struct{}{})
	}
	return item, exists, err
}

func (m *MemcachedStore) getByKey(key string) (item any, exists bool, err error) {
	start := time.Now()
	fromCache, err := m.client.Get(key)
	m.readLatency.WithLabelValues(m.kind.Kind()).Observe(time.Since(start).Seconds())
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

func (*MemcachedStore) Replace([]any, string) error {
	return nil
}
func (*MemcachedStore) Resync() error {
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

func (m *MemcachedStore) setKeysFromCache() error {
	item, err := m.client.Get(fmt.Sprintf(keysCacheKey, m.kind.Plural()))
	if err != nil {
		if errors.Is(err, memcache.ErrCacheMiss) {
			return nil
		}
		return err
	}
	keys := make([]string, 0)
	err = json.Unmarshal(item.Value, &keys)
	if err != nil {
		return err
	}
	for _, key := range keys {
		m.keys.Store(key, struct{}{})
	}
	return nil
}

func (m *MemcachedStore) syncKeys() error {
	current := make([]string, 0)
	item, err := m.client.Get(fmt.Sprintf(keysCacheKey, m.kind.Plural()))
	if err != nil {
		if !errors.Is(err, memcache.ErrCacheMiss) {
			return err
		}
		// The memcached was cleared at some point, default to having no keys now,
		// because we don't know what keys have been added since the clear
		m.keys = sync.Map{}
		item = &memcache.Item{
			Key:   fmt.Sprintf(keysCacheKey, m.kind.Plural()),
			Value: []byte("[]"),
		}
		err = m.client.Add(item)
		if err != nil {
			return err
		}
	}
	err = json.Unmarshal(item.Value, &current)
	if err != nil {
		return err
	}
	externalKeys := make([]string, 0)
	m.keys.Range(func(key, value any) bool {
		externalKeys = append(externalKeys, key.(string))
		return true
	})
	externalKeysJSON, err := json.Marshal(externalKeys)
	if err != nil {
		return err
	}
	return m.client.Replace(&memcache.Item{
		Key:   fmt.Sprintf(keysCacheKey, m.kind.Plural()),
		Value: externalKeysJSON,
	})
}

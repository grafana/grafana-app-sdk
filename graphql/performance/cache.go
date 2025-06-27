package performance

import (
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResourceCache provides intelligent caching for GraphQL resources
type ResourceCache struct {
	// Cache entries by resource type and key
	entries map[string]*CacheEntry
	mutex   sync.RWMutex

	// Cache configuration
	config CacheConfig

	// Statistics
	hits   int64
	misses int64
}

// CacheEntry represents a cached resource
type CacheEntry struct {
	Data      interface{}
	ExpiresAt time.Time
	AccessAt  time.Time
	HitCount  int64
}

// CacheConfig configures the resource cache
type CacheConfig struct {
	// TTL for cache entries
	TTL time.Duration

	// Maximum number of entries
	MaxEntries int

	// Cleanup interval for expired entries
	CleanupInterval time.Duration

	// Enable/disable caching
	Enabled bool
}

// DefaultCacheConfig returns sensible cache defaults
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		TTL:             5 * time.Minute,
		MaxEntries:      10000,
		CleanupInterval: 10 * time.Minute,
		Enabled:         true,
	}
}

// NewResourceCache creates a new resource cache
func NewResourceCache(config CacheConfig) *ResourceCache {
	if config.TTL == 0 {
		config = DefaultCacheConfig()
	}

	cache := &ResourceCache{
		entries: make(map[string]*CacheEntry),
		config:  config,
	}

	// Start cleanup goroutine if configured
	if config.CleanupInterval > 0 {
		go cache.cleanupLoop()
	}

	return cache
}

// Get retrieves a cached resource
func (rc *ResourceCache) Get(gvr schema.GroupVersionResource, namespace, key string) interface{} {
	if !rc.config.Enabled {
		return nil
	}

	cacheKey := rc.buildKey(gvr, namespace, key)

	rc.mutex.RLock()
	entry, exists := rc.entries[cacheKey]
	rc.mutex.RUnlock()

	if !exists {
		rc.incrementMisses()
		return nil
	}

	// Check expiration
	now := time.Now()
	if now.After(entry.ExpiresAt) {
		// Entry expired, remove it
		rc.mutex.Lock()
		delete(rc.entries, cacheKey)
		rc.mutex.Unlock()

		rc.incrementMisses()
		return nil
	}

	// Update access time and hit count
	rc.mutex.Lock()
	entry.AccessAt = now
	entry.HitCount++
	rc.mutex.Unlock()

	rc.incrementHits()
	return entry.Data
}

// Set stores a resource in the cache
func (rc *ResourceCache) Set(gvr schema.GroupVersionResource, namespace, key string, data interface{}) {
	if !rc.config.Enabled || data == nil {
		return
	}

	cacheKey := rc.buildKey(gvr, namespace, key)
	now := time.Now()

	entry := &CacheEntry{
		Data:      data,
		ExpiresAt: now.Add(rc.config.TTL),
		AccessAt:  now,
		HitCount:  0,
	}

	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	// Check if we need to evict entries
	if len(rc.entries) >= rc.config.MaxEntries {
		rc.evictOldestEntry()
	}

	rc.entries[cacheKey] = entry
}

// Delete removes a resource from the cache
func (rc *ResourceCache) Delete(gvr schema.GroupVersionResource, namespace, key string) {
	if !rc.config.Enabled {
		return
	}

	cacheKey := rc.buildKey(gvr, namespace, key)

	rc.mutex.Lock()
	delete(rc.entries, cacheKey)
	rc.mutex.Unlock()
}

// Clear removes all entries from the cache
func (rc *ResourceCache) Clear() {
	rc.mutex.Lock()
	rc.entries = make(map[string]*CacheEntry)
	rc.hits = 0
	rc.misses = 0
	rc.mutex.Unlock()
}

// Stats returns cache statistics
func (rc *ResourceCache) Stats() CacheStats {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	totalRequests := rc.hits + rc.misses
	var hitRate float64
	if totalRequests > 0 {
		hitRate = float64(rc.hits) / float64(totalRequests)
	}

	return CacheStats{
		Hits:       rc.hits,
		Misses:     rc.misses,
		HitRate:    hitRate,
		EntryCount: len(rc.entries),
		MaxEntries: rc.config.MaxEntries,
		TTL:        rc.config.TTL,
		Enabled:    rc.config.Enabled,
	}
}

// CacheStats contains cache statistics
type CacheStats struct {
	Hits       int64         `json:"hits"`
	Misses     int64         `json:"misses"`
	HitRate    float64       `json:"hit_rate"`
	EntryCount int           `json:"entry_count"`
	MaxEntries int           `json:"max_entries"`
	TTL        time.Duration `json:"ttl"`
	Enabled    bool          `json:"enabled"`
}

// buildKey creates a cache key from GVR, namespace, and resource key
func (rc *ResourceCache) buildKey(gvr schema.GroupVersionResource, namespace, key string) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource, namespace, key)
}

// incrementHits atomically increments hit counter
func (rc *ResourceCache) incrementHits() {
	rc.mutex.Lock()
	rc.hits++
	rc.mutex.Unlock()
}

// incrementMisses atomically increments miss counter
func (rc *ResourceCache) incrementMisses() {
	rc.mutex.Lock()
	rc.misses++
	rc.mutex.Unlock()
}

// evictOldestEntry removes the least recently accessed entry
func (rc *ResourceCache) evictOldestEntry() {
	var oldestKey string
	var oldestTime time.Time

	// Find the oldest entry
	for key, entry := range rc.entries {
		if oldestKey == "" || entry.AccessAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.AccessAt
		}
	}

	// Remove the oldest entry
	if oldestKey != "" {
		delete(rc.entries, oldestKey)
	}
}

// cleanupLoop periodically removes expired entries
func (rc *ResourceCache) cleanupLoop() {
	if rc.config.CleanupInterval <= 0 {
		return
	}

	ticker := time.NewTicker(rc.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rc.cleanupExpired()
	}
}

// cleanupExpired removes all expired entries
func (rc *ResourceCache) cleanupExpired() {
	now := time.Now()
	expiredKeys := make([]string, 0)

	rc.mutex.RLock()
	for key, entry := range rc.entries {
		if now.After(entry.ExpiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
	}
	rc.mutex.RUnlock()

	if len(expiredKeys) > 0 {
		rc.mutex.Lock()
		for _, key := range expiredKeys {
			delete(rc.entries, key)
		}
		rc.mutex.Unlock()
	}
}

// MultiLevelCache provides multiple cache layers for different access patterns
type MultiLevelCache struct {
	// L1: Fast in-memory cache for frequently accessed items
	l1Cache *ResourceCache

	// L2: Larger cache with longer TTL for less frequent items
	l2Cache *ResourceCache

	// Configuration
	l1Config CacheConfig
	l2Config CacheConfig
}

// NewMultiLevelCache creates a multi-level cache system
func NewMultiLevelCache(l1Config, l2Config CacheConfig) *MultiLevelCache {
	return &MultiLevelCache{
		l1Cache:  NewResourceCache(l1Config),
		l2Cache:  NewResourceCache(l2Config),
		l1Config: l1Config,
		l2Config: l2Config,
	}
}

// Get checks L1 cache first, then L2 cache
func (mlc *MultiLevelCache) Get(gvr schema.GroupVersionResource, namespace, key string) interface{} {
	// Check L1 cache first
	if data := mlc.l1Cache.Get(gvr, namespace, key); data != nil {
		return data
	}

	// Check L2 cache
	if data := mlc.l2Cache.Get(gvr, namespace, key); data != nil {
		// Promote to L1 cache
		mlc.l1Cache.Set(gvr, namespace, key, data)
		return data
	}

	return nil
}

// Set stores in both L1 and L2 caches
func (mlc *MultiLevelCache) Set(gvr schema.GroupVersionResource, namespace, key string, data interface{}) {
	mlc.l1Cache.Set(gvr, namespace, key, data)
	mlc.l2Cache.Set(gvr, namespace, key, data)
}

// Delete removes from both caches
func (mlc *MultiLevelCache) Delete(gvr schema.GroupVersionResource, namespace, key string) {
	mlc.l1Cache.Delete(gvr, namespace, key)
	mlc.l2Cache.Delete(gvr, namespace, key)
}

// Clear clears both caches
func (mlc *MultiLevelCache) Clear() {
	mlc.l1Cache.Clear()
	mlc.l2Cache.Clear()
}

// Stats returns combined statistics
func (mlc *MultiLevelCache) Stats() MultiLevelCacheStats {
	return MultiLevelCacheStats{
		L1Stats: mlc.l1Cache.Stats(),
		L2Stats: mlc.l2Cache.Stats(),
	}
}

// MultiLevelCacheStats contains statistics for multi-level cache
type MultiLevelCacheStats struct {
	L1Stats CacheStats `json:"l1_stats"`
	L2Stats CacheStats `json:"l2_stats"`
}

// Example cache configurations for different use cases:
//
// // Fast cache for frequently accessed relationships
// fastConfig := CacheConfig{
//     TTL:             30 * time.Second,
//     MaxEntries:      1000,
//     CleanupInterval: 5 * time.Minute,
//     Enabled:         true,
// }
//
// // Slower cache for less frequent data
// slowConfig := CacheConfig{
//     TTL:             30 * time.Minute,
//     MaxEntries:      10000,
//     CleanupInterval: 15 * time.Minute,
//     Enabled:         true,
// }
//
// cache := NewMultiLevelCache(fastConfig, slowConfig)

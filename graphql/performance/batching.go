package performance

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// BatchLoader provides intelligent batching for relationship queries
type BatchLoader struct {
	// Storage access interface
	storageGetter func(gvr schema.GroupVersionResource) Storage

	// Batching configuration
	maxBatchSize int
	batchTimeout time.Duration

	// Active batches by key
	batches    map[string]*Batch
	batchMutex sync.RWMutex

	// Cache for resolved resources
	cache *ResourceCache
}

// Storage interface for batch operations
type Storage interface {
	// BatchGet retrieves multiple resources by key
	BatchGet(namespace string, keys []string, opts ...interface{}) (map[string]interface{}, error)

	// Get retrieves a single resource (fallback)
	Get(namespace, name string) (interface{}, error)

	// List retrieves resources (for complex queries)
	List(namespace string, opts ...interface{}) (interface{}, error)
}

// Batch represents a collection of pending requests
type Batch struct {
	gvr       schema.GroupVersionResource
	namespace string
	keys      []string
	callbacks []chan<- BatchResult
	timer     *time.Timer
	mutex     sync.Mutex
}

// BatchResult contains the result of a batch operation
type BatchResult struct {
	Data  map[string]interface{}
	Error error
}

// LoaderConfig configures the batch loader
type LoaderConfig struct {
	MaxBatchSize int
	BatchTimeout time.Duration
	CacheConfig  CacheConfig
}

// NewBatchLoader creates a new batch loader
func NewBatchLoader(storageGetter func(gvr schema.GroupVersionResource) Storage, config LoaderConfig) *BatchLoader {
	if config.MaxBatchSize == 0 {
		config.MaxBatchSize = 100 // Default batch size
	}
	if config.BatchTimeout == 0 {
		config.BatchTimeout = 10 * time.Millisecond // Default timeout
	}

	return &BatchLoader{
		storageGetter: storageGetter,
		maxBatchSize:  config.MaxBatchSize,
		batchTimeout:  config.BatchTimeout,
		batches:       make(map[string]*Batch),
		cache:         NewResourceCache(config.CacheConfig),
	}
}

// Load requests a resource, potentially batched with other requests
func (bl *BatchLoader) Load(ctx context.Context, gvr schema.GroupVersionResource, namespace, key string) (interface{}, error) {
	// Check cache first
	if cached := bl.cache.Get(gvr, namespace, key); cached != nil {
		return cached, nil
	}

	// Create batch key
	batchKey := fmt.Sprintf("%s/%s/%s", gvr.String(), namespace, "batch")

	// Get or create batch
	batch := bl.getOrCreateBatch(batchKey, gvr, namespace)

	// Add to batch and wait for result
	resultChan := make(chan BatchResult, 1)

	batch.mutex.Lock()
	batch.keys = append(batch.keys, key)
	batch.callbacks = append(batch.callbacks, resultChan)

	// Trigger batch if we hit max size
	if len(batch.keys) >= bl.maxBatchSize {
		batch.mutex.Unlock()
		go bl.executeBatch(batch)
	} else {
		batch.mutex.Unlock()
	}

	// Wait for result
	select {
	case result := <-resultChan:
		if result.Error != nil {
			return nil, result.Error
		}
		if data, exists := result.Data[key]; exists {
			// Cache the result
			bl.cache.Set(gvr, namespace, key, data)
			return data, nil
		}
		return nil, fmt.Errorf("resource not found: %s", key)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// LoadMany requests multiple resources in a single batch
func (bl *BatchLoader) LoadMany(ctx context.Context, gvr schema.GroupVersionResource, namespace string, keys []string) (map[string]interface{}, error) {
	if len(keys) == 0 {
		return make(map[string]interface{}), nil
	}

	// Check cache for all keys
	results := make(map[string]interface{})
	uncachedKeys := make([]string, 0)

	for _, key := range keys {
		if cached := bl.cache.Get(gvr, namespace, key); cached != nil {
			results[key] = cached
		} else {
			uncachedKeys = append(uncachedKeys, key)
		}
	}

	// If all keys are cached, return immediately
	if len(uncachedKeys) == 0 {
		return results, nil
	}

	// Load uncached keys
	storage := bl.storageGetter(gvr)
	if storage == nil {
		return nil, fmt.Errorf("storage not found for %s", gvr)
	}

	batchResults, err := storage.BatchGet(namespace, uncachedKeys)
	if err != nil {
		return nil, fmt.Errorf("batch get failed: %w", err)
	}

	// Cache and merge results
	for key, data := range batchResults {
		bl.cache.Set(gvr, namespace, key, data)
		results[key] = data
	}

	return results, nil
}

// getOrCreateBatch gets an existing batch or creates a new one
func (bl *BatchLoader) getOrCreateBatch(batchKey string, gvr schema.GroupVersionResource, namespace string) *Batch {
	bl.batchMutex.RLock()
	if batch, exists := bl.batches[batchKey]; exists {
		bl.batchMutex.RUnlock()
		return batch
	}
	bl.batchMutex.RUnlock()

	bl.batchMutex.Lock()
	defer bl.batchMutex.Unlock()

	// Double-check after acquiring write lock
	if batch, exists := bl.batches[batchKey]; exists {
		return batch
	}

	// Create new batch
	batch := &Batch{
		gvr:       gvr,
		namespace: namespace,
		keys:      make([]string, 0),
		callbacks: make([]chan<- BatchResult, 0),
	}

	// Set timeout timer
	batch.timer = time.AfterFunc(bl.batchTimeout, func() {
		bl.executeBatch(batch)
	})

	bl.batches[batchKey] = batch
	return batch
}

// executeBatch executes a batch request
func (bl *BatchLoader) executeBatch(batch *Batch) {
	batch.mutex.Lock()
	defer batch.mutex.Unlock()

	// Stop timer if still running
	if batch.timer != nil {
		batch.timer.Stop()
	}

	// Remove from active batches
	batchKey := fmt.Sprintf("%s/%s/%s", batch.gvr.String(), batch.namespace, "batch")
	bl.batchMutex.Lock()
	delete(bl.batches, batchKey)
	bl.batchMutex.Unlock()

	// Execute batch request
	storage := bl.storageGetter(batch.gvr)
	if storage == nil {
		result := BatchResult{Error: fmt.Errorf("storage not found for %s", batch.gvr)}
		bl.notifyCallbacks(batch.callbacks, result)
		return
	}

	// Deduplicate keys
	uniqueKeys := bl.deduplicateKeys(batch.keys)

	// Perform batch get
	data, err := storage.BatchGet(batch.namespace, uniqueKeys)
	result := BatchResult{Data: data, Error: err}

	// Cache successful results
	if err == nil {
		for key, resource := range data {
			bl.cache.Set(batch.gvr, batch.namespace, key, resource)
		}
	}

	// Notify all callbacks
	bl.notifyCallbacks(batch.callbacks, result)
}

// notifyCallbacks sends results to all waiting callbacks
func (bl *BatchLoader) notifyCallbacks(callbacks []chan<- BatchResult, result BatchResult) {
	for _, callback := range callbacks {
		select {
		case callback <- result:
		default:
			// Channel is full or closed, skip
		}
	}
}

// deduplicateKeys removes duplicate keys from a slice
func (bl *BatchLoader) deduplicateKeys(keys []string) []string {
	seen := make(map[string]bool)
	unique := make([]string, 0, len(keys))

	for _, key := range keys {
		if !seen[key] {
			seen[key] = true
			unique = append(unique, key)
		}
	}

	return unique
}

// Clear clears all batches and cache (useful for testing)
func (bl *BatchLoader) Clear() {
	bl.batchMutex.Lock()
	bl.batches = make(map[string]*Batch)
	bl.batchMutex.Unlock()

	bl.cache.Clear()
}

// Stats returns loader statistics
func (bl *BatchLoader) Stats() LoaderStats {
	bl.batchMutex.RLock()
	activeBatches := len(bl.batches)
	bl.batchMutex.RUnlock()

	return LoaderStats{
		ActiveBatches: activeBatches,
		CacheStats:    bl.cache.Stats(),
	}
}

// LoaderStats contains batch loader statistics
type LoaderStats struct {
	ActiveBatches int
	CacheStats    CacheStats
}

// RelationshipBatchLoader extends BatchLoader for relationship-specific optimizations
type RelationshipBatchLoader struct {
	*BatchLoader

	// Relationship-specific optimizations
	relationshipCache map[string]map[string]interface{} // [sourceKey][relationshipField] -> result
	relationshipMutex sync.RWMutex
}

// NewRelationshipBatchLoader creates a loader optimized for relationships
func NewRelationshipBatchLoader(storageGetter func(gvr schema.GroupVersionResource) Storage, config LoaderConfig) *RelationshipBatchLoader {
	return &RelationshipBatchLoader{
		BatchLoader:       NewBatchLoader(storageGetter, config),
		relationshipCache: make(map[string]map[string]interface{}),
	}
}

// LoadRelationship loads a relationship with additional caching
func (rbl *RelationshipBatchLoader) LoadRelationship(ctx context.Context, sourceKey, relationshipField string, gvr schema.GroupVersionResource, namespace, targetKey string) (interface{}, error) {
	// Check relationship cache first
	cacheKey := fmt.Sprintf("%s#%s", sourceKey, relationshipField)
	if cached := rbl.getRelationshipCache(cacheKey); cached != nil {
		return cached, nil
	}

	// Use standard batch loading
	result, err := rbl.Load(ctx, gvr, namespace, targetKey)
	if err != nil {
		return nil, err
	}

	// Cache the relationship result
	rbl.setRelationshipCache(cacheKey, result)

	return result, nil
}

// getRelationshipCache gets cached relationship result
func (rbl *RelationshipBatchLoader) getRelationshipCache(key string) interface{} {
	rbl.relationshipMutex.RLock()
	defer rbl.relationshipMutex.RUnlock()

	if cached, exists := rbl.relationshipCache[key]; exists {
		return cached
	}

	return nil
}

// setRelationshipCache caches relationship result
func (rbl *RelationshipBatchLoader) setRelationshipCache(key string, value interface{}) {
	rbl.relationshipMutex.Lock()
	defer rbl.relationshipMutex.Unlock()

	rbl.relationshipCache[key] = map[string]interface{}{"data": value}
}

// Example usage in relationship resolver:
//
// ```go
// // Instead of individual queries causing N+1 problem:
// for _, item := range playlist.Items {
//     dashboard, err := storage.Get(namespace, item.Value) // N+1 problem!
// }
//
// // Use batch loader:
// loader := NewRelationshipBatchLoader(storageGetter, LoaderConfig{})
// for _, item := range playlist.Items {
//     dashboard, err := loader.LoadRelationship(ctx,
//         playlist.UID, "dashboard", dashboardGVR, namespace, item.Value)
// }
// // All dashboard queries are automatically batched into single request!
// ```

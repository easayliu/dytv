package cache

import (
	"sync"
	"time"
)

// entry holds a cached value with its expiration time.
type entry[V any] struct {
	value     V
	expiresAt time.Time
}

// Cache is a generic, concurrency-safe TTL cache with built-in singleflight.
type Cache[V any] struct {
	mu      sync.RWMutex
	items   map[string]entry[V]
	ttl     time.Duration

	// singleflight: one in-flight loader per key
	sfMu    sync.Mutex
	inflight map[string]*call[V]
}

// call represents an in-flight or completed loader invocation.
type call[V any] struct {
	wg  sync.WaitGroup
	val V
	err error
}

// New creates a Cache with the given TTL for entries.
func New[V any](ttl time.Duration) *Cache[V] {
	return &Cache[V]{
		items:    make(map[string]entry[V]),
		ttl:      ttl,
		inflight: make(map[string]*call[V]),
	}
}

// Get returns the cached value and true if the key exists and has not expired.
func (c *Cache[V]) Get(key string) (V, bool) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expiresAt) {
		var zero V
		return zero, false
	}
	return e.value, true
}

// Set stores a value with the default TTL.
func (c *Cache[V]) Set(key string, value V) {
	c.mu.Lock()
	c.items[key] = entry[V]{value: value, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

// Delete removes a key from the cache.
func (c *Cache[V]) Delete(key string) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// GetOrLoad returns the cached value for key, or calls loader exactly once
// (even under concurrent access for the same key) and caches the result.
// Errors from loader are returned but not cached.
func (c *Cache[V]) GetOrLoad(key string, loader func() (V, error)) (V, error) {
	// Fast path: cache hit
	if v, ok := c.Get(key); ok {
		return v, nil
	}

	// Singleflight: deduplicate concurrent loads for the same key
	c.sfMu.Lock()
	if cl, ok := c.inflight[key]; ok {
		c.sfMu.Unlock()
		cl.wg.Wait()
		return cl.val, cl.err
	}
	cl := &call[V]{}
	cl.wg.Add(1)
	c.inflight[key] = cl
	c.sfMu.Unlock()

	cl.val, cl.err = loader()
	if cl.err == nil {
		c.Set(key, cl.val)
	}
	cl.wg.Done()

	c.sfMu.Lock()
	delete(c.inflight, key)
	c.sfMu.Unlock()

	return cl.val, cl.err
}

// StartCleanup runs a background goroutine that periodically removes expired
// entries. It stops when the stop channel is closed.
func (c *Cache[V]) StartCleanup(interval time.Duration, stop <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				c.deleteExpired()
			}
		}
	}()
}

func (c *Cache[V]) deleteExpired() {
	now := time.Now()
	c.mu.Lock()
	for k, e := range c.items {
		if now.After(e.expiresAt) {
			delete(c.items, k)
		}
	}
	c.mu.Unlock()
}

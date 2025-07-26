package cache

import (
	"sync"
	"time"

	"github.com/gomcpgo/url_fetcher/pkg/types"
)

// Cache provides in-memory caching with TTL support
type Cache struct {
	entries map[string]*types.CacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
}

// NewCache creates a new cache instance
func NewCache(ttl time.Duration) *Cache {
	cache := &Cache{
		entries: make(map[string]*types.CacheEntry),
		ttl:     ttl,
	}

	// Start cleanup goroutine if TTL is set
	if ttl > 0 {
		go cache.cleanupExpired()
	}

	return cache
}

// generateKey creates a cache key from request parameters
func (c *Cache) generateKey(url, engine, format string) string {
	return url + "|" + engine + "|" + format
}

// Get retrieves a cached response if it exists and hasn't expired
func (c *Cache) Get(url, engine, format string) (*types.FetchResponse, bool) {
	if c.ttl == 0 {
		return nil, false
	}

	key := c.generateKey(url, engine, format)

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		return nil, false
	}

	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		c.Delete(url, engine, format)
		return nil, false
	}

	return entry.Response, true
}

// Set stores a response in the cache
func (c *Cache) Set(url, engine, format string, response *types.FetchResponse) {
	if c.ttl == 0 {
		return
	}

	// Don't cache error responses
	if response.StatusCode == 0 || response.StatusCode >= 400 {
		return
	}

	key := c.generateKey(url, engine, format)

	c.mu.Lock()
	c.entries[key] = &types.CacheEntry{
		Response:  response,
		ExpiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// Delete removes an entry from the cache
func (c *Cache) Delete(url, engine, format string) {
	key := c.generateKey(url, engine, format)

	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Clear removes all entries from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	c.entries = make(map[string]*types.CacheEntry)
	c.mu.Unlock()
}

// Size returns the number of entries in the cache
func (c *Cache) Size() int {
	c.mu.RLock()
	size := len(c.entries)
	c.mu.RUnlock()
	return size
}

// cleanupExpired periodically removes expired entries
func (c *Cache) cleanupExpired() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()

		c.mu.Lock()
		for key, entry := range c.entries {
			if now.After(entry.ExpiresAt) {
				delete(c.entries, key)
			}
		}
		c.mu.Unlock()
	}
}

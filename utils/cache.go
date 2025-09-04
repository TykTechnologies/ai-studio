package utils

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// CacheEntry represents a cached item with expiry
type CacheEntry struct {
	Data      interface{}
	ExpiresAt time.Time
}

// MemoryCache is a simple in-memory cache with TTL support
type MemoryCache struct {
	data  map[string]CacheEntry
	mutex sync.RWMutex
}

var (
	// Global cache instance
	globalCache *MemoryCache
	once        sync.Once
)

// GetCache returns the global cache instance (singleton)
func GetCache() *MemoryCache {
	once.Do(func() {
		globalCache = &MemoryCache{
			data: make(map[string]CacheEntry),
		}
		// Start cleanup routine
		go globalCache.cleanup()
	})
	return globalCache
}

// Set stores a value in the cache with the given TTL
func (c *MemoryCache) Set(key string, value interface{}, ttl time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.data[key] = CacheEntry{
		Data:      value,
		ExpiresAt: time.Now().Add(ttl),
	}
}

// Get retrieves a value from the cache
func (c *MemoryCache) Get(key string) (interface{}, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, exists := c.data[key]
	if !exists || time.Now().After(entry.ExpiresAt) {
		return nil, false
	}

	return entry.Data, true
}

// Delete removes a value from the cache
func (c *MemoryCache) Delete(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.data, key)
}

// DeletePattern removes all keys matching a pattern (prefix)
func (c *MemoryCache) DeletePattern(pattern string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for key := range c.data {
		if len(key) >= len(pattern) && key[:len(pattern)] == pattern {
			delete(c.data, key)
		}
	}
}

// Clear removes all entries from the cache
func (c *MemoryCache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.data = make(map[string]CacheEntry)
}

// cleanup runs periodically to remove expired entries
func (c *MemoryCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute) // Cleanup every 5 minutes
	defer ticker.Stop()

	for range ticker.C {
		c.mutex.Lock()
		now := time.Now()
		for key, entry := range c.data {
			if now.After(entry.ExpiresAt) {
				delete(c.data, key)
			}
		}
		c.mutex.Unlock()
	}
}

// CacheUserPermissions caches user permission results with a standardized key format
func CacheUserPermissions(userID uint, method string, data interface{}, ttl time.Duration) {
	cache := GetCache()
	key := fmt.Sprintf("user:%d:%s", userID, method)
	cache.Set(key, data, ttl)
}

// GetCachedUserPermissions retrieves cached user permission results
func GetCachedUserPermissions(userID uint, method string) (interface{}, bool) {
	cache := GetCache()
	key := fmt.Sprintf("user:%d:%s", userID, method)
	return cache.Get(key)
}

// InvalidateUserCache invalidates all cached permissions for a user
func InvalidateUserCache(userID uint) {
	cache := GetCache()
	pattern := fmt.Sprintf("user:%d:", userID)
	cache.DeletePattern(pattern)
}

// InvalidateUserGroupCache invalidates cache for all users affected by group changes
func InvalidateUserGroupCache(groupID uint, userIDs []uint) {
	for _, userID := range userIDs {
		InvalidateUserCache(userID)
	}
}

// SerializeCacheData converts data to JSON for consistent caching
func SerializeCacheData(data interface{}) []byte {
	bytes, _ := json.Marshal(data)
	return bytes
}

// DeserializeCacheData converts JSON back to data
func DeserializeCacheData(bytes []byte, target interface{}) error {
	return json.Unmarshal(bytes, target)
}

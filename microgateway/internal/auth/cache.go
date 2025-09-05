// internal/auth/cache.go
package auth

import (
	"sync"
	"time"
)

// TokenCache provides thread-safe caching for tokens and credentials
type TokenCache struct {
	mu              sync.RWMutex
	tokens          map[string]*CachedToken
	creds           map[string]*CachedCredential
	maxSize         int
	ttl             time.Duration
	cleanupInterval time.Duration
	stopCleanup     chan bool
	stopped         bool
}

// NewTokenCache creates a new token cache
func NewTokenCache(maxSize int, ttl time.Duration) *TokenCache {
	cache := &TokenCache{
		tokens:          make(map[string]*CachedToken),
		creds:           make(map[string]*CachedCredential),
		maxSize:         maxSize,
		ttl:             ttl,
		cleanupInterval: ttl / 2,
		stopCleanup:     make(chan bool, 1),
		stopped:         false,
	}

	// Start cleanup goroutine
	go cache.cleanupLoop()

	return cache
}

// Get retrieves a cached token
func (c *TokenCache) Get(token string) *CachedToken {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, exists := c.tokens[token]
	if !exists {
		return nil
	}

	// Check if expired
	if time.Since(cached.CreatedAt) > c.ttl {
		return nil
	}

	if cached.ExpiresAt != nil && cached.ExpiresAt.Before(time.Now()) {
		return nil
	}

	return cached
}

// Set stores a token in the cache
func (c *TokenCache) Set(token string, cached *CachedToken) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Implement LRU if cache is full
	if len(c.tokens) >= c.maxSize {
		c.evictOldest()
	}

	c.tokens[token] = cached
}

// GetCredential retrieves a cached credential (returns nil for now, to be implemented with proper models)
func (c *TokenCache) GetCredential(secret string) interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, exists := c.creds[secret]
	if !exists {
		return nil
	}

	// Check if expired
	if time.Since(cached.CachedAt) > cached.TTL {
		return nil
	}

	// Return cached credential data
	return cached
}

// SetCredential stores a credential in the cache
func (c *TokenCache) SetCredential(secret string, keyID string, appID uint, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Implement LRU if cache is full
	if len(c.creds) >= c.maxSize/2 { // Use half the size for credentials
		c.evictOldestCredential()
	}

	c.creds[secret] = &CachedCredential{
		KeyID:      keyID,
		AppID:      appID,
		SecretHash: secret, // Note: This should be the hash, not the secret
		CachedAt:   time.Now(),
		TTL:        ttl,
	}
}

// Clear removes all cached items
func (c *TokenCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tokens = make(map[string]*CachedToken)
	c.creds = make(map[string]*CachedCredential)
}

// Size returns the current cache size
func (c *TokenCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.tokens) + len(c.creds)
}

// Close stops the cleanup goroutine and clears the cache
func (c *TokenCache) Close() {
	c.mu.Lock()
	if !c.stopped {
		close(c.stopCleanup)
		c.stopped = true
	}
	c.mu.Unlock()
	
	c.Clear()
}

// cleanupLoop runs periodic cleanup of expired items
func (c *TokenCache) cleanupLoop() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCleanup:
			return
		}
	}
}

// cleanup removes expired items from the cache
func (c *TokenCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Clean up expired tokens
	for token, cached := range c.tokens {
		if time.Since(cached.CreatedAt) > c.ttl ||
			(cached.ExpiresAt != nil && cached.ExpiresAt.Before(now)) {
			delete(c.tokens, token)
		}
	}

	// Clean up expired credentials
	for key, cached := range c.creds {
		if time.Since(cached.CachedAt) > cached.TTL {
			delete(c.creds, key)
		}
	}
}

// evictOldest removes the oldest token from cache (LRU eviction)
func (c *TokenCache) evictOldest() {
	var oldestToken string
	var oldestTime time.Time

	for token, cached := range c.tokens {
		if oldestToken == "" || cached.CreatedAt.Before(oldestTime) {
			oldestToken = token
			oldestTime = cached.CreatedAt
		}
	}

	if oldestToken != "" {
		delete(c.tokens, oldestToken)
	}
}

// evictOldestCredential removes the oldest credential from cache
func (c *TokenCache) evictOldestCredential() {
	var oldestKey string
	var oldestTime time.Time

	for key, cached := range c.creds {
		if oldestKey == "" || cached.CachedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = cached.CachedAt
		}
	}

	if oldestKey != "" {
		delete(c.creds, oldestKey)
	}
}

// Stats returns cache statistics
type CacheStats struct {
	TokenCount      int
	CredentialCount int
	MaxSize         int
	TTL             time.Duration
}

// GetStats returns current cache statistics
func (c *TokenCache) GetStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return CacheStats{
		TokenCount:      len(c.tokens),
		CredentialCount: len(c.creds),
		MaxSize:         c.maxSize,
		TTL:             c.ttl,
	}
}
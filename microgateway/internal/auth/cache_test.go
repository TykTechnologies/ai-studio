// internal/auth/cache_test.go
package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTokenCache_Basic(t *testing.T) {
	cache := NewTokenCache(10, 5*time.Minute)
	defer cache.Close()

	token := &CachedToken{
		Token:     "test-token",
		AppID:     1,
		Scopes:    []string{"read", "write"},
		CreatedAt: time.Now(),
	}

	t.Run("SetAndGet", func(t *testing.T) {
		cache.Set("test-token", token)
		
		retrieved := cache.Get("test-token")
		assert.NotNil(t, retrieved)
		assert.Equal(t, token.Token, retrieved.Token)
		assert.Equal(t, token.AppID, retrieved.AppID)
		assert.Equal(t, token.Scopes, retrieved.Scopes)
	})

	t.Run("GetNonexistent", func(t *testing.T) {
		retrieved := cache.Get("nonexistent-token")
		assert.Nil(t, retrieved)
	})
}

func TestTokenCache_Expiration(t *testing.T) {
	cache := NewTokenCache(10, 100*time.Millisecond) // Very short TTL
	defer cache.Close()

	token := &CachedToken{
		Token:     "test-token",
		AppID:     1,
		Scopes:    []string{"read"},
		CreatedAt: time.Now(),
	}

	t.Run("ExpirationByTTL", func(t *testing.T) {
		cache.Set("test-token", token)
		
		// Should be available immediately
		retrieved := cache.Get("test-token")
		assert.NotNil(t, retrieved)
		
		// Wait for expiration
		time.Sleep(150 * time.Millisecond)
		
		// Should be expired now
		retrieved = cache.Get("test-token")
		assert.Nil(t, retrieved)
	})

	t.Run("ExpirationByTime", func(t *testing.T) {
		expiredTime := time.Now().Add(-1 * time.Hour)
		expiredToken := &CachedToken{
			Token:     "expired-token",
			AppID:     1,
			Scopes:    []string{"read"},
			ExpiresAt: &expiredTime,
			CreatedAt: time.Now(),
		}

		cache.Set("expired-token", expiredToken)
		
		// Should be nil due to expiration time
		retrieved := cache.Get("expired-token")
		assert.Nil(t, retrieved)
	})
}

func TestTokenCache_LRUEviction(t *testing.T) {
	cache := NewTokenCache(2, 5*time.Minute) // Very small cache
	defer cache.Close()

	// Add tokens that exceed capacity
	token1 := &CachedToken{Token: "token1", AppID: 1, Scopes: []string{}, CreatedAt: time.Now()}
	token2 := &CachedToken{Token: "token2", AppID: 2, Scopes: []string{}, CreatedAt: time.Now().Add(1 * time.Second)}
	token3 := &CachedToken{Token: "token3", AppID: 3, Scopes: []string{}, CreatedAt: time.Now().Add(2 * time.Second)}

	cache.Set("token1", token1)
	cache.Set("token2", token2)
	cache.Set("token3", token3) // Should evict token1

	t.Run("OldestEvicted", func(t *testing.T) {
		// token1 should be evicted
		retrieved := cache.Get("token1")
		assert.Nil(t, retrieved)
		
		// token2 and token3 should still exist
		retrieved = cache.Get("token2")
		assert.NotNil(t, retrieved)
		
		retrieved = cache.Get("token3")
		assert.NotNil(t, retrieved)
	})
}

func TestTokenCache_Credentials(t *testing.T) {
	cache := NewTokenCache(10, 5*time.Minute)
	defer cache.Close()

	t.Run("SetAndGetCredential", func(t *testing.T) {
		cache.SetCredential("secret", "key-123", 1, 1*time.Hour)
		
		retrieved := cache.GetCredential("secret")
		assert.NotNil(t, retrieved)
	})

	t.Run("CredentialExpiration", func(t *testing.T) {
		cache.SetCredential("short-secret", "key-456", 2, 50*time.Millisecond)
		
		// Should be available immediately
		retrieved := cache.GetCredential("short-secret")
		assert.NotNil(t, retrieved)
		
		// Wait for expiration
		time.Sleep(100 * time.Millisecond)
		
		// Should be expired now
		retrieved = cache.GetCredential("short-secret")
		assert.Nil(t, retrieved)
	})
}

func TestTokenCache_ClearAndClose(t *testing.T) {
	cache := NewTokenCache(10, 5*time.Minute)

	token := &CachedToken{
		Token:     "test-token",
		AppID:     1,
		Scopes:    []string{"read"},
		CreatedAt: time.Now(),
	}

	t.Run("Clear", func(t *testing.T) {
		cache.Set("test-token", token)
		cache.SetCredential("secret", "key-123", 1, 1*time.Hour)
		
		// Verify items exist
		assert.NotNil(t, cache.Get("test-token"))
		assert.NotNil(t, cache.GetCredential("secret"))
		
		// Clear cache
		cache.Clear()
		
		// Verify items are gone
		assert.Nil(t, cache.Get("test-token"))
		assert.Nil(t, cache.GetCredential("secret"))
	})

	t.Run("Size", func(t *testing.T) {
		cache.Set("token1", token)
		cache.SetCredential("secret1", "key-123", 1, 1*time.Hour)
		
		size := cache.Size()
		assert.Equal(t, 2, size)
	})

	t.Run("Stats", func(t *testing.T) {
		cache.Clear()
		cache.Set("token1", token)
		cache.SetCredential("secret1", "key-123", 1, 1*time.Hour)
		
		stats := cache.GetStats()
		assert.Equal(t, 1, stats.TokenCount)
		assert.Equal(t, 1, stats.CredentialCount)
		assert.Equal(t, 10, stats.MaxSize)
		assert.Equal(t, 5*time.Minute, stats.TTL)
	})

	// Test Close
	cache.Close()
}
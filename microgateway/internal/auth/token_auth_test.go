// internal/auth/token_auth_test.go
package auth

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto-migrate only the models we need for auth tests
	err = db.AutoMigrate(
		&database.APIToken{},
		&database.App{},
		&database.Credential{},
	)
	require.NoError(t, err)

	return db
}

func TestTokenAuthProvider_ValidateToken(t *testing.T) {
	db := setupTestDB(t)
	cache := NewTokenCache(100, 5*time.Minute)
	provider := NewTokenAuthProvider(db, cache)

	// Create test app
	app := database.App{
		Name:     "Test App",
		IsActive: true,
	}
	err := db.Create(&app).Error
	require.NoError(t, err)

	// Generate a token
	token, err := provider.GenerateToken(app.ID, "Test Token", []string{"read", "write"}, 1*time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	t.Run("ValidToken", func(t *testing.T) {
		result, err := provider.ValidateToken(token)
		assert.NoError(t, err)
		assert.True(t, result.Valid)
		assert.Equal(t, app.ID, result.AppID)
		assert.Contains(t, result.Scopes, "read")
		assert.Contains(t, result.Scopes, "write")
	})

	t.Run("CachedToken", func(t *testing.T) {
		// Second call should hit cache
		result, err := provider.ValidateToken(token)
		assert.NoError(t, err)
		assert.True(t, result.Valid)
		assert.Equal(t, app.ID, result.AppID)
	})

	t.Run("InvalidToken", func(t *testing.T) {
		result, err := provider.ValidateToken("invalid-token")
		assert.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Equal(t, "Invalid token", result.Error)
	})

	t.Run("InactiveApp", func(t *testing.T) {
		// Deactivate app
		db.Model(&app).Update("is_active", false)
		
		// Clear cache to force DB lookup
		cache.Clear()
		
		result, err := provider.ValidateToken(token)
		assert.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Equal(t, "App is inactive", result.Error)
	})
}

func TestTokenAuthProvider_GenerateToken(t *testing.T) {
	db := setupTestDB(t)
	cache := NewTokenCache(100, 5*time.Minute)
	provider := NewTokenAuthProvider(db, cache)

	// Create test app
	app := database.App{
		Name:     "Test App",
		IsActive: true,
	}
	err := db.Create(&app).Error
	require.NoError(t, err)

	t.Run("ValidGeneration", func(t *testing.T) {
		token, err := provider.GenerateToken(app.ID, "Test Token", []string{"admin"}, 24*time.Hour)
		assert.NoError(t, err)
		assert.NotEmpty(t, token)
		assert.Len(t, token, 64) // 32 bytes hex encoded

		// Verify token exists in database
		var dbToken database.APIToken
		err = db.Where("token = ?", token).First(&dbToken).Error
		assert.NoError(t, err)
		assert.Equal(t, "Test Token", dbToken.Name)
		assert.Equal(t, app.ID, dbToken.AppID)
		assert.True(t, dbToken.IsActive)
	})

	t.Run("InvalidApp", func(t *testing.T) {
		_, err := provider.GenerateToken(999, "Test Token", []string{}, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "app not found")
	})

	t.Run("ExpiringToken", func(t *testing.T) {
		token, err := provider.GenerateToken(app.ID, "Expiring Token", []string{"read"}, 1*time.Second)
		assert.NoError(t, err)

		// Wait for expiration
		time.Sleep(2 * time.Second)

		result, err := provider.ValidateToken(token)
		assert.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Equal(t, "Token expired", result.Error)
	})
}

func TestTokenAuthProvider_RevokeToken(t *testing.T) {
	db := setupTestDB(t)
	cache := NewTokenCache(100, 5*time.Minute)
	provider := NewTokenAuthProvider(db, cache)

	// Create test app and token
	app := database.App{
		Name:     "Test App",
		IsActive: true,
	}
	err := db.Create(&app).Error
	require.NoError(t, err)

	token, err := provider.GenerateToken(app.ID, "Test Token", []string{"read"}, 1*time.Hour)
	require.NoError(t, err)

	t.Run("ValidRevocation", func(t *testing.T) {
		err := provider.RevokeToken(token)
		assert.NoError(t, err)

		// Verify token is now invalid
		result, err := provider.ValidateToken(token)
		assert.NoError(t, err)
		assert.False(t, result.Valid)
	})

	t.Run("InvalidTokenRevocation", func(t *testing.T) {
		err := provider.RevokeToken("nonexistent-token")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token not found")
	})
}

func TestTokenAuthProvider_GetTokenInfo(t *testing.T) {
	db := setupTestDB(t)
	cache := NewTokenCache(100, 5*time.Minute)
	provider := NewTokenAuthProvider(db, cache)

	// Create test app and token
	app := database.App{
		Name:     "Test App",
		IsActive: true,
	}
	err := db.Create(&app).Error
	require.NoError(t, err)

	token, err := provider.GenerateToken(app.ID, "Test Token", []string{"admin", "read"}, 0)
	require.NoError(t, err)

	t.Run("ValidTokenInfo", func(t *testing.T) {
		info, err := provider.GetTokenInfo(token)
		assert.NoError(t, err)
		assert.Equal(t, "Test Token", info.Name)
		assert.Equal(t, app.ID, info.AppID)
		assert.Contains(t, info.Scopes, "admin")
		assert.Contains(t, info.Scopes, "read")
		assert.True(t, info.IsActive)
		assert.Nil(t, info.ExpiresAt)
	})

	t.Run("InvalidTokenInfo", func(t *testing.T) {
		_, err := provider.GetTokenInfo("invalid-token")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token not found")
	})
}

func TestTokenAuthProvider_ListTokensForApp(t *testing.T) {
	db := setupTestDB(t)
	cache := NewTokenCache(100, 5*time.Minute)
	provider := NewTokenAuthProvider(db, cache)

	// Create test app
	app := database.App{
		Name:     "Test App",
		IsActive: true,
	}
	err := db.Create(&app).Error
	require.NoError(t, err)

	// Create multiple tokens
	_, _ = provider.GenerateToken(app.ID, "Token 1", []string{"read"}, 1*time.Hour)
	_, _ = provider.GenerateToken(app.ID, "Token 2", []string{"write"}, 0)

	t.Run("ListTokens", func(t *testing.T) {
		tokens, err := provider.ListTokensForApp(app.ID)
		assert.NoError(t, err)
		assert.Len(t, tokens, 2)

		// Verify token names
		names := make(map[string]bool)
		for _, token := range tokens {
			names[token.Name] = true
		}
		assert.True(t, names["Token 1"])
		assert.True(t, names["Token 2"])
	})
}

func TestTokenAuthProvider_GetStats(t *testing.T) {
	db := setupTestDB(t)
	cache := NewTokenCache(100, 5*time.Minute)
	provider := NewTokenAuthProvider(db, cache)

	// Create test app and tokens
	app := database.App{
		Name:     "Test App",
		IsActive: true,
	}
	err := db.Create(&app).Error
	require.NoError(t, err)

	// Create tokens
	_, _ = provider.GenerateToken(app.ID, "Token 1", []string{"read"}, 1*time.Hour)
	_, _ = provider.GenerateToken(app.ID, "Token 2", []string{"write"}, 0)

	t.Run("GetStats", func(t *testing.T) {
		stats, err := provider.GetStats()
		assert.NoError(t, err)
		assert.Equal(t, int64(2), stats.TotalTokens)
		assert.Equal(t, int64(2), stats.ActiveTokens)
		assert.Equal(t, int64(0), stats.ExpiredTokens)
		assert.Equal(t, int64(1), stats.AppsWithTokens)
	})
}

func TestTokenAuthProvider_CleanupExpiredTokens(t *testing.T) {
	db := setupTestDB(t)
	cache := NewTokenCache(100, 5*time.Minute)
	provider := NewTokenAuthProvider(db, cache)

	// Create test app
	app := database.App{
		Name:     "Test App",
		IsActive: true,
	}
	err := db.Create(&app).Error
	require.NoError(t, err)

	// Create expired token manually
	expiredTime := time.Now().Add(-1 * time.Hour)
	expiredToken := database.APIToken{
		Token:     "expired-token",
		Name:      "Expired Token",
		AppID:     app.ID,
		IsActive:  true,
		ExpiresAt: &expiredTime,
	}
	err = db.Create(&expiredToken).Error
	require.NoError(t, err)

	// Create active token
	_, _ = provider.GenerateToken(app.ID, "Active Token", []string{"read"}, 0)

	t.Run("CleanupExpired", func(t *testing.T) {
		err := provider.CleanupExpiredTokens()
		assert.NoError(t, err)

		// Verify expired token was removed
		var count int64
		db.Model(&database.APIToken{}).Where("token = ?", "expired-token").Count(&count)
		assert.Equal(t, int64(0), count)

		// Verify active token still exists
		db.Model(&database.APIToken{}).Where("name = ?", "Active Token").Count(&count)
		assert.Equal(t, int64(1), count)
	})
}
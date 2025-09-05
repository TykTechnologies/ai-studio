// internal/services/container_test.go
package services

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestServiceContainer(t *testing.T) (*ServiceContainer, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto-migrate
	err = db.AutoMigrate(
		&database.APIToken{},
		&database.App{},
		&database.LLM{},
		&database.Credential{},
		&database.BudgetUsage{},
		&database.AnalyticsEvent{},
	)
	require.NoError(t, err)

	cfg := &config.Config{
		Cache: config.CacheConfig{
			Enabled: true,
			MaxSize: 100,
			TTL:     5 * time.Minute,
		},
		Security: config.SecurityConfig{
			EncryptionKey: "12345678901234567890123456789012",
		},
		Analytics: config.AnalyticsConfig{
			Enabled:       true,
			BufferSize:    10,
			FlushInterval: 1 * time.Second,
		},
	}

	container, err := NewServiceContainer(db, cfg)
	require.NoError(t, err)

	return container, db
}

func TestServiceContainer_Creation(t *testing.T) {
	t.Run("ValidCreation", func(t *testing.T) {
		container, db := setupTestServiceContainer(t)
		defer func() {
			if err := database.Close(db); err != nil {
				t.Logf("Failed to close database: %v", err)
			}
		}()

		assert.NotNil(t, container.DB)
		assert.NotNil(t, container.Repository)
		assert.NotNil(t, container.GatewayService)
		assert.NotNil(t, container.BudgetService)
		assert.NotNil(t, container.AnalyticsService)
		assert.NotNil(t, container.Management)
		assert.NotNil(t, container.Token)
		assert.NotNil(t, container.AuthProvider)
		assert.NotNil(t, container.Cache)
		assert.NotNil(t, container.Crypto)
	})

	t.Run("Health", func(t *testing.T) {
		container, db := setupTestServiceContainer(t)
		defer func() {
			if err := database.Close(db); err != nil {
				t.Logf("Failed to close database: %v", err)
			}
		}()

		err := container.Health()
		assert.NoError(t, err)
	})
}

func TestServiceContainer_BackgroundTasks(t *testing.T) {
	container, db := setupTestServiceContainer(t)
	defer func() { _ = database.Close(db) }()

	t.Run("StartStopBackgroundTasks", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Start background tasks
		container.StartBackgroundTasks(ctx)

		// Let them run for a short while
		time.Sleep(100 * time.Millisecond)

		// Stop background tasks
		container.StopBackgroundTasks()

		// Should complete without hanging
	})
}

func TestServiceContainer_GetStats(t *testing.T) {
	container, db := setupTestServiceContainer(t)
	defer func() { _ = database.Close(db) }()

	t.Run("GetStats", func(t *testing.T) {
		stats := container.GetStats()
		assert.NotNil(t, stats)
		assert.Contains(t, stats, "cache")
		
		// Cache stats should be present (interface{} type for now)
		if cacheStats, ok := stats["cache"]; ok {
			assert.NotNil(t, cacheStats)
		}
	})
}

func TestServiceContainer_Cleanup(t *testing.T) {
	container, db := setupTestServiceContainer(t)
	defer func() { _ = database.Close(db) }()

	t.Run("Cleanup", func(t *testing.T) {
		// Should not panic or error
		assert.NotPanics(t, func() {
			container.Cleanup()
		})
	})
}

// Integration test for the service container
func TestServiceContainer_Integration(t *testing.T) {
	container, db := setupTestServiceContainer(t)
	defer func() { 
		container.Cleanup()
		if err := database.Close(db); err != nil {
			t.Logf("Failed to close database: %v", err)
		} 
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	t.Run("FullLifecycle", func(t *testing.T) {
		// Start background tasks
		container.StartBackgroundTasks(ctx)

		// Check health
		err := container.Health()
		assert.NoError(t, err)

		// Get stats
		stats := container.GetStats()
		assert.NotNil(t, stats)

		// Stop background tasks
		container.StopBackgroundTasks()

		// Cleanup
		container.Cleanup()
	})
}
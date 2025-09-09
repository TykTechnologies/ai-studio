// tests/integration/server_test.go
package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/server"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupIntegrationTest(t *testing.T) (*server.Server, *services.ServiceContainer) {
	// Create in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Run migrations
	err = database.Migrate(db)
	require.NoError(t, err)

	// Create test configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:        8080,
			Host:        "0.0.0.0",
			ReadTimeout: 30 * time.Second,
		},
		Database: config.DatabaseConfig{
			Type: "sqlite",
		},
		Cache: config.CacheConfig{
			Enabled: true,
			MaxSize: 100,
			TTL:     5 * time.Minute,
		},
		Security: config.SecurityConfig{
			EncryptionKey: "12345678901234567890123456789012",
			JWTSecret:     "test-secret-key",
		},
		Analytics: config.AnalyticsConfig{
			Enabled:       true,
			BufferSize:    10,
			FlushInterval: 1 * time.Second,
		},
		Observability: config.ObservabilityConfig{
			LogLevel:  "info",
			LogFormat: "json",
		},
	}

	// Create service container
	serviceContainer, err := services.NewServiceContainer(db, cfg)
	require.NoError(t, err)

	// Create server
	srv, err := server.New(cfg, serviceContainer, "test", "test-hash", "test-time")
	require.NoError(t, err)

	return srv, serviceContainer
}

func TestServer_HealthEndpoints(t *testing.T) {
	srv, _ := setupIntegrationTest(t)
	router := srv.GetRouter()

	t.Run("HealthCheck", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "microgateway")
	})

	t.Run("ReadinessCheck", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ready", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "ready")
	})

	t.Run("RootEndpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "microgateway")
		assert.Contains(t, w.Body.String(), "running")
	})
}

func TestServer_ProtectedEndpoints(t *testing.T) {
	srv, _ := setupIntegrationTest(t)
	router := srv.GetRouter()

	t.Run("UnauthorizedAccess", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/llms", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Unauthorized")
	})

	t.Run("InvalidToken", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/llms", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// TestServer_CORSHeaders removed - CORS middleware was intentionally removed during simplification

// TestServer_RequestLogging removed - RequestID functionality was simplified/removed during simplification
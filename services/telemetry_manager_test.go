package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTelemetryManagerTest(t *testing.T) (*gorm.DB, *TelemetryManager) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	tm := NewTelemetryManager(db, true, "1.0.0-test")
	return db, tm
}

func TestNewTelemetryManager(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	t.Run("Create enabled telemetry manager", func(t *testing.T) {
		tm := NewTelemetryManager(db, true, "1.0.0")
		assert.NotNil(t, tm)
		assert.True(t, tm.enabled)
		assert.Equal(t, "1.0.0", tm.version)
		assert.NotNil(t, tm.db)
		assert.NotNil(t, tm.telemetryService)
		assert.NotNil(t, tm.ctx)
		assert.NotNil(t, tm.cancel)
	})

	t.Run("Create disabled telemetry manager", func(t *testing.T) {
		tm := NewTelemetryManager(db, false, "2.0.0")
		assert.NotNil(t, tm)
		assert.False(t, tm.enabled)
		assert.Equal(t, "2.0.0", tm.version)
	})
}

func TestTelemetryManager_IsEnabled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	t.Run("Enabled telemetry manager", func(t *testing.T) {
		tm := NewTelemetryManager(db, true, "1.0.0")
		assert.True(t, tm.IsEnabled())
	})

	t.Run("Disabled telemetry manager", func(t *testing.T) {
		tm := NewTelemetryManager(db, false, "1.0.0")
		assert.False(t, tm.IsEnabled())
	})
}

func TestTelemetryManager_Stop(t *testing.T) {
	_, tm := setupTelemetryManagerTest(t)

	t.Run("Stop telemetry manager", func(t *testing.T) {
		// Should not panic
		tm.Stop()

		// Context should be cancelled
		select {
		case <-tm.ctx.Done():
			// Expected - context is cancelled
		case <-time.After(100 * time.Millisecond):
			t.Error("Context should be cancelled after Stop()")
		}
	})

	t.Run("Stop with nil cancel function", func(t *testing.T) {
		tm := &TelemetryManager{
			cancel: nil,
		}
		// Should not panic
		tm.Stop()
	})
}

func TestTelemetryManager_GenerateInstanceID(t *testing.T) {
	_, tm := setupTelemetryManagerTest(t)

	t.Run("Generate instance ID", func(t *testing.T) {
		id1 := tm.generateInstanceID()
		assert.NotEmpty(t, id1)
		assert.Len(t, id1, 16) // Should be 16 characters (first 16 of hash)

		// Generate again - should be same on same day
		id2 := tm.generateInstanceID()
		assert.Equal(t, id1, id2, "Instance ID should be consistent within the same day")
	})
}

func TestTelemetryManager_CollectAndSend(t *testing.T) {
	t.Run("CollectAndSend with disabled telemetry", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		assert.NoError(t, err)
		err = models.InitModels(db)
		assert.NoError(t, err)

		tm := NewTelemetryManager(db, false, "1.0.0")

		// Should return immediately without sending
		tm.collectAndSend()
		// No assertions needed - just verify it doesn't panic
	})

	t.Run("CollectAndSend with mock HTTP server", func(t *testing.T) {
		db, tm := setupTelemetryManagerTest(t)

		// Create test data
		user := &models.User{Email: "telemetry@test.com", Name: "Telemetry User", Password: "pass"}
		db.Create(user)

		llm := &models.LLM{
			Name:             "test-llm",
			Vendor:           models.OPENAI,
			ShortDescription: "Test LLM",
			Active:           true,
		}
		db.Create(llm)

		// Create a mock HTTP server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Contains(t, r.Header.Get("User-Agent"), "Tyk-AI-Portal")

			var payload TelemetryPayload
			err := json.NewDecoder(r.Body).Decode(&payload)
			assert.NoError(t, err)

			assert.NotEmpty(t, payload.InstanceID)
			assert.Equal(t, "1.0.0-test", payload.Version)
			assert.NotNil(t, payload.LLMStats)
			assert.NotNil(t, payload.AppStats)
			assert.NotNil(t, payload.UserStats)
			assert.NotNil(t, payload.ChatStats)

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Note: Can't easily test collectAndSend with mock server without modifying production code
		// So we'll just test that the method doesn't panic with real data
		// In a real scenario, you'd use dependency injection for the HTTP client

		// Just verify the method doesn't panic
		// (It will fail to send to the real telemetry endpoint, but that's expected in tests)
		tm.collectAndSend()
	})
}

func TestTelemetryManager_SendTelemetry(t *testing.T) {
	t.Run("Send telemetry - context cancellation", func(t *testing.T) {
		_, tm := setupTelemetryManagerTest(t)

		// Cancel context before sending
		tm.cancel()

		payload := TelemetryPayload{
			Timestamp:  time.Now(),
			InstanceID: "test-instance",
			Version:    "1.0.0",
			LLMStats:   map[string]interface{}{"count": 5},
			AppStats:   map[string]interface{}{"count": 10},
			UserStats:  map[string]interface{}{"count": 3},
			ChatStats:  map[string]interface{}{"count": 20},
		}

		// Should fail due to cancelled context
		err := tm.sendTelemetry(payload)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "telemetry")
	})

	t.Run("Send telemetry with invalid payload", func(t *testing.T) {
		_, tm := setupTelemetryManagerTest(t)

		// Create a payload with un-marshalable data
		payload := TelemetryPayload{
			Timestamp:  time.Now(),
			InstanceID: "test",
			Version:    "1.0.0",
			LLMStats:   map[string]interface{}{"invalid": make(chan int)}, // channels can't be marshaled
		}

		err := tm.sendTelemetry(payload)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to marshal telemetry payload")
	})
}

func TestTelemetryManager_Start(t *testing.T) {
	t.Run("Start with disabled telemetry", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		assert.NoError(t, err)
		err = models.InitModels(db)
		assert.NoError(t, err)

		tm := NewTelemetryManager(db, false, "1.0.0")

		// Should return immediately without starting goroutines
		tm.Start()

		// Give it a moment to ensure no goroutines were started
		time.Sleep(50 * time.Millisecond)

		// No assertions needed - just verify it doesn't panic
	})

	t.Run("Start and Stop with enabled telemetry", func(t *testing.T) {
		_, tm := setupTelemetryManagerTest(t)

		// Start telemetry (will start goroutines)
		// Note: This will fail to send data to real endpoint, but that's expected in tests
		tm.Start()

		// Give goroutines time to start
		time.Sleep(50 * time.Millisecond)

		// Stop telemetry
		tm.Stop()

		// Verify context is cancelled
		select {
		case <-tm.ctx.Done():
			// Expected
		case <-time.After(100 * time.Millisecond):
			t.Error("Context should be cancelled after Stop()")
		}
	})
}

func TestTelemetryPayload_Marshaling(t *testing.T) {
	t.Run("Marshal and unmarshal telemetry payload", func(t *testing.T) {
		original := TelemetryPayload{
			Timestamp:  time.Now().Truncate(time.Second), // Truncate for comparison
			InstanceID: "test-instance-123",
			Version:    "1.2.3",
			LLMStats:   map[string]interface{}{"total": 10, "active": 8},
			AppStats:   map[string]interface{}{"total": 20, "active": 15},
			UserStats:  map[string]interface{}{"total": 100, "admins": 5},
			ChatStats:  map[string]interface{}{"total": 500, "messages": 10000},
		}

		// Marshal to JSON
		jsonData, err := json.Marshal(original)
		assert.NoError(t, err)
		assert.NotEmpty(t, jsonData)

		// Unmarshal back
		var decoded TelemetryPayload
		err = json.Unmarshal(jsonData, &decoded)
		assert.NoError(t, err)

		// Verify fields
		assert.Equal(t, original.InstanceID, decoded.InstanceID)
		assert.Equal(t, original.Version, decoded.Version)
		// JSON unmarshaling converts numbers to float64
		assert.Equal(t, float64(10), decoded.LLMStats["total"])
		assert.Equal(t, float64(20), decoded.AppStats["total"])
	})
}

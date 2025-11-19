package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPluginKVTest(t *testing.T) (*PluginKVService, *gorm.DB, uint) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	// Create a test plugin
	plugin := &models.Plugin{
		Name:        "test-plugin",
		Description: "Test Plugin",
		Command:     "/usr/bin/test",
		HookType:    "pre_request",
		IsActive:    true,
	}
	err = db.Create(plugin).Error
	assert.NoError(t, err)

	kvService := NewPluginKVService(db)
	return kvService, db, plugin.ID
}

func TestWriteKV(t *testing.T) {
	kvService, _, pluginID := setupPluginKVTest(t)

	t.Run("Write new KV entry", func(t *testing.T) {
		created, err := kvService.WriteKV(pluginID, "test-key", []byte("test-value"), nil)
		assert.NoError(t, err)
		assert.True(t, created, "Should create new entry")
	})

	t.Run("Update existing KV entry", func(t *testing.T) {
		// Write initial value
		created, err := kvService.WriteKV(pluginID, "update-key", []byte("initial"), nil)
		assert.NoError(t, err)
		assert.True(t, created)

		// Update the value
		created, err = kvService.WriteKV(pluginID, "update-key", []byte("updated"), nil)
		assert.NoError(t, err)
		assert.False(t, created, "Should update existing entry")

		// Verify updated value
		value, err := kvService.ReadKV(pluginID, "update-key")
		assert.NoError(t, err)
		assert.Equal(t, []byte("updated"), value)
	})

	t.Run("Write with expiration", func(t *testing.T) {
		future := time.Now().Add(1 * time.Hour)
		created, err := kvService.WriteKV(pluginID, "expire-key", []byte("expires"), &future)
		assert.NoError(t, err)
		assert.True(t, created)
	})

	t.Run("Validation: zero plugin ID rejected", func(t *testing.T) {
		created, err := kvService.WriteKV(0, "key", []byte("value"), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be zero")
		assert.False(t, created)
	})

	t.Run("Validation: empty key rejected", func(t *testing.T) {
		created, err := kvService.WriteKV(pluginID, "", []byte("value"), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be empty")
		assert.False(t, created)
	})

	t.Run("Validation: key too long rejected", func(t *testing.T) {
		longKey := string(make([]byte, MaxKVKeyLength+1))
		created, err := kvService.WriteKV(pluginID, longKey, []byte("value"), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum")
		assert.False(t, created)
	})

	t.Run("Validation: value too large rejected", func(t *testing.T) {
		largeValue := make([]byte, MaxKVValueSize+1)
		created, err := kvService.WriteKV(pluginID, "key", largeValue, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum")
		assert.False(t, created)
	})

	t.Run("Validation: non-existent plugin rejected", func(t *testing.T) {
		created, err := kvService.WriteKV(99999, "key", []byte("value"), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.False(t, created)
	})
}

func TestReadKV(t *testing.T) {
	kvService, _, pluginID := setupPluginKVTest(t)

	t.Run("Read existing key", func(t *testing.T) {
		// Write a value
		_, err := kvService.WriteKV(pluginID, "read-key", []byte("read-value"), nil)
		assert.NoError(t, err)

		// Read it back
		value, err := kvService.ReadKV(pluginID, "read-key")
		assert.NoError(t, err)
		assert.Equal(t, []byte("read-value"), value)
	})

	t.Run("Read non-existent key", func(t *testing.T) {
		value, err := kvService.ReadKV(pluginID, "non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.Nil(t, value)
	})

	t.Run("Read expired key", func(t *testing.T) {
		// Write with past expiration
		past := time.Now().Add(-1 * time.Hour)
		_, err := kvService.WriteKV(pluginID, "expired-key", []byte("expired"), &past)
		assert.NoError(t, err)

		// Reading expired key should fail
		value, err := kvService.ReadKV(pluginID, "expired-key")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.Nil(t, value)
	})

	t.Run("Validation: zero plugin ID rejected", func(t *testing.T) {
		value, err := kvService.ReadKV(0, "key")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be zero")
		assert.Nil(t, value)
	})

	t.Run("Validation: empty key rejected", func(t *testing.T) {
		value, err := kvService.ReadKV(pluginID, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be empty")
		assert.Nil(t, value)
	})
}

func TestDeleteKV(t *testing.T) {
	kvService, _, pluginID := setupPluginKVTest(t)

	t.Run("Delete existing key", func(t *testing.T) {
		// Write a value
		_, err := kvService.WriteKV(pluginID, "delete-key", []byte("to-delete"), nil)
		assert.NoError(t, err)

		// Delete it
		deleted, err := kvService.DeleteKV(pluginID, "delete-key")
		assert.NoError(t, err)
		assert.True(t, deleted, "Should return true when key existed")

		// Verify it's gone
		value, err := kvService.ReadKV(pluginID, "delete-key")
		assert.Error(t, err)
		assert.Nil(t, value)
	})

	t.Run("Delete non-existent key", func(t *testing.T) {
		deleted, err := kvService.DeleteKV(pluginID, "never-existed")
		assert.NoError(t, err)
		assert.False(t, deleted, "Should return false when key doesn't exist")
	})

	t.Run("Validation: zero plugin ID rejected", func(t *testing.T) {
		deleted, err := kvService.DeleteKV(0, "key")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be zero")
		assert.False(t, deleted)
	})

	t.Run("Validation: empty key rejected", func(t *testing.T) {
		deleted, err := kvService.DeleteKV(pluginID, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be empty")
		assert.False(t, deleted)
	})
}

func TestClearAllPluginData(t *testing.T) {
	kvService, _, pluginID := setupPluginKVTest(t)

	t.Run("Clear all data for plugin", func(t *testing.T) {
		// Write multiple entries
		_, err := kvService.WriteKV(pluginID, "key1", []byte("value1"), nil)
		assert.NoError(t, err)
		_, err = kvService.WriteKV(pluginID, "key2", []byte("value2"), nil)
		assert.NoError(t, err)
		_, err = kvService.WriteKV(pluginID, "key3", []byte("value3"), nil)
		assert.NoError(t, err)

		// Clear all
		err = kvService.ClearAllPluginData(pluginID)
		assert.NoError(t, err)

		// Verify all are gone
		keys, err := kvService.ListPluginKeys(pluginID)
		assert.NoError(t, err)
		assert.Len(t, keys, 0)
	})

	t.Run("Validation: zero plugin ID rejected", func(t *testing.T) {
		err := kvService.ClearAllPluginData(0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be zero")
	})

	t.Run("Validation: non-existent plugin rejected", func(t *testing.T) {
		err := kvService.ClearAllPluginData(99999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestCountPluginData(t *testing.T) {
	kvService, _, pluginID := setupPluginKVTest(t)

	t.Run("Count returns correct number", func(t *testing.T) {
		// Initially should be 0
		count, err := kvService.CountPluginData(pluginID)
		assert.NoError(t, err)
		assert.Equal(t, int64(0), count)

		// Add some entries
		_, err = kvService.WriteKV(pluginID, "count1", []byte("v1"), nil)
		assert.NoError(t, err)
		_, err = kvService.WriteKV(pluginID, "count2", []byte("v2"), nil)
		assert.NoError(t, err)

		count, err = kvService.CountPluginData(pluginID)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), count)
	})

	t.Run("Validation: zero plugin ID rejected", func(t *testing.T) {
		count, err := kvService.CountPluginData(0)
		assert.Error(t, err)
		assert.Equal(t, int64(0), count)
	})
}

func TestListPluginKeys(t *testing.T) {
	kvService, _, pluginID := setupPluginKVTest(t)

	t.Run("List returns all keys", func(t *testing.T) {
		// Write multiple entries
		_, err := kvService.WriteKV(pluginID, "list-key-1", []byte("v1"), nil)
		assert.NoError(t, err)
		_, err = kvService.WriteKV(pluginID, "list-key-2", []byte("v2"), nil)
		assert.NoError(t, err)
		_, err = kvService.WriteKV(pluginID, "list-key-3", []byte("v3"), nil)
		assert.NoError(t, err)

		keys, err := kvService.ListPluginKeys(pluginID)
		assert.NoError(t, err)
		assert.Len(t, keys, 3)
		assert.Contains(t, keys, "list-key-1")
		assert.Contains(t, keys, "list-key-2")
		assert.Contains(t, keys, "list-key-3")
	})

	t.Run("List returns empty for plugin with no data", func(t *testing.T) {
		// Create new plugin with no data
		db := kvService.db
		newPlugin := &models.Plugin{
			Name:     "empty-plugin",
			Command:  "/bin/test",
			HookType: "test",
			IsActive: true,
		}
		err := db.Create(newPlugin).Error
		assert.NoError(t, err)

		keys, err := kvService.ListPluginKeys(newPlugin.ID)
		assert.NoError(t, err)
		assert.Len(t, keys, 0)
	})

	t.Run("Validation: zero plugin ID rejected", func(t *testing.T) {
		keys, err := kvService.ListPluginKeys(0)
		assert.Error(t, err)
		assert.Nil(t, keys)
	})
}

func TestCleanupExpiredData(t *testing.T) {
	kvService, _, pluginID := setupPluginKVTest(t)

	t.Run("Cleanup deletes expired entries", func(t *testing.T) {
		// Write entry that expired 2 hours ago
		expired := time.Now().Add(-2 * time.Hour)
		_, err := kvService.WriteKV(pluginID, "old-key", []byte("old"), &expired)
		assert.NoError(t, err)

		// Write entry that's not expired
		future := time.Now().Add(1 * time.Hour)
		_, err = kvService.WriteKV(pluginID, "future-key", []byte("future"), &future)
		assert.NoError(t, err)

		// Write entry with no expiration
		_, err = kvService.WriteKV(pluginID, "permanent-key", []byte("permanent"), nil)
		assert.NoError(t, err)

		// Cleanup with 30-minute grace period
		deleted, err := kvService.CleanupExpiredData(30 * time.Minute)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), deleted, "Should delete 1 expired entry")

		// Verify only expired entry was deleted
		_, err = kvService.ReadKV(pluginID, "old-key")
		assert.Error(t, err, "Old key should be gone")

		value, err := kvService.ReadKV(pluginID, "future-key")
		assert.NoError(t, err)
		assert.Equal(t, []byte("future"), value)

		value, err = kvService.ReadKV(pluginID, "permanent-key")
		assert.NoError(t, err)
		assert.Equal(t, []byte("permanent"), value)
	})

	t.Run("Cleanup with no expired entries", func(t *testing.T) {
		kvService, _, pluginID := setupPluginKVTest(t)

		// Write entry that's not expired
		future := time.Now().Add(1 * time.Hour)
		_, err := kvService.WriteKV(pluginID, "fresh-key", []byte("fresh"), &future)
		assert.NoError(t, err)

		deleted, err := kvService.CleanupExpiredData(1 * time.Minute)
		assert.NoError(t, err)
		assert.Equal(t, int64(0), deleted)
	})
}

// Note: StartCleanupRoutine is not tested as it starts a background goroutine
// It would require complex coordination and is better tested in integration tests

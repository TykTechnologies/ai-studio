package models

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupKVTestDB(t *testing.T) *GormKVStore {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	store := &GormKVStore{}
	require.NoError(t, store.Init(db))

	return store
}

func TestNewGormKVStore(t *testing.T) {
	// Create a new database connection
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Test successful creation
	store := NewGormKVStore(db)
	assert.NotNil(t, store)
	assert.IsType(t, &GormKVStore{}, store)
}

func TestGormKVStore_Init(t *testing.T) {
	// Get a database connection from the setup function
	// This already tests successful initialization
	store := setupKVTestDB(t)
	assert.NotNil(t, store.DB)

	// Test initialization with invalid config
	err := store.Init("invalid-config")
	assert.Error(t, err)
	assert.Equal(t, "invalid config", err.Error())
}

// Custom type that will cause a marshalling error
type UnmarshalableType struct{}

func (u UnmarshalableType) MarshalJSON() ([]byte, error) {
	return nil, errors.New("marshalling error")
}

func TestGormKVStore_SetKey(t *testing.T) {
	store := setupKVTestDB(t)

	testCases := []struct {
		name        string
		key         string
		orgID       string
		value       interface{}
		expectError bool
	}{
		{"Set JSON object string", "key1", "org1", `{"test":"json"}`, false},
		{"Set JSON array string", "key2", "org1", `["value1","value2"]`, false},
		{"Set invalid JSON string", "key3", "org1", `invalid json`, true},
		{"Set struct value", "key4", "org1", struct{ Name string }{"Test"}, false},
		{"Set unmarshalable value", "key5", "org1", UnmarshalableType{}, true},
		{"Update existing key", "key1", "org1", `{"updated":"value"}`, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := store.SetKey(tc.key, tc.orgID, tc.value)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGormKVStore_GetKey(t *testing.T) {
	store := setupKVTestDB(t)

	// Set up test data
	require.NoError(t, store.SetKey("key1", "org1", `{"test":"json"}`))
	require.NoError(t, store.SetKey("key2", "org1", `["value1","value2"]`))
	require.NoError(t, store.SetKey("key3", "org1", 42))
	require.NoError(t, store.SetKey("key4", "org1", struct{ Name string }{"Test"}))

	testCases := []struct {
		name        string
		key         string
		orgID       string
		expectValue interface{}
		expectError bool
	}{
		{"Get JSON object", "key1", "org1", map[string]interface{}{"test": "json"}, false},
		{"Get JSON array", "key2", "org1", []interface{}{"value1", "value2"}, false},
		{"Get number value", "key3", "org1", float64(42), false},
		{"Get struct value", "key4", "org1", map[string]interface{}{"Name": "Test"}, false},
		{"Get non-existent key", "key5", "org1", nil, true},
		{"Get key from wrong org", "key1", "org2", nil, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var result interface{}
			err := store.GetKey(tc.key, tc.orgID, &result)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectValue, result)
			}
		})
	}
}

func TestGormKVStore_GetAll(t *testing.T) {
	store := setupKVTestDB(t)

	// Set up test data
	require.NoError(t, store.SetKey("key1", "org1", `{"test":"json"}`))
	require.NoError(t, store.SetKey("key2", "org1", `["value1","value2"]`))
	require.NoError(t, store.SetKey("key3", "org2", `{"another":"value"}`))

	testCases := []struct {
		name         string
		orgID        string
		expectedKeys int
	}{
		{"Get all for org1", "org1", 2},
		{"Get all for org2", "org2", 1},
		{"Get all for non-existent org", "org3", 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results := store.GetAll(tc.orgID)
			assert.Len(t, results, tc.expectedKeys)
		})
	}
}

func TestGormKVStore_DeleteKey(t *testing.T) {
	store := setupKVTestDB(t)

	// Set up test data
	require.NoError(t, store.SetKey("key1", "org1", `{"test":"json"}`))
	require.NoError(t, store.SetKey("key2", "org1", `["value1","value2"]`))
	require.NoError(t, store.SetKey("key3", "org2", `{"another":"value"}`))

	testCases := []struct {
		name        string
		key         string
		orgID       string
		expectError bool
	}{
		{"Delete existing key", "key1", "org1", false},
		{"Delete non-existent key", "key4", "org1", false},
		{"Delete key from wrong org", "key3", "org1", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := store.DeleteKey(tc.key, tc.orgID)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify key is deleted
			var result interface{}
			err = store.GetKey(tc.key, tc.orgID, &result)
			assert.Error(t, err)
		})
	}

	// Verify other keys were not affected
	var result interface{}
	err := store.GetKey("key2", "org1", &result)
	assert.NoError(t, err)

	err = store.GetKey("key3", "org2", &result)
	assert.NoError(t, err)
}

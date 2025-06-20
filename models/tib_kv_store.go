package models

import (
	"encoding/json"
	"fmt"

	"github.com/TykTechnologies/tyk-identity-broker/tap"
	"gorm.io/gorm"
)

// GormKVStore implements AuthRegisterBackend using GORM
type GormKVStore struct {
	DB *gorm.DB
}

// NewGormKVStore creates a new instance of GormKVStore and initializes it with the given database connection
func NewGormKVStore(db *gorm.DB) tap.AuthRegisterBackend {
	store := &GormKVStore{}

	err := store.Init(db)
	if err != nil {
		return nil
	}

	return store
}

// KVPair represents a key-value pair in the store
type KVPair struct {
	StoreKey   string `gorm:"primary_key"`
	OrgID      string `gorm:"index"`
	StoreValue json.RawMessage
}

// Init initializes the GormKVStore with the given configuration
func (store *GormKVStore) Init(config interface{}) error {
	db, ok := config.(*gorm.DB)
	if !ok {
		return fmt.Errorf("invalid config")
	}

	store.DB = db
	store.DB.AutoMigrate(&KVPair{})

	return nil
}

// SetKey sets a key-value pair in the store for a given organization
func (store *GormKVStore) SetKey(key, orgID string, val interface{}) error {
	var (
		value []byte
		err   error
	)

	switch v := val.(type) {
	case string:
		var raw json.RawMessage

		err = json.Unmarshal([]byte(v), &raw)
		if err != nil {
			return fmt.Errorf("failed to marshal string: %w", err)
		}

		value = raw
	default:
		value, err = json.Marshal(val)
		if err != nil {
			return fmt.Errorf("failed to marshal value: %w", err)
		}
	}

	kvPair := KVPair{StoreKey: key, OrgID: orgID, StoreValue: value}
	if err := store.DB.Save(&kvPair).Error; err != nil {
		return fmt.Errorf("failed to save KVPair: %w", err)
	}

	return nil
}

// GetKey retrieves a value for a given key and organization
func (store *GormKVStore) GetKey(key, orgID string, val interface{}) error {
	var kvPair KVPair

	if err := store.DB.Where("store_key = ? AND org_id = ?", key, orgID).First(&kvPair).Error; err != nil {
		return err
	}

	return json.Unmarshal(kvPair.StoreValue, val)
}

// GetAll retrieves all key-value pairs for a given organization
func (store *GormKVStore) GetAll(orgID string) []interface{} {
	var kvPairs []KVPair
	store.DB.Where("org_id = ?", orgID).Find(&kvPairs)
	var result []interface{}

	for _, kvPair := range kvPairs {
		var val interface{}
		if err := json.Unmarshal(kvPair.StoreValue, &val); err == nil {
			result = append(result, val)
		}
	}

	return result
}

// DeleteKey deletes a key-value pair for a given key and organization
func (store *GormKVStore) DeleteKey(key, orgID string) error {
	return store.DB.Where("store_key = ? AND org_id = ?", key, orgID).Delete(&KVPair{}).Error
}

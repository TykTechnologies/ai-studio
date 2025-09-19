package models

import (
	"time"

	"gorm.io/gorm"
)

// PluginConfigSchema represents cached configuration schemas for plugins
// Cache is keyed by Command to allow sharing schemas between plugins with same command
type PluginConfigSchema struct {
	gorm.Model
	ID          uint      `json:"id" gorm:"primaryKey"`
	Command     string    `json:"command" gorm:"uniqueIndex;not null;size:500"` // Plugin command as cache key
	SchemaJSON  string    `json:"schema_json" gorm:"type:text"`                 // JSON Schema as text
	LastFetched time.Time `json:"last_fetched"`                                 // When schema was last fetched
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

// PluginConfigSchemas is a collection of PluginConfigSchema
type PluginConfigSchemas []PluginConfigSchema

// NewPluginConfigSchema creates a new PluginConfigSchema instance
func NewPluginConfigSchema() *PluginConfigSchema {
	return &PluginConfigSchema{}
}

// Get retrieves a plugin config schema by command
func (pcs *PluginConfigSchema) GetByCommand(db *gorm.DB, command string) error {
	return db.Where("command = ?", command).First(pcs).Error
}

// Create creates a new plugin config schema
func (pcs *PluginConfigSchema) Create(db *gorm.DB) error {
	pcs.LastFetched = time.Now()
	return db.Create(pcs).Error
}

// Update updates an existing plugin config schema
func (pcs *PluginConfigSchema) Update(db *gorm.DB) error {
	pcs.LastFetched = time.Now()
	return db.Save(pcs).Error
}

// Delete soft deletes a plugin config schema
func (pcs *PluginConfigSchema) Delete(db *gorm.DB) error {
	return db.Delete(pcs).Error
}

// Upsert creates or updates a plugin config schema by command
func (pcs *PluginConfigSchema) Upsert(db *gorm.DB, command string, schemaJSON string) error {
	now := time.Now()
	pcs.Command = command
	pcs.SchemaJSON = schemaJSON
	pcs.LastFetched = now

	// Try to find existing record
	existing := &PluginConfigSchema{}
	err := existing.GetByCommand(db, command)

	if err == gorm.ErrRecordNotFound {
		// Create new record
		pcs.CreatedAt = now
		pcs.UpdatedAt = now
		return db.Create(pcs).Error
	} else if err != nil {
		return err
	}

	// Update existing record
	existing.SchemaJSON = schemaJSON
	existing.LastFetched = now
	existing.UpdatedAt = now
	*pcs = *existing
	return db.Save(pcs).Error
}

// IsStale checks if the cached schema is older than the specified duration
func (pcs *PluginConfigSchema) IsStale(maxAge time.Duration) bool {
	return time.Since(pcs.LastFetched) > maxAge
}

// ListAll returns all plugin config schemas
func (schemas *PluginConfigSchemas) ListAll(db *gorm.DB) error {
	return db.Order("updated_at DESC").Find(schemas).Error
}

// ListByCommands returns schemas for specific commands
func (schemas *PluginConfigSchemas) ListByCommands(db *gorm.DB, commands []string) error {
	return db.Where("command IN ?", commands).Find(schemas).Error
}

// DeleteExpired deletes schemas older than the specified age
func DeleteExpiredSchemas(db *gorm.DB, maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	result := db.Where("last_fetched < ?", cutoff).Delete(&PluginConfigSchema{})
	return result.RowsAffected, result.Error
}

// CountSchemas returns the total number of cached schemas
func CountSchemas(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&PluginConfigSchema{}).Count(&count).Error
	return count, err
}
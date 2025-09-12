package models

import (
	"time"

	"gorm.io/gorm"
)

// ConfigurationChange represents a configuration change that needs to be propagated to edges
type ConfigurationChange struct {
	gorm.Model
	ID                 uint                   `json:"id" gorm:"primaryKey"`
	ChangeType         string                 `json:"change_type" gorm:"not null;index:idx_config_changes_type"`
	EntityType         string                 `json:"entity_type" gorm:"not null;index:idx_config_changes_entity"`
	EntityID           uint                   `json:"entity_id" gorm:"not null"`
	EntityData         map[string]interface{} `json:"entity_data" gorm:"serializer:json"`
	Namespace          string                 `json:"namespace" gorm:"default:'';index:idx_config_changes_namespace"`
	PropagatedToEdges  []string               `json:"propagated_to_edges" gorm:"serializer:json"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
	Processed          bool                   `json:"processed" gorm:"default:false;index:idx_config_changes_processed"`
}

type ConfigurationChanges []ConfigurationChange

// Configuration change constants
const (
	// Change types
	ChangeTypeCreate = "CREATE"
	ChangeTypeUpdate = "UPDATE"
	ChangeTypeDelete = "DELETE"

	// Entity types
	EntityTypeLLM         = "LLM"
	EntityTypeApp         = "APP"
	EntityTypeAPIToken    = "TOKEN"
	EntityTypeModelPrice  = "MODEL_PRICE"
	EntityTypeFilter      = "FILTER"
	EntityTypePlugin      = "PLUGIN"
)

// NewConfigurationChange creates a new ConfigurationChange
func NewConfigurationChange() *ConfigurationChange {
	return &ConfigurationChange{
		Processed:         false,
		PropagatedToEdges: make([]string, 0),
	}
}

// Get retrieves a configuration change by ID
func (c *ConfigurationChange) Get(db *gorm.DB, id uint) error {
	return db.First(c, id).Error
}

// Create creates a new configuration change
func (c *ConfigurationChange) Create(db *gorm.DB) error {
	return db.Create(c).Error
}

// Update updates an existing configuration change
func (c *ConfigurationChange) Update(db *gorm.DB) error {
	return db.Save(c).Error
}

// Delete soft deletes a configuration change
func (c *ConfigurationChange) Delete(db *gorm.DB) error {
	return db.Delete(c).Error
}

// MarkAsProcessed marks the change as processed
func (c *ConfigurationChange) MarkAsProcessed(db *gorm.DB) error {
	c.Processed = true
	return db.Model(c).Update("processed", true).Error
}

// AddPropagatedEdge adds an edge ID to the list of edges that received this change
func (c *ConfigurationChange) AddPropagatedEdge(db *gorm.DB, edgeID string) error {
	// Check if edge is already in the list
	for _, id := range c.PropagatedToEdges {
		if id == edgeID {
			return nil // Already propagated
		}
	}
	
	c.PropagatedToEdges = append(c.PropagatedToEdges, edgeID)
	return db.Model(c).Update("propagated_to_edges", c.PropagatedToEdges).Error
}

// ListUnprocessedChanges returns all unprocessed changes
func (changes *ConfigurationChanges) ListUnprocessedChanges(db *gorm.DB) error {
	return db.Where("processed = ?", false).
		Order("created_at ASC").
		Find(changes).Error
}

// ListUnprocessedChangesInNamespace returns unprocessed changes for a specific namespace
func (changes *ConfigurationChanges) ListUnprocessedChangesInNamespace(db *gorm.DB, namespace string) error {
	return db.Where("processed = ? AND (namespace = ? OR namespace = '')", false, namespace).
		Order("created_at ASC").
		Find(changes).Error
}

// ListChangesByEntityType returns changes for a specific entity type
func (changes *ConfigurationChanges) ListChangesByEntityType(db *gorm.DB, entityType string) error {
	return db.Where("entity_type = ?", entityType).
		Order("created_at DESC").
		Find(changes).Error
}

// ListRecentChanges returns recent changes within the specified duration
func (changes *ConfigurationChanges) ListRecentChanges(db *gorm.DB, since time.Duration) error {
	cutoff := time.Now().Add(-since)
	return db.Where("created_at >= ?", cutoff).
		Order("created_at DESC").
		Find(changes).Error
}

// CountUnprocessedChanges returns the count of unprocessed changes
func (c *ConfigurationChange) CountUnprocessedChanges(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&ConfigurationChange{}).Where("processed = ?", false).Count(&count).Error
	return count, err
}

// CountChangesByNamespace returns the count of changes in a specific namespace
func (c *ConfigurationChange) CountChangesByNamespace(db *gorm.DB, namespace string) (int64, error) {
	var count int64
	err := db.Model(&ConfigurationChange{}).
		Where("namespace = ? OR namespace = ''", namespace).
		Count(&count).Error
	return count, err
}

// CleanupOldChanges removes processed changes older than the specified duration
func (c *ConfigurationChange) CleanupOldChanges(db *gorm.DB, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	return db.Where("processed = ? AND created_at < ?", true, cutoff).
		Delete(&ConfigurationChange{}).Error
}

// CreateLLMChange creates a configuration change for an LLM
func CreateLLMChange(db *gorm.DB, changeType string, llm *LLM) error {
	change := &ConfigurationChange{
		ChangeType: changeType,
		EntityType: EntityTypeLLM,
		EntityID:   llm.ID,
		EntityData: map[string]interface{}{
			"id":          llm.ID,
			"name":        llm.Name,
			"vendor":      llm.Vendor,
			"active":      llm.Active,
			"namespace":   llm.Namespace,
		},
		Namespace: llm.Namespace,
		Processed: false,
	}
	return change.Create(db)
}

// CreateAppChange creates a configuration change for an App
func CreateAppChange(db *gorm.DB, changeType string, app *App) error {
	change := &ConfigurationChange{
		ChangeType: changeType,
		EntityType: EntityTypeApp,
		EntityID:   app.ID,
		EntityData: map[string]interface{}{
			"id":          app.ID,
			"name":        app.Name,
			"description": app.Description,
			"namespace":   app.Namespace,
		},
		Namespace: app.Namespace,
		Processed: false,
	}
	return change.Create(db)
}

// CreateCredentialChange creates a configuration change for a Credential (AI Studio uses credentials, not API tokens)
func CreateCredentialChange(db *gorm.DB, changeType string, credential *Credential) error {
	change := &ConfigurationChange{
		ChangeType: changeType,
		EntityType: EntityTypeAPIToken, // Keep the same entity type for compatibility
		EntityID:   credential.ID,
		EntityData: map[string]interface{}{
			"id":        credential.ID,
			"key_id":    credential.KeyID,
			"secret":    credential.Secret,
			"active":    credential.Active,
		},
		Namespace: "", // Credentials are global in AI Studio
		Processed: false,
	}
	return change.Create(db)
}

// CreateFilterChange creates a configuration change for a Filter
func CreateFilterChange(db *gorm.DB, changeType string, filter *Filter) error {
	change := &ConfigurationChange{
		ChangeType: changeType,
		EntityType: EntityTypeFilter,
		EntityID:   filter.ID,
		EntityData: map[string]interface{}{
			"id":          filter.ID,
			"name":        filter.Name,
			"description": filter.Description,
			"namespace":   filter.Namespace,
		},
		Namespace: filter.Namespace,
		Processed: false,
	}
	return change.Create(db)
}
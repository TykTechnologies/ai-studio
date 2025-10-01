package models

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// AgentConfig represents an agent plugin configuration in AI Studio
// Agents are separate from Chats and leverage Apps for resource access
type AgentConfig struct {
	gorm.Model
	ID          uint                   `json:"id" gorm:"primaryKey"`
	Name        string                 `json:"name" gorm:"not null"`
	Slug        string                 `json:"slug" gorm:"uniqueIndex;not null"`
	Description string                 `json:"description"`
	PluginID    uint                   `json:"plugin_id" gorm:"not null;index:idx_agent_plugin"`
	Plugin      *Plugin                `json:"plugin,omitempty" gorm:"foreignKey:PluginID"`
	AppID       uint                   `json:"app_id" gorm:"not null;index:idx_agent_app"`
	App         *App                   `json:"app,omitempty" gorm:"foreignKey:AppID"`
	Config      map[string]interface{} `json:"config" gorm:"serializer:json"` // Plugin-specific configuration from GetConfigSchema
	Groups      []Group                `json:"groups" gorm:"many2many:agent_groups;"`
	IsActive    bool                   `json:"is_active" gorm:"default:true;index:idx_agent_is_active"`
	Namespace   string                 `json:"namespace" gorm:"default:'';index:idx_agent_namespace"`
}

type AgentConfigs []AgentConfig

// TableName returns the table name for the AgentConfig model
func (AgentConfig) TableName() string {
	return "agent_configs"
}

// NewAgentConfig creates a new AgentConfig instance
func NewAgentConfig() *AgentConfig {
	return &AgentConfig{
		IsActive: true,
		Config:   make(map[string]interface{}),
	}
}

// Validate performs validation on the AgentConfig
func (a *AgentConfig) Validate(db *gorm.DB) error {
	if a.Name == "" {
		return errors.New("agent name is required")
	}
	if a.Slug == "" {
		return errors.New("agent slug is required")
	}
	if a.PluginID == 0 {
		return errors.New("plugin ID is required")
	}
	if a.AppID == 0 {
		return errors.New("app ID is required")
	}

	// Verify plugin exists and is of type agent
	var plugin Plugin
	if err := db.First(&plugin, a.PluginID).Error; err != nil {
		return fmt.Errorf("plugin not found: %w", err)
	}
	if !plugin.IsAgentPlugin() {
		return fmt.Errorf("plugin is not of type agent (found: %s)", plugin.PluginType)
	}
	if !plugin.IsActive {
		return errors.New("plugin is not active")
	}

	// Verify app exists
	var app App
	if err := db.First(&app, a.AppID).Error; err != nil {
		return fmt.Errorf("app not found: %w", err)
	}
	if !app.IsActive {
		return errors.New("app is not active")
	}
	if app.CredentialID == 0 {
		return errors.New("app does not have a credential")
	}

	// Verify app has at least one LLM
	llmCount := db.Model(&app).Association("LLMs").Count()
	if llmCount == 0 {
		return errors.New("app must have at least one LLM")
	}

	return nil
}

// Create creates a new agent config
func (a *AgentConfig) Create(db *gorm.DB) error {
	if err := a.Validate(db); err != nil {
		return err
	}
	return db.Create(a).Error
}

// Get retrieves an agent config by ID
func (a *AgentConfig) Get(db *gorm.DB, id uint) error {
	return db.Preload("Plugin").Preload("App").Preload("App.LLMs").Preload("App.Tools").Preload("App.Datasources").Preload("App.Credential").Preload("Groups").First(a, id).Error
}

// GetBySlug retrieves an agent config by slug
func (a *AgentConfig) GetBySlug(db *gorm.DB, slug string) error {
	return db.Preload("Plugin").Preload("App").Preload("App.LLMs").Preload("App.Tools").Preload("App.Datasources").Preload("App.Credential").Preload("Groups").Where("slug = ?", slug).First(a).Error
}

// Update updates an existing agent config
func (a *AgentConfig) Update(db *gorm.DB) error {
	if err := a.Validate(db); err != nil {
		return err
	}
	return db.Save(a).Error
}

// Delete soft deletes an agent config
func (a *AgentConfig) Delete(db *gorm.DB) error {
	return db.Delete(a).Error
}

// Activate activates the agent config
func (a *AgentConfig) Activate(db *gorm.DB) error {
	a.IsActive = true
	return db.Save(a).Error
}

// Deactivate deactivates the agent config
func (a *AgentConfig) Deactivate(db *gorm.DB) error {
	a.IsActive = false
	return db.Save(a).Error
}

// AddGroup adds a group to the agent config
func (a *AgentConfig) AddGroup(db *gorm.DB, group *Group) error {
	return db.Model(a).Association("Groups").Append(group)
}

// RemoveGroup removes a group from the agent config
func (a *AgentConfig) RemoveGroup(db *gorm.DB, group *Group) error {
	return db.Model(a).Association("Groups").Delete(group)
}

// GetGroups retrieves all groups associated with the agent config
func (a *AgentConfig) GetGroups(db *gorm.DB) error {
	return db.Model(a).Association("Groups").Find(&a.Groups)
}

// ListWithPagination returns paginated list of agent configs with filtering
func (configs *AgentConfigs) ListWithPagination(db *gorm.DB, pageSize, pageNumber int, all bool, namespace string, isActive *bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&AgentConfig{})

	// Apply namespace filtering
	if namespace == "__ALL_NAMESPACES__" || namespace == "" {
		// No namespace filtering - return agents from all namespaces
	} else {
		// Specific namespace: include global agents (empty namespace) + agents in specified namespace
		query = query.Where("namespace = '' OR namespace = ?", namespace)
	}

	// Apply is_active filtering
	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}

	if err := query.Count(&totalCount).Error; err != nil {
		return 0, 0, err
	}

	totalPages := 0
	if totalCount > 0 {
		if all {
			totalPages = 1
		} else {
			totalPages = int(totalCount) / pageSize
			if int(totalCount)%pageSize != 0 {
				totalPages++
			}
		}
	}

	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	err := query.Preload("Plugin").Preload("App").Preload("Groups").Order("created_at DESC").Find(configs).Error
	return totalCount, totalPages, err
}

// GetByPluginID returns all agent configs for a specific plugin
func (configs *AgentConfigs) GetByPluginID(db *gorm.DB, pluginID uint) error {
	return db.Where("plugin_id = ? AND is_active = ?", pluginID, true).
		Preload("Plugin").Preload("App").Preload("Groups").
		Order("created_at DESC").
		Find(configs).Error
}

// GetByAppID returns all agent configs for a specific app
func (configs *AgentConfigs) GetByAppID(db *gorm.DB, appID uint) error {
	return db.Where("app_id = ? AND is_active = ?", appID, true).
		Preload("Plugin").Preload("App").Preload("Groups").
		Order("created_at DESC").
		Find(configs).Error
}

// CountActive returns the count of active agent configs
func (a *AgentConfig) CountActive(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&AgentConfig{}).Where("is_active = ?", true).Count(&count).Error
	return count, err
}

// CountByPluginID returns the count of agent configs for a specific plugin
func (a *AgentConfig) CountByPluginID(db *gorm.DB, pluginID uint) (int64, error) {
	var count int64
	err := db.Model(&AgentConfig{}).Where("plugin_id = ? AND is_active = ?", pluginID, true).Count(&count).Error
	return count, err
}

// HasAccessForUser checks if a user has access to this agent via groups
func (a *AgentConfig) HasAccessForUser(db *gorm.DB, userID uint) (bool, error) {
	// If agent has no groups assigned, it's available to all users
	groupCount := db.Model(a).Association("Groups").Count()
	if groupCount == 0 {
		return true, nil
	}

	// Check if user is in any of the agent's groups
	var user User
	if err := db.Preload("Groups").First(&user, userID).Error; err != nil {
		return false, fmt.Errorf("failed to load user: %w", err)
	}

	if err := a.GetGroups(db); err != nil {
		return false, fmt.Errorf("failed to load agent groups: %w", err)
	}

	// Check for group overlap
	for _, agentGroup := range a.Groups {
		for _, userGroup := range user.Groups {
			if agentGroup.ID == userGroup.ID {
				return true, nil
			}
		}
	}

	return false, nil
}

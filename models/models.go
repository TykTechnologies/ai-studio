package models

import (
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

func InitModels(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&User{},      //Done
		&Group{},     //Done
		&LLM{},       //Done
		&Catalogue{}, //Done
		&Tag{},
		&Datasource{},    //Done
		&DataCatalogue{}, //Done
		&Credential{},    // Done [partially handled by Apps]
		&App{},           // Done
		&LLMSettings{},   //Done
		&Chat{},
		&CMessage{},
		&Tool{},       //Done
		&ModelPrice{}, //Done
		&Filter{},     // Done
		&ChatHistoryRecord{},
		&ToolCatalogue{}, // Done
		&secrets.Secret{},
		&LLMChatRecord{},
		&Notification{},   // For storing notifications
		&PromptTemplate{}, // For storing prompt templates
		&OAuthClient{},
		&AuthCode{},
		&AccessToken{},
		&PendingOAuthRequest{},
		// Hub-and-Spoke Models
		&EdgeInstance{},       // Edge instance tracking
		&Plugin{},             // Plugin configurations
		&LLMPlugin{},          // LLM-Plugin associations
		&PluginConfigSchema{}, // Plugin config schema cache
		&RegisteredPlugin{},   // Registered plugins with parsed manifests
		&UIRegistry{},         // UI component registry
		&PluginData{},         // Plugin key-value storage
		&AgentConfig{},        // Agent plugin configurations
		// Marketplace Models
		&MarketplacePlugin{},      // Marketplace plugin catalog cache
		&MarketplaceIndex{},       // Marketplace index metadata
		&InstalledPluginVersion{}, // Installed plugin version tracking
		&MarketplaceConfig{},      // Marketplace configuration
		&BrandingSettings{},       // UI branding customization
		// Scheduler Models
		&PluginSchedule{},          // Plugin scheduled tasks
		&PluginScheduleExecution{}, // Schedule execution history
		&SchedulerLease{},          // Scheduler leader election
		// Export Models
		&ProxyLogExport{}, // Proxy log export jobs (Enterprise)
	); err != nil {
		return err
	}

	if err := db.Table("group_catalogues").AutoMigrate(&struct {
		GroupID     uint `gorm:"primaryKey"`
		CatalogueID uint `gorm:"primaryKey"`
	}{}); err != nil {
		return err
	}

	// Initialize user-group relationship table
	err := db.Table("user_groups").AutoMigrate(&struct {
		UserID  uint `gorm:"primaryKey"`
		GroupID uint `gorm:"primaryKey"`
	}{})

	return err
}

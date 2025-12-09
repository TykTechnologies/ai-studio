package models

import (
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

func InitModels(db *gorm.DB) error {
	// Migration: model_mappings moved from pool_id to vendor_id
	// Drop the old pool_id column if it exists (this is a breaking schema change)
	if db.Migrator().HasColumn(&ModelMapping{}, "pool_id") {
		// First, clear any existing mappings that reference pool_id
		// (they're orphaned now since we changed to vendor_id)
		db.Exec("DELETE FROM model_mappings WHERE vendor_id IS NULL OR vendor_id = 0")
		// Then drop the old column
		if err := db.Migrator().DropColumn(&ModelMapping{}, "pool_id"); err != nil {
			// Log but don't fail - column might already be dropped
			// or this might be a fresh install
		}
	}

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
		// Model Router Models (Enterprise)
		&ModelRouter{},  // Model router configurations
		&ModelPool{},    // Model pools with patterns
		&PoolVendor{},   // Pool-LLM vendor associations
		&ModelMapping{}, // Model name mappings
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

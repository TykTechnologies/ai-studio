package models

import (
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/gosimple/slug"
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

	// Migration: Drop orphaned slug column from plugins table
	// The slug field was removed from the Plugin model but the column remained
	// in the database with a NOT NULL constraint, causing INSERT failures.
	if db.Migrator().HasColumn(&Plugin{}, "slug") {
		if err := db.Migrator().DropColumn(&Plugin{}, "slug"); err != nil {
			// Log but don't fail - column might already be dropped
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
		// Sync Status Models
		&NamespaceSyncStatus{}, // Namespace configuration sync status tracking
		&SyncAuditLog{},        // Sync audit log for control-edge synchronization
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
		// UGC (User-Generated Content) Models
		&Submission{},          // Community resource submissions
		&AttestationTemplate{}, // Admin-configurable attestation templates
		&SubmissionVersion{},   // Resource version snapshots for rollback
		&SubmissionActivity{},  // Submission review action audit trail
		// Pluggable Resource Types
		&PluginResourceType{},  // Plugin-registered resource types
		&AppPluginResource{},   // App ↔ plugin resource instance associations
		&GroupPluginResource{}, // Group ↔ plugin resource instance access control
		// Outbound Webhooks
		&WebhookSubscription{}, // Webhook endpoint subscriptions
		&WebhookTopic{},        // Subscription–topic join table
		&WebhookEvent{},        // Persistent delivery queue
		&WebhookDeliveryLog{},  // Per-attempt delivery audit log
		&WebhookConfig{},       // Global webhook runtime configuration (singleton)
	); err != nil {
		return err
	}

	// Migration: Populate Tool.Slug for existing records
	// This ensures tools created before the Slug field was added get their slugs computed
	var toolCount int64
	db.Model(&Tool{}).Where("slug = '' OR slug IS NULL").Count(&toolCount)
	if toolCount > 0 {
		var tools []Tool
		db.Find(&tools)
		for i := range tools {
			if tools[i].Slug == "" {
				tools[i].Slug = slug.Make(tools[i].Name)
				db.Model(&tools[i]).Update("slug", tools[i].Slug)
			}
		}
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

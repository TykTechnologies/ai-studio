package models

import (
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

func fixOAuthClientUserIDConstraint(db *gorm.DB) error {
	// Check if we're using PostgreSQL
	dialect := db.Dialector.Name()
	if dialect != "postgres" {
		// SQLite and other databases don't have the same strict constraint issues
		return nil
	}

	// For PostgreSQL, we need to alter the user_id column to allow NULL values
	// Check if the constraint exists first
	var constraintExists bool
	err := db.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.table_constraints 
			WHERE constraint_name = 'fk_o_auth_clients_user' 
			AND table_name = 'o_auth_clients'
		)
	`).Scan(&constraintExists).Error

	if err != nil {
		return err
	}

	if constraintExists {
		// Drop the foreign key constraint temporarily
		if err := db.Exec(`ALTER TABLE o_auth_clients DROP CONSTRAINT IF EXISTS fk_o_auth_clients_user`).Error; err != nil {
			return err
		}
	}

	// Make user_id column nullable
	if err := db.Exec(`ALTER TABLE o_auth_clients ALTER COLUMN user_id DROP NOT NULL`).Error; err != nil {
		return err
	}

	// Re-add the foreign key constraint but allow NULL values
	if err := db.Exec(`
		ALTER TABLE o_auth_clients 
		ADD CONSTRAINT fk_o_auth_clients_user 
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	`).Error; err != nil {
		return err
	}

	return nil
}

func InitModels(db *gorm.DB) error {
	err := db.AutoMigrate(
		&User{},      //Done
		&Group{},     //Done
		&LLM{},       //Done
		&Catalogue{}, //Done
		&Tags{},
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
	)

	// Fix OAuth client user_id constraint for PostgreSQL
	err = fixOAuthClientUserIDConstraint(db)
	if err != nil {
		return err
	}

	err = db.Table("group_catalogues").AutoMigrate(&struct {
		GroupID     uint `gorm:"primaryKey"`
		CatalogueID uint `gorm:"primaryKey"`
	}{})
	if err != nil {
		return err
	}

	// Initialize user-group relationship table
	err = db.Table("user_groups").AutoMigrate(&struct {
		UserID  uint `gorm:"primaryKey"`
		GroupID uint `gorm:"primaryKey"`
	}{})

	return err
}

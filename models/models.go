package models

import (
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

// func fixLLMChatRecordIDs(db *gorm.DB) error {
// 	// Update anthropic chat records
// 	if err := db.Exec(`
// 		UPDATE llm_chat_records
// 		SET interaction_type = 'proxy',
// 		    name = 'claude-3-5-sonnet-20241022',
// 		    currency = 'USD'
// 		WHERE llm_id = 5
// 	`).Error; err != nil {
// 		return err
// 	}

// 	return nil
// }

func InitModels(db *gorm.DB) error {
	// Fix LLM chat record IDs
	// fixLLMChatRecordIDs(db)

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
		&Notification{}, // For storing notifications
		&PromptTemplate{}, // For storing prompt templates
		&OAuthClient{},
		&AuthCode{},
		&AccessToken{},
		&PendingOAuthRequest{},
	)

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

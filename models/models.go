package models

import (
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

func cleanupDuplicateModelPrices(db *gorm.DB) error {
	// Delete duplicates keeping the oldest record for each model_name + vendor combination
	return db.Exec(`
		DELETE FROM model_prices 
		WHERE id NOT IN (
			SELECT MIN(id)
			FROM (SELECT * FROM model_prices) as mp
			GROUP BY model_name, vendor
		)
	`).Error
}

func InitModels(db *gorm.DB) error {
	// First clean up any duplicate model prices before adding the unique constraint
	cleanupDuplicateModelPrices(db)

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

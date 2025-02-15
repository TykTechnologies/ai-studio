package models

import (
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

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
	)

	err = db.Table("group_catalogues").AutoMigrate(&struct {
		GroupID     uint `gorm:"primaryKey"`
		CatalogueID uint `gorm:"primaryKey"`
	}{})

	return err
}

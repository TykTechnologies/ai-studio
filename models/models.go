package models

import "gorm.io/gorm"

func InitModels(db *gorm.DB) error {
	err := db.AutoMigrate(
		&User{},      //Done
		&Group{},     //Done
		&LLM{},       //Done
		&Catalogue{}, //Done
		&Tags{},
		&Datasource{},    //Done
		&DataCatalogue{}, //Done
		&Credential{},
		&App{},
		&LLMSettings{}, //Done
		&Chat{},
		&CMessage{},
		&Tool{},       //Done
		&ModelPrice{}, //Done
		&Filter{},
		&ChatHistoryRecord{},
		&ToolCatalogue{}, // Done
	)

	err = db.Table("group_catalogues").AutoMigrate(&struct {
		GroupID     uint `gorm:"primaryKey"`
		CatalogueID uint `gorm:"primaryKey"`
	}{})

	return err
}

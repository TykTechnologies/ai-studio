package models

import "gorm.io/gorm"

func InitModels(db *gorm.DB) error {
	err := db.AutoMigrate(
		&User{},  //Done
		&Group{}, //Done
		&LLM{},   //Done
		&Catalogue{},
		&Tags{},
		&Datasource{}, //Done
		&DataCatalogue{},
		&Credential{},
		&App{},
		&LLMSettings{}, //Done
		&Chat{},
		&CMessage{},
		&Tool{},
		&ModelPrice{}, //Done
		&Filter{},
		&ChatHistoryRecord{},
		&ToolCatalogue{},
	)

	err = db.Table("group_catalogues").AutoMigrate(&struct {
		GroupID     uint `gorm:"primaryKey"`
		CatalogueID uint `gorm:"primaryKey"`
	}{})

	return err
}

package models

import "gorm.io/gorm"

func InitModels(db *gorm.DB) error {
	err := db.AutoMigrate(
		&User{},
		&Group{},
		&LLM{},
		&Catalogue{},
		&Tags{},
		&Datasource{},
		&DataCatalogue{},
	)

	err = db.Table("group_catalogues").AutoMigrate(&struct {
		GroupID     uint `gorm:"primaryKey"`
		CatalogueID uint `gorm:"primaryKey"`
	}{})

	return err
}

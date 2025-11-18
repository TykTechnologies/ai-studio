package models

import "gorm.io/gorm"

const DefaultDataCatalogueName = "Default"

type DataCatalogue struct {
	gorm.Model
	ID               uint         `json:"id" gorm:"primaryKey"`
	Name             string       `json:"name"`
	ShortDescription string       `json:"short_description"`
	LongDescription  string       `json:"long_description"`
	Icon             string       `json:"icon"`
	Datasources      []Datasource `json:"data_sources" gorm:"many2many:data_catalogue_data_sources;"`
	Tags             []Tag        `json:"tags" gorm:"many2many:data_catalogue_tags;"`
}

type DataCatalogues []DataCatalogue

func NewDataCatalogue() *DataCatalogue {
	return &DataCatalogue{}
}

// Create a new data catalogue
func (dc *DataCatalogue) Create(db *gorm.DB) error {
	return db.Create(dc).Error
}

// Get a data catalogue by ID
func (dc *DataCatalogue) Get(db *gorm.DB, id uint) error {
	return db.Preload("Datasources", func(db *gorm.DB) *gorm.DB {
		return db.Where("active = ?", true)
	}).Preload("Tags").First(dc, id).Error
}

// Update an existing data catalogue
func (dc *DataCatalogue) Update(db *gorm.DB) error {
	return db.Save(dc).Error
}

// Delete a data catalogue
func (dc *DataCatalogue) Delete(db *gorm.DB) error {
	return db.Delete(dc).Error
}

// Add a tag to the data catalogue
func (dc *DataCatalogue) AddTag(db *gorm.DB, tag *Tag) error {
	return db.Model(dc).Association("Tags").Append(tag)
}

// Remove a tag from the data catalogue
func (dc *DataCatalogue) RemoveTag(db *gorm.DB, tag *Tag) error {
	return db.Model(dc).Association("Tags").Delete(tag)
}

// Add a data source to the data catalogue
func (dc *DataCatalogue) AddDatasource(db *gorm.DB, datasource *Datasource) error {
	return db.Model(dc).Association("Datasources").Append(datasource)
}

// Remove a data source from the data catalogue
func (dc *DataCatalogue) RemoveDatasource(db *gorm.DB, datasource *Datasource) error {
	return db.Model(dc).Association("Datasources").Delete(datasource)
}

// Get all data catalogues
func (dc *DataCatalogues) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&DataCatalogue{})

	if err := query.Count(&totalCount).Error; err != nil {
		return 0, 0, err
	}

	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	err := query.Preload("Datasources").Preload("Tags").Find(dc).Error
	return totalCount, totalPages, err
}

// Search data catalogues by name, short description, and long description
func (dc *DataCatalogues) Search(db *gorm.DB, query string) error {
	return db.Preload("Datasources").Preload("Tags").
		Where("name LIKE ? OR short_description LIKE ? OR long_description LIKE ?",
			"%"+query+"%", "%"+query+"%", "%"+query+"%").
		Find(dc).Error
}

// Get data catalogues by tag
func (dc *DataCatalogues) GetByTag(db *gorm.DB, tagName string) error {
	return db.Preload("Datasources").Preload("Tags").
		Joins("JOIN data_catalogue_tags ON data_catalogue_tags.data_catalogue_id = data_catalogues.id").
		Joins("JOIN tags ON tags.id = data_catalogue_tags.tag_id").
		Where("tags.name = ?", tagName).
		Find(dc).Error
}

// Get data catalogues by datasource
func (dc *DataCatalogues) GetByDatasource(db *gorm.DB, datasourceID uint) error {
	return db.Preload("Datasources").Preload("Tags").
		Joins("JOIN data_catalogue_data_sources ON data_catalogue_data_sources.data_catalogue_id = data_catalogues.id").
		Where("data_catalogue_data_sources.datasource_id = ?", datasourceID).
		Find(dc).Error
}

// GetOrCreateDefaultDataCatalogue finds or creates the Default data catalogue by name
// This is safe for databases where auto-increment has been reset or cleared
func GetOrCreateDefaultDataCatalogue(db *gorm.DB) (*DataCatalogue, error) {
	var catalogue DataCatalogue
	err := db.Where("name = ?", DefaultDataCatalogueName).First(&catalogue).Error

	if err == gorm.ErrRecordNotFound {
		// Create Default data catalogue
		catalogue = DataCatalogue{Name: DefaultDataCatalogueName}
		err = db.Create(&catalogue).Error
	}

	return &catalogue, err
}

// IsDefault checks if this data catalogue is the default data catalogue
func (dc *DataCatalogue) IsDefault() bool {
	return dc.Name == DefaultDataCatalogueName
}

package models

import (
	"gorm.io/gorm"
)

type Datasource struct {
	gorm.Model
	ID               uint   `json:"id" gorm:"primaryKey"`
	Name             string `json:"name"`
	ShortDescription string `json:"short_description"`
	LongDescription  string `json:"long_description"`
	Icon             string `json:"icon"`
	Url              string `json:"url"`
	PrivacyScore     int    `json:"privacy_score"`
	UserID           uint   `json:"user_id" gorm:"foreignKey:ID"`
	Tags             []Tag  `json:"tags" gorm:"many2many:datasource_tags;"`

	DBConnString string `json:"db_conn_string"`
	DBSourceType string `json:"db_source_type"`
	DBConnAPIKey string `json:"db_conn_api_key"`
	DBName       string `json:"db_name"`

	EmbedVendor Vendor `json:"embed_vendor"`
	EmbedUrl    string `json:"embed_url"`
	EmbedAPIKey string `json:"embed_api_key"`
	EmbedModel  string `json:"embed_model"`

	Active bool
}

type Datasources []Datasource

func NewDatasource() *Datasource {
	return &Datasource{}
}

// Create a new datasource
func (d *Datasource) Create(db *gorm.DB) error {
	return db.Create(d).Error
}

// Get a datasource by ID
func (d *Datasource) Get(db *gorm.DB, id uint) error {
	return db.Preload("Tags").First(d, id).Error
}

// Update an existing datasource
func (d *Datasource) Update(db *gorm.DB) error {
	return db.Save(d).Error
}

// Delete a datasource
func (d *Datasource) Delete(db *gorm.DB) error {
	return db.Delete(d).Error
}

// Get all datasources
func (d *Datasources) GetAll(db *gorm.DB) error {
	return db.Preload("Tags").Find(d).Error
}

// Search datasources by name, short description and long description
func (d *Datasources) Search(db *gorm.DB, query string) error {
	return db.Preload("Tags").Where("name LIKE ? OR short_description LIKE ? OR long_description LIKE ?", "%"+query+"%", "%"+query+"%", "%"+query+"%").Find(d).Error
}

// Fetch datasources by tag
func (d *Datasources) GetByTag(db *gorm.DB, tagName string) error {
	return db.Preload("Tags").Joins("JOIN datasource_tags ON datasource_tags.datasource_id = datasources.id").
		Joins("JOIN tags ON tags.id = datasource_tags.tag_id").
		Where("tags.name = ?", tagName).
		Find(d).Error
}

// Add tags to a datasource
func (d *Datasource) AddTags(db *gorm.DB, tagNames []string) error {
	for _, tagName := range tagNames {
		var tag Tag
		if err := db.Where("name = ?", tagName).FirstOrCreate(&tag, Tag{Name: tagName}).Error; err != nil {
			return err
		}
		if err := db.Model(d).Association("Tags").Append(&tag); err != nil {
			return err
		}
	}
	return nil
}

// Filter datasources by minimum privacy score
func (d *Datasources) GetByMinPrivacyScore(db *gorm.DB, minScore int) error {
	return db.Preload("Tags").Where("privacy_score >= ?", minScore).Find(d).Error
}

// Filter datasources by maximum privacy score
func (d *Datasources) GetByMaxPrivacyScore(db *gorm.DB, maxScore int) error {
	return db.Preload("Tags").Where("privacy_score <= ?", maxScore).Find(d).Error
}

// Filter datasources by privacy score range
func (d *Datasources) GetByPrivacyScoreRange(db *gorm.DB, minScore, maxScore int) error {
	return db.Preload("Tags").Where("privacy_score BETWEEN ? AND ?", minScore, maxScore).Find(d).Error
}

// Get all datasources belonging to a specific user
func (d *Datasources) GetByUserID(db *gorm.DB, userID uint) error {
	return db.Preload("Tags").Where("user_id = ?", userID).Find(d).Error
}

func (d *Datasources) GetActiveDataSources(db *gorm.DB) error {
	return db.Preload("Tags").Where("active = ?", true).Find(d).Error
}

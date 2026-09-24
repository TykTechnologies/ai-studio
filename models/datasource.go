package models

import (
	"gorm.io/gorm"
)

type Datasource struct {
	gorm.Model
	ID               uint   `json:"id" gorm:"primaryKey"`
	Name             string `json:"name" gorm:"index"`
	ShortDescription string `json:"short_description"`
	LongDescription  string `json:"long_description"`
	Icon             string `json:"icon"`
	Url              string `json:"url"`
	PrivacyScore     int    `json:"privacy_score"`
	UserID           uint   `json:"user_id" gorm:"foreignKey:ID;index:idx_ds_user_community"`
	Tags             []Tag  `json:"tags" gorm:"many2many:datasource_tags;"`

	DBConnString string `json:"db_conn_string" gorm:"index"`
	DBSourceType string `json:"db_source_type"`
	DBConnAPIKey string `json:"db_conn_api_key"`
	DBName       string `json:"db_name"`

	// EmbedderID is the embedder that turns this datasource's text into
	// vectors. The embed_vendor/url/api_key/model columns it replaced stay in
	// the table (cleared by MigrateEmbedders) until a later release drops
	// them; runtime code reads the embedder, never those columns.
	EmbedderID *uint     `json:"embedder_id" gorm:"index"`
	Embedder   *Embedder `json:"-" gorm:"foreignKey:EmbedderID"`

	Files []FileStore `gorm:"many2many:datasource_filestores;" json:"files"`

	Active    bool
	Namespace string `json:"namespace" gorm:"default:'';index:idx_datasource_namespace"`

	// UGC (User-Generated Content) fields
	CommunitySubmitted bool  `json:"community_submitted" gorm:"index:idx_ds_user_community"`
	SubmissionID       *uint `json:"submission_id"`

	// Plugin-stored metadata
	Metadata JSONMap `json:"metadata" gorm:"type:json"`

	// ext carries JSON-only state (see datasource_json.go).
	ext *datasourceExtra
}

type Datasources []Datasource

// SetEmbedder links the datasource to an embedder (nil unlinks).
func (d *Datasource) SetEmbedder(e *Embedder) {
	d.Embedder = e
	if e == nil {
		d.EmbedderID = nil
		return
	}
	id := e.ID
	d.EmbedderID = &id
}

func NewDatasource() *Datasource {
	return &Datasource{}
}

// Create a new datasource
func (d *Datasource) Create(db *gorm.DB) error {
	return db.Omit("Embedder").Create(d).Error
}

// Get a datasource by ID
func (d *Datasource) Get(db *gorm.DB, id uint) error {
	return db.Preload("Tags").Preload("Embedder.LLM").Preload("Files").First(d, id).Error
}

// Update an existing datasource
func (d *Datasource) Update(db *gorm.DB) error {
	return db.Omit("Embedder").Save(d).Error
}

// Delete a datasource
func (d *Datasource) Delete(db *gorm.DB) error {
	return db.Delete(d).Error
}

// Get all datasources
func (d *Datasources) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool, scopes ...func(*gorm.DB) *gorm.DB) (int64, int, error) {
	var totalCount int64
	query := db.Model(&Datasource{})

	// Optional search/sort scopes (services.ListOptions); applied before the
	// count so X-Total-Count reflects the filtered set.
	for _, scope := range scopes {
		query = scope(query)
	}
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

	err := query.Preload("Tags").Preload("Embedder.LLM").Find(d).Error
	return totalCount, totalPages, err
}

// GetAllWithFilters returns all datasources with filtering by active status and user ID
func (d *Datasources) GetAllWithFilters(db *gorm.DB, pageSize int, pageNumber int, all bool, isActive *bool, userID *uint) (int64, int, error) {
	var totalCount int64
	query := db.Model(&Datasource{})

	// Apply is_active filtering
	if isActive != nil {
		query = query.Where("active = ?", *isActive)
	}

	// Apply user_id filtering
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}

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

	err := query.Preload("Tags").Preload("Embedder.LLM").Find(d).Error
	return totalCount, totalPages, err
}

// Search datasources by name, short description and long description
func (d *Datasources) Search(db *gorm.DB, query string) error {
	return db.Preload("Tags").Preload("Embedder.LLM").Where("name LIKE ? OR short_description LIKE ? OR long_description LIKE ?", "%"+query+"%", "%"+query+"%", "%"+query+"%").Find(d).Error
}

// Fetch datasources by tag
func (d *Datasources) GetByTag(db *gorm.DB, tagName string) error {
	return db.Preload("Tags").Preload("Embedder.LLM").Joins("JOIN datasource_tags ON datasource_tags.datasource_id = datasources.id").
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

// Remove tags from a datasource
func (d *Datasource) RemoveTags(db *gorm.DB, tagNames []string) error {
	for _, tagName := range tagNames {
		var tag Tag
		if err := db.Where("name = ?", tagName).First(&tag).Error; err != nil {
			return err
		}
		if err := db.Model(d).Association("Tags").Delete(&tag); err != nil {
			return err
		}
	}
	return nil
}

// AddFileStore adds a FileStore to the DS
func (d *Datasource) AddFileStore(db *gorm.DB, fileStore *FileStore) error {
	return db.Model(d).Association("Files").Append(fileStore)
}

// RemoveFileStore removes a FileStore from the DS
func (d *Datasource) RemoveFileStore(db *gorm.DB, fileStore *FileStore) error {
	return db.Model(d).Association("Files").Delete(fileStore)
}

// Filter datasources by minimum privacy score
func (d *Datasources) GetByMinPrivacyScore(db *gorm.DB, minScore int) error {
	return db.Preload("Tags").Preload("Embedder.LLM").Where("privacy_score >= ?", minScore).Find(d).Error
}

// Filter datasources by maximum privacy score
func (d *Datasources) GetByMaxPrivacyScore(db *gorm.DB, maxScore int) error {
	return db.Preload("Tags").Preload("Embedder.LLM").Where("privacy_score <= ?", maxScore).Find(d).Error
}

// Filter datasources by privacy score range
func (d *Datasources) GetByPrivacyScoreRange(db *gorm.DB, minScore, maxScore int) error {
	return db.Preload("Tags").Preload("Embedder.LLM").Where("privacy_score BETWEEN ? AND ?", minScore, maxScore).Find(d).Error
}

// Get all datasources belonging to a specific user
func (d *Datasources) GetByUserID(db *gorm.DB, userID uint) error {
	return db.Preload("Tags").Preload("Embedder.LLM").Where("user_id = ?", userID).Find(d).Error
}

func (d *Datasources) GetActiveDataSources(db *gorm.DB) error {
	return db.Preload("Tags").Preload("Embedder.LLM").Where("active = ?", true).Find(d).Error
}

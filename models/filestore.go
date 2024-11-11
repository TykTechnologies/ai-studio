package models

import "gorm.io/gorm"

type FileStore struct {
	gorm.Model
	ID          uint   `gorm:"primary_key" json:"id"`
	FileName    string `json:"file_name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Length      int    `json:"length"`
	Tools       []Tool `gorm:"many2many:tool_filestores;" json:"-"` // Note the json:"-" to
}

// NewFileStore creates a new FileStore instance
func NewFileStore() *FileStore {
	return &FileStore{}
}

// Create a new filestore entry
func (f *FileStore) Create(db *gorm.DB) error {
	return db.Create(f).Error
}

// Get a filestore entry by ID
func (f *FileStore) Get(db *gorm.DB, id uint) error {
	return db.First(f, id).Error
}

// Update an existing filestore entry
func (f *FileStore) Update(db *gorm.DB) error {
	return db.Save(f).Error
}

// Delete a filestore entry
func (f *FileStore) Delete(db *gorm.DB) error {
	return db.Delete(f).Error
}

// FileStores represents a collection of FileStore
type FileStores []FileStore

// GetAll retrieves all filestore entries with pagination
func (f *FileStores) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&FileStore{})

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

	err := query.Find(f).Error
	return totalCount, totalPages, err
}

// GetByFileName gets a filestore entry by its filename
func (f *FileStore) GetByFileName(db *gorm.DB, fileName string) error {
	return db.Where("file_name = ?", fileName).First(f).Error
}

// Search retrieves all filestore entries matching the given query in filename or description
func (f *FileStores) Search(db *gorm.DB, query string) error {
	return db.Where("file_name LIKE ? OR description LIKE ?", "%"+query+"%", "%"+query+"%").Find(f).Error
}

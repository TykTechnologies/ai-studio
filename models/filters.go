package models

import "gorm.io/gorm"

type Filter struct {
	gorm.Model
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Script      []byte `json:"script"`
}

func NewFilter() *Filter {
	return &Filter{}
}

// Create a new filter
func (f *Filter) Create(db *gorm.DB) error {
	return db.Create(f).Error
}

// Get a filter by ID
func (f *Filter) Get(db *gorm.DB, id uint) error {
	return db.First(f, id).Error
}

// Update an existing filter
func (f *Filter) Update(db *gorm.DB) error {
	return db.Save(f).Error
}

// Delete a filter
func (f *Filter) Delete(db *gorm.DB) error {
	return db.Delete(f).Error
}

// GetAll retrieves all filters
func (f *Filter) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool) ([]Filter, int64, int, error) {
	var filters []Filter
	var totalCount int64
	query := db.Model(&Filter{})

	if err := query.Count(&totalCount).Error; err != nil {
		return nil, 0, 0, err
	}

	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	err := query.Find(&filters).Error
	return filters, totalCount, totalPages, err
}

// GetByName gets a filter by its name
func (f *Filter) GetByName(db *gorm.DB, name string) error {
	return db.Where("name = ?", name).First(f).Error
}

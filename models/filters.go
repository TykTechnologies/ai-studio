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
func (f *Filter) GetAll(db *gorm.DB) ([]Filter, error) {
	var filters []Filter
	err := db.Find(&filters).Error
	return filters, err
}

// GetByName gets a filter by its name
func (f *Filter) GetByName(db *gorm.DB, name string) error {
	return db.Where("name = ?", name).First(f).Error
}

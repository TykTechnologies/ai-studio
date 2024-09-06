package models

import "gorm.io/gorm"

type Tag struct {
	gorm.Model
	ID   uint   `json:"id" gorm:"primaryKey"`
	Name string `json:"name"`
}

type Tags []Tag

func NewTag() *Tag {
	return &Tag{}
}

// Create a new tag
func (t *Tag) Create(db *gorm.DB) error {
	return db.Create(t).Error
}

// Get a tag by ID
func (t *Tag) Get(db *gorm.DB, id uint) error {
	return db.First(t, id).Error
}

// Update an existing tag
func (t *Tag) Update(db *gorm.DB) error {
	return db.Save(t).Error
}

// Delete a tag
func (t *Tag) Delete(db *gorm.DB) error {
	return db.Delete(t).Error
}

// Get all tags
func (t *Tags) GetAll(db *gorm.DB) error {
	return db.Find(t).Error
}

// Get tags by name stub
func (t *Tags) GetByNameStub(db *gorm.DB, stub string) error {
	return db.Where("name LIKE ?", stub+"%").Find(t).Error
}

// Get tag by exact name
func (t *Tag) GetByName(db *gorm.DB, name string) error {
	return db.Where("name = ?", name).First(t).Error
}

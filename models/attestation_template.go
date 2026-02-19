package models

import (
	"gorm.io/gorm"
)

const (
	AttestationAppliesToDatasource = "datasource"
	AttestationAppliesToTool       = "tool"
	AttestationAppliesToAll        = "all"
)

type AttestationTemplate struct {
	gorm.Model
	ID            uint   `json:"id" gorm:"primaryKey"`
	Name          string `json:"name"`
	Text          string `json:"text"`
	Required      bool   `json:"required"`
	AppliesToType string `json:"applies_to_type"` // datasource | tool | all
	Active        bool   `json:"active"`
	SortOrder     int    `json:"sort_order"`
}

type AttestationTemplates []AttestationTemplate

func NewAttestationTemplate() *AttestationTemplate {
	return &AttestationTemplate{}
}

func (a *AttestationTemplate) Create(db *gorm.DB) error {
	return db.Create(a).Error
}

func (a *AttestationTemplate) Get(db *gorm.DB, id uint) error {
	return db.First(a, id).Error
}

func (a *AttestationTemplate) Update(db *gorm.DB) error {
	return db.Save(a).Error
}

func (a *AttestationTemplate) Delete(db *gorm.DB) error {
	return db.Delete(a).Error
}

// GetAll retrieves all attestation templates, optionally filtered
func (a *AttestationTemplates) GetAll(db *gorm.DB, activeOnly bool) error {
	query := db.Model(&AttestationTemplate{})
	if activeOnly {
		query = query.Where("active = ?", true)
	}
	return query.Order("sort_order ASC").Find(a).Error
}

// GetByType retrieves templates applicable to a specific resource type
func (a *AttestationTemplates) GetByType(db *gorm.DB, resourceType string, activeOnly bool) error {
	query := db.Model(&AttestationTemplate{}).
		Where("applies_to_type = ? OR applies_to_type = ?", resourceType, AttestationAppliesToAll)
	if activeOnly {
		query = query.Where("active = ?", true)
	}
	return query.Order("sort_order ASC").Find(a).Error
}

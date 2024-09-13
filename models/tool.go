package models

import (
	"strings"

	"gorm.io/gorm"
)

type Tool struct {
	gorm.Model
	ID          uint   `json:"id" gorm:"primary_key"`
	Name        string `json:"name"`
	Description string `json:"description"`

	ToolType            string `json:"tool_type"`
	OASSpec             []byte `json:"oas_spec"`
	AvailableOperations string `json:"available_operations"`
	PrivacyScore        int    `json:"privacy_score"`
	AuthKey             string `json:"auth_key"`
	AuthSchemaName      string `json:"auth_schema_name"`
}

type Tools []Tool

const (
	ToolTypeREST = "REST"
)

func NewTool() *Tool {
	return &Tool{}
}

// Create a new tool
func (t *Tool) Create(db *gorm.DB) error {
	return db.Create(t).Error
}

// Get a tool by ID
func (t *Tool) Get(db *gorm.DB, id uint) error {
	return db.First(t, id).Error
}

// Update an existing tool
func (t *Tool) Update(db *gorm.DB) error {
	return db.Save(t).Error
}

// Delete a tool
func (t *Tool) Delete(db *gorm.DB) error {
	return db.Delete(t).Error
}

// GetByName gets a tool by its name
func (t *Tool) GetByName(db *gorm.DB, name string) error {
	return db.Where("name = ?", name).First(t).Error
}

// GetAll retrieves all tools
func (t *Tools) GetAll(db *gorm.DB) error {
	return db.Find(t).Error
}

// GetByType retrieves all tools of a specific type
func (t *Tools) GetByType(db *gorm.DB, toolType string) error {
	return db.Where("tool_type = ?", toolType).Find(t).Error
}

// GetByPrivacyScoreMin retrieves all tools with a privacy score greater than or equal to the given minimum
func (t *Tools) GetByPrivacyScoreMin(db *gorm.DB, minScore float64) error {
	return db.Where("privacy_score >= ?", minScore).Find(t).Error
}

// GetByPrivacyScoreMax retrieves all tools with a privacy score less than or equal to the given maximum
func (t *Tools) GetByPrivacyScoreMax(db *gorm.DB, maxScore float64) error {
	return db.Where("privacy_score <= ?", maxScore).Find(t).Error
}

// GetByPrivacyScoreRange retrieves all tools with a privacy score within the given range
func (t *Tools) GetByPrivacyScoreRange(db *gorm.DB, minScore, maxScore float64) error {
	return db.Where("privacy_score BETWEEN ? AND ?", minScore, maxScore).Find(t).Error
}

// Search retrieves all tools matching the given query in name or description
func (t *Tools) Search(db *gorm.DB, query string) error {
	return db.Where("name LIKE ? OR description LIKE ?", "%"+query+"%", "%"+query+"%").Find(t).Error
}

// AddOperation adds a new operation to the AvailableOperations list
func (t *Tool) AddOperation(operation string) {
	operations := t.GetOperations()
	for _, op := range operations {
		if op == operation {
			return // Operation already exists, do nothing
		}
	}
	if t.AvailableOperations == "" {
		t.AvailableOperations = operation
	} else {
		t.AvailableOperations += "," + operation
	}
}

// RemoveOperation removes an operation from the AvailableOperations list
func (t *Tool) RemoveOperation(operation string) {
	operations := t.GetOperations()
	var newOperations []string
	for _, op := range operations {
		if op != operation {
			newOperations = append(newOperations, op)
		}
	}
	t.AvailableOperations = strings.Join(newOperations, ",")
}

// GetOperations returns the AvailableOperations as a []string
func (t *Tool) GetOperations() []string {
	if t.AvailableOperations == "" {
		return []string{}
	}
	return strings.Split(t.AvailableOperations, ",")
}

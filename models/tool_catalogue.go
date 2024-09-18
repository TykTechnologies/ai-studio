package models

import "gorm.io/gorm"

type ToolCatalogue struct {
	gorm.Model
	ID               uint   `json:"id" gorm:"primaryKey"`
	Name             string `json:"name"`
	ShortDescription string `json:"short_description"`
	LongDescription  string `json:"long_description"`
	Icon             string `json:"icon"`
	Tools            []Tool `json:"tools" gorm:"many2many:tool_catalogue_tools;"`
	Tags             []Tag  `json:"tags" gorm:"many2many:tool_catalogue_tags;"`
}

type ToolCatalogues []ToolCatalogue

func NewToolCatalogue() *ToolCatalogue {
	return &ToolCatalogue{}
}

// Create a new tool catalogue
func (tc *ToolCatalogue) Create(db *gorm.DB) error {
	return db.Create(tc).Error
}

// Get a tool catalogue by ID
func (tc *ToolCatalogue) Get(db *gorm.DB, id uint) error {
	return db.Preload("Tools").Preload("Tags").First(tc, id).Error
}

// Update an existing tool catalogue
func (tc *ToolCatalogue) Update(db *gorm.DB) error {
	return db.Save(tc).Error
}

// Delete a tool catalogue
func (tc *ToolCatalogue) Delete(db *gorm.DB) error {
	return db.Delete(tc).Error
}

// Add a tag to the tool catalogue
func (tc *ToolCatalogue) AddTag(db *gorm.DB, tag *Tag) error {
	return db.Model(tc).Association("Tags").Append(tag)
}

// Remove a tag from the tool catalogue
func (tc *ToolCatalogue) RemoveTag(db *gorm.DB, tag *Tag) error {
	return db.Model(tc).Association("Tags").Delete(tag)
}

// Add a tool to the tool catalogue
func (tc *ToolCatalogue) AddTool(db *gorm.DB, tool *Tool) error {
	return db.Model(tc).Association("Tools").Append(tool)
}

// Remove a tool from the tool catalogue
func (tc *ToolCatalogue) RemoveTool(db *gorm.DB, tool *Tool) error {
	return db.Model(tc).Association("Tools").Delete(tool)
}

// Get all tool catalogues
func (tc *ToolCatalogues) GetAll(db *gorm.DB) error {
	return db.Preload("Tools").Preload("Tags").Find(tc).Error
}

// Search tool catalogues by name, short description, and long description
func (tc *ToolCatalogues) Search(db *gorm.DB, query string) error {
	return db.Preload("Tools").Preload("Tags").
		Where("name LIKE ? OR short_description LIKE ? OR long_description LIKE ?",
			"%"+query+"%", "%"+query+"%", "%"+query+"%").
		Find(tc).Error
}

// Get tool catalogues by tag
func (tc *ToolCatalogues) GetByTag(db *gorm.DB, tagName string) error {
	return db.Preload("Tools").Preload("Tags").
		Joins("JOIN tool_catalogue_tags ON tool_catalogue_tags.tool_catalogue_id = tool_catalogues.id").
		Joins("JOIN tags ON tags.id = tool_catalogue_tags.tag_id").
		Where("tags.name = ?", tagName).
		Find(tc).Error
}

// Get tool catalogues by tool
func (tc *ToolCatalogues) GetByTool(db *gorm.DB, toolID uint) error {
	return db.Preload("Tools").Preload("Tags").
		Joins("JOIN tool_catalogue_tools ON tool_catalogue_tools.tool_catalogue_id = tool_catalogues.id").
		Where("tool_catalogue_tools.tool_id = ?", toolID).
		Find(tc).Error
}

package models

import "gorm.io/gorm"

type Chat struct {
	gorm.Model
	ID            uint         `gorm:"primaryKey" json:"id"`
	Name          string       `json:"name"`
	Groups        []Group      `gorm:"many2many:chat_groups;"`
	LLMSettingsID uint         `json:"llm_settings_id"`
	LLMSettings   *LLMSettings `gorm:"foreignKey:LLMSettingsID" json:"llm_settings"`
	LLMID         uint         `json:"llm_id"`
	LLM           *LLM         `gorm:"foreignKey:LLMID" json:"llm"`
}

type Chats []Chat

// Create a new chat
func (c *Chat) Create(db *gorm.DB) error {
	return db.Create(c).Error
}

// Get a chat by ID
func (c *Chat) Get(db *gorm.DB, id uint) error {
	return db.Preload("Groups").Preload("LLMSettings").Preload("LLM").First(c, id).Error
}

// Update an existing chat
func (c *Chat) Update(db *gorm.DB) error {
	return db.Save(c).Error
}

// Delete a chat
func (c *Chat) Delete(db *gorm.DB) error {
	return db.Delete(c).Error
}

// List all chats
func (cs *Chats) List(db *gorm.DB) error {
	return db.Preload("Groups").Preload("LLMSettings").Preload("LLM").Find(cs).Error
}

// Get chats by group ID
func (cs *Chats) GetByGroupID(db *gorm.DB, groupID uint) error {
	return db.Preload("Groups").Preload("LLMSettings").Preload("LLM").
		Joins("JOIN chat_groups ON chat_groups.chat_id = chats.id").
		Where("chat_groups.group_id = ?", groupID).
		Find(cs).Error
}

// Get chats by LLM ID
func (cs *Chats) GetByLLMID(db *gorm.DB, llmID uint) error {
	return db.Preload("Groups").Preload("LLMSettings").Preload("LLM").
		Where("llm_id = ?", llmID).
		Find(cs).Error
}

// Get chats by LLMSettings ID
func (cs *Chats) GetByLLMSettingsID(db *gorm.DB, llmSettingsID uint) error {
	return db.Preload("Groups").Preload("LLMSettings").Preload("LLM").
		Where("llm_settings_id = ?", llmSettingsID).
		Find(cs).Error
}

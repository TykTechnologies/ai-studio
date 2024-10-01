package models

import "gorm.io/gorm"

type Chat struct {
	gorm.Model
	ID                  uint         `gorm:"primaryKey" json:"id"`
	Name                string       `json:"name"`
	Groups              []Group      `gorm:"many2many:chat_groups;"`
	LLMSettingsID       uint         `json:"llm_settings_id"`
	LLMSettings         *LLMSettings `gorm:"foreignKey:LLMSettingsID" json:"llm_settings"`
	LLMID               uint         `json:"llm_id"`
	LLM                 *LLM         `gorm:"foreignKey:LLMID" json:"llm"`
	Filters             []*Filter    `gorm:"many2many:chat_filters;"`
	RagResultsPerSource int          `json:"rag_results_per_source"`
	SupportsTools       bool         `json:"supports_tools"`
}

type Chats []Chat

// Create a new chat
func (c *Chat) Create(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Create the chat
		if err := tx.Create(c).Error; err != nil {
			return err
		}

		// Handle Groups association
		if len(c.Groups) > 0 {
			if err := tx.Model(c).Association("Groups").Replace(c.Groups); err != nil {
				return err
			}
		}

		// Handle Filters association
		if len(c.Filters) > 0 {
			if err := tx.Model(c).Association("Filters").Replace(c.Filters); err != nil {
				return err
			}
		}

		return nil
	})
}

// Get a chat by ID
func (c *Chat) Get(db *gorm.DB, id uint) error {
	return db.Preload("Groups").Preload("LLMSettings").Preload("LLM").Preload("Filters").First(c, id).Error
}

// Update an existing chat
func (c *Chat) Update(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Update the chat's fields
		if err := tx.Model(c).Updates(Chat{
			Name:          c.Name,
			LLMSettingsID: c.LLMSettingsID,
			LLMID:         c.LLMID,
		}).Error; err != nil {
			return err
		}

		// Handle Groups association
		if err := tx.Model(c).Association("Groups").Replace(c.Groups); err != nil {
			return err
		}

		// Handle Filters association
		if err := tx.Model(c).Association("Filters").Replace(c.Filters); err != nil {
			return err
		}

		return nil
	})
}

// Delete a chat
func (c *Chat) Delete(db *gorm.DB) error {
	return db.Delete(c).Error
}

// List all chats
func (cs *Chats) List(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&Chat{})

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

	err := query.Preload("Groups").Preload("LLMSettings").Preload("LLM").Find(cs).Error
	return totalCount, totalPages, err
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

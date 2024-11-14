package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// CreateChat creates a new chat
func (s *Service) CreateChat(name string, llmSettingsID, llmID uint, groupIDs []uint, filterIDs []uint, ragN int, toolSupport bool, systemPrompt string) (*models.Chat, error) {
	chat := &models.Chat{
		Name:                name,
		LLMSettingsID:       llmSettingsID,
		LLMID:               llmID,
		RagResultsPerSource: ragN,
		SupportsTools:       toolSupport,
		SystemPrompt:        systemPrompt,
	}

	for _, filterID := range filterIDs {
		filter := &models.Filter{}
		if err := filter.Get(s.DB, filterID); err != nil {
			return nil, err
		}
		chat.Filters = append(chat.Filters, filter)
	}

	// Fetch the groups
	var groups []models.Group
	if err := s.DB.Where("id IN ?", groupIDs).Find(&groups).Error; err != nil {
		return nil, err
	}

	if len(groups) == 0 {
		return nil, fmt.Errorf("no groups found with the provided IDs")
	}
	chat.Groups = groups

	if err := chat.Create(s.DB); err != nil {
		return nil, err
	}

	return chat, nil
}

// GetChatByID retrieves a chat by its ID
func (s *Service) GetChatByID(id uint) (*models.Chat, error) {
	chat := &models.Chat{}
	if err := chat.Get(s.DB, id); err != nil {
		return nil, err
	}

	return chat, nil
}

// UpdateChat updates an existing chat
func (s *Service) UpdateChat(id uint, name string, llmSettingsID, llmID uint, groupIDs []uint, filterIDs []uint, ragN int, toolSupport bool, systemPrompt string) (*models.Chat, error) {
	// Start a transaction
	tx := s.DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Get the chat within the transaction
	chat := &models.Chat{}
	if err := tx.First(chat, id).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Update chat fields
	updates := map[string]interface{}{
		"name":                   name,
		"llm_settings_id":        llmSettingsID,
		"llm_id":                 llmID,
		"rag_results_per_source": ragN,
		"supports_tools":         toolSupport,
		"system_prompt":          systemPrompt,
	}

	// Update the chat's basic information
	if err := tx.Model(chat).Updates(updates).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Clear existing associations
	if err := tx.Model(chat).Association("Groups").Clear(); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Model(chat).Association("Filters").Clear(); err != nil {
		tx.Rollback()
		return nil, err
	}

	// Add new group associations
	if len(groupIDs) > 0 {
		var groups []models.Group
		if err := tx.Where("id IN ?", groupIDs).Find(&groups).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := tx.Model(chat).Association("Groups").Append(&groups); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	// Add new filter associations
	if len(filterIDs) > 0 {
		var filters []*models.Filter
		if err := tx.Where("id IN ?", filterIDs).Find(&filters).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := tx.Model(chat).Association("Filters").Append(&filters); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	// Reload the chat with all associations
	updatedChat := &models.Chat{}
	if err := s.DB.Preload("Groups").
		Preload("Filters").
		Preload("LLMSettings").
		Preload("LLM").
		First(updatedChat, id).Error; err != nil {
		return nil, err
	}

	return updatedChat, nil
}

// DeleteChat deletes a chat by its ID
func (s *Service) DeleteChat(id uint) error {
	chat, err := s.GetChatByID(id)
	if err != nil {
		return err
	}
	return chat.Delete(s.DB)
}

// ListChats retrieves all chats
func (s *Service) ListChats(pageSize int, pageNumber int, all bool) (models.Chats, int64, int, error) {
	var chats models.Chats
	totalCount, totalPages, err := chats.List(s.DB, pageSize, pageNumber, all)
	if err != nil {
		return nil, 0, 0, err
	}
	return chats, totalCount, totalPages, nil
}

// GetChatsByGroupID retrieves all chats associated with a specific group
func (s *Service) GetChatsByGroupID(groupID uint) (models.Chats, error) {
	var chats models.Chats
	if err := chats.GetByGroupID(s.DB, groupID); err != nil {
		return nil, err
	}
	return chats, nil
}

// GetChatsByLLMID retrieves all chats associated with a specific LLM
func (s *Service) GetChatsByLLMID(llmID uint) (models.Chats, error) {
	var chats models.Chats
	if err := chats.GetByLLMID(s.DB, llmID); err != nil {
		return nil, err
	}
	return chats, nil
}

// GetChatsByLLMSettingsID retrieves all chats associated with specific LLM settings
func (s *Service) GetChatsByLLMSettingsID(llmSettingsID uint) (models.Chats, error) {
	var chats models.Chats
	if err := chats.GetByLLMSettingsID(s.DB, llmSettingsID); err != nil {
		return nil, err
	}
	return chats, nil
}

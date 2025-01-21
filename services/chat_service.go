package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// CreateChat creates a new chat
func (s *Service) CreateChat(name string, llmSettingsID, llmID uint, groupIDs []uint,
	filterIDs []uint, ragN int, toolSupport bool, systemPrompt string, defaultDSID uint, defaultTools []uint) (*models.Chat, error) {

	chat := &models.Chat{
		Name:                name,
		LLMSettingsID:       llmSettingsID,
		LLMID:               llmID,
		RagResultsPerSource: ragN,
		SupportsTools:       toolSupport,
		SystemPrompt:        systemPrompt,
		DefaultDataSourceID: nil,
	}

	if defaultDSID != 0 {
		chat.DefaultDataSourceID = &defaultDSID
	}

	for _, filterID := range filterIDs {
		filter := &models.Filter{}
		if err := filter.Get(s.DB, filterID); err != nil {
			return nil, err
		}
		chat.Filters = append(chat.Filters, filter)
	}

	for _, toolID := range defaultTools {
		tool := &models.Tool{}
		if err := tool.Get(s.DB, toolID); err != nil {
			return nil, err
		}
		chat.DefaultTools = append(chat.DefaultTools, tool)
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

	fmt.Println("getting chat by ID")
	fmt.Println(chat.LLM.APIKey)

	chat.LLM.APIKey = secrets.GetValue(chat.LLM.APIKey)
	fmt.Println("AFTER")
	fmt.Println(chat.LLM.APIKey)
	for i := range chat.DefaultTools {
		chat.DefaultTools[i].AuthKey = secrets.GetValue(chat.DefaultTools[i].AuthKey)
	}

	if chat.DefaultDataSource != nil {
		chat.DefaultDataSource.DBConnAPIKey = secrets.GetValue(chat.DefaultDataSource.DBConnAPIKey)
		chat.DefaultDataSource.EmbedAPIKey = secrets.GetValue(chat.DefaultDataSource.EmbedAPIKey)
	}

	return chat, nil
}

// UpdateChat updates an existing chat
func (s *Service) UpdateChat(id uint, name string, llmSettingsID, llmID uint, groupIDs []uint,
	filterIDs []uint, ragN int, toolSupport bool, systemPrompt string, defaultDSID uint, defaultToolIDs []uint) (*models.Chat, error) {
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
		"default_data_source_id": nil,
	}

	if defaultDSID != 0 {
		updates["default_data_source_id"] = &defaultDSID
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

	if err := tx.Model(chat).Association("DefaultTools").Clear(); err != nil {
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

	if len(defaultToolIDs) > 0 {
		var tools []*models.Tool
		if err := tx.Where("id IN ?", defaultToolIDs).Find(&tools).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := tx.Model(chat).Association("DefaultTools").Append(&tools); err != nil {
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
		Preload("DefaultTools").
		Preload("LLMSettings").
		Preload("LLM").
		Preload("DefaultDataSource").
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

// AddExtraContextToChat adds a ExtraContext to a Chat
func (s *Service) AddExtraContextToChat(toolID uint, fileStoreID uint) error {
	chat, err := s.GetChatByID(toolID)
	if err != nil {
		return err
	}

	fileStore := &models.FileStore{}
	if err := fileStore.Get(s.DB, fileStoreID); err != nil {
		return err
	}

	return chat.AddExtraContext(s.DB, fileStore)
}

// RemoveExtraContextFromChat removes a ExtraContext from a Chat
func (s *Service) RemoveExtraContextFromChat(toolID uint, fileStoreID uint) error {
	chat, err := s.GetChatByID(toolID)
	if err != nil {
		return err
	}

	fileStore := &models.FileStore{}
	if err := fileStore.Get(s.DB, fileStoreID); err != nil {
		return err
	}

	return chat.RemoveExtraContext(s.DB, fileStore)
}

// GetChatExtraContexts gets all ExtraContexts associated with a Chat
func (s *Service) GetChatExtraContexts(toolID uint) ([]models.FileStore, error) {
	chat, err := s.GetChatByID(toolID)
	if err != nil {
		return nil, err
	}

	return chat.GetExtraContext(s.DB)
}

// SetChatExtraContexts replaces all existing ExtraContext associations with new ones
func (s *Service) SetChatExtraContexts(toolID uint, fileStoreIDs []uint) error {
	chat, err := s.GetChatByID(toolID)
	if err != nil {
		return err
	}

	fileStores := make([]models.FileStore, len(fileStoreIDs))
	for i, id := range fileStoreIDs {
		fileStore := models.FileStore{}
		if err := fileStore.Get(s.DB, id); err != nil {
			return err
		}
		fileStores[i] = fileStore
	}

	return chat.SetExtraContext(s.DB, fileStores)
}

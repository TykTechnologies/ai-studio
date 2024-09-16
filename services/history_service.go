package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

// CreateChatHistoryRecord creates a new ChatHistoryRecord
func (s *Service) CreateChatHistoryRecord(sessionID string, chatID, userID uint, name string) (*models.ChatHistoryRecord, error) {
	record := &models.ChatHistoryRecord{
		SessionID: sessionID,
		ChatID:    chatID,
		UserID:    userID,
		Name:      name,
	}

	if err := record.Create(s.DB); err != nil {
		return nil, err
	}

	return record, nil
}

// GetChatHistoryRecordByID retrieves a ChatHistoryRecord by its ID
func (s *Service) GetChatHistoryRecordByID(id uint) (*models.ChatHistoryRecord, error) {
	record := &models.ChatHistoryRecord{}
	if err := record.Get(s.DB, id); err != nil {
		return nil, err
	}
	return record, nil
}

// UpdateChatHistoryRecord updates an existing ChatHistoryRecord
func (s *Service) UpdateChatHistoryRecord(id uint, sessionID string, chatID, userID uint, name string) (*models.ChatHistoryRecord, error) {
	record, err := s.GetChatHistoryRecordByID(id)
	if err != nil {
		return nil, err
	}

	record.SessionID = sessionID
	record.ChatID = chatID
	record.UserID = userID
	record.Name = name

	if err := record.Update(s.DB); err != nil {
		return nil, err
	}

	return record, nil
}

// DeleteChatHistoryRecord deletes a ChatHistoryRecord by its ID
func (s *Service) DeleteChatHistoryRecord(id uint) error {
	record, err := s.GetChatHistoryRecordByID(id)
	if err != nil {
		return err
	}

	return record.Delete(s.DB)
}

// GetChatHistoryRecordBySessionID retrieves a ChatHistoryRecord by its SessionID
func (s *Service) GetChatHistoryRecordBySessionID(sessionID string) (*models.ChatHistoryRecord, error) {
	record := &models.ChatHistoryRecord{}
	if err := record.GetBySessionID(s.DB, sessionID); err != nil {
		return nil, err
	}
	return record, nil
}

// GetChatHistoryRecordByChatID retrieves a ChatHistoryRecord by its ChatID
func (s *Service) GetChatHistoryRecordByChatID(chatID uint) (*models.ChatHistoryRecord, error) {
	record := &models.ChatHistoryRecord{}
	if err := record.GetByChatID(s.DB, chatID); err != nil {
		return nil, err
	}
	return record, nil
}

// ListChatHistoryRecordsByUserID retrieves all ChatHistoryRecords for a given UserID
func (s *Service) ListChatHistoryRecordsByUserID(userID uint) ([]models.ChatHistoryRecord, error) {
	return models.ListChatHistoryRecordsByUserID(s.DB, userID)
}

// ListChatHistoryRecordsByUserIDPaginated retrieves ChatHistoryRecords for a given UserID with pagination
func (s *Service) ListChatHistoryRecordsByUserIDPaginated(userID uint, page, pageSize int) ([]models.ChatHistoryRecord, int64, error) {
	return models.ListChatHistoryRecordsByUserIDPaginated(s.DB, userID, page, pageSize)
}

// SearchChatHistoryRecords searches for ChatHistoryRecords by name for a given UserID
func (s *Service) SearchChatHistoryRecords(userID uint, query string) ([]models.ChatHistoryRecord, error) {
	return models.SearchChatHistoryRecords(s.DB, userID, query)
}

// GetLatestChatHistoryRecord retrieves the most recent ChatHistoryRecord for a given UserID
func (s *Service) GetLatestChatHistoryRecord(userID uint) (*models.ChatHistoryRecord, error) {
	return models.GetLatestChatHistoryRecord(s.DB, userID)
}

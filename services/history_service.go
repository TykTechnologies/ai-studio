package services

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
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
func (s *Service) ListChatHistoryRecordsByUserID(userID uint, pageSize int, pageNumber int, all bool) ([]models.ChatHistoryRecord, int64, int, error) {
	return models.ListChatHistoryRecordsByUserID(s.DB, userID, pageSize, pageNumber, all)
}

// ListChatHistoryRecordsByUserIDPaginated retrieves ChatHistoryRecords for a given UserID with pagination
func (s *Service) ListChatHistoryRecordsByUserIDPaginated(userID uint, pageSize int, pageNumber int, all bool) ([]models.ChatHistoryRecord, int64, int, error) {
	return models.ListChatHistoryRecordsByUserIDPaginated(s.DB, userID, pageSize, pageNumber, all)
}

// SearchChatHistoryRecords searches for ChatHistoryRecords by name for a given UserID
func (s *Service) SearchChatHistoryRecords(userID uint, query string, pageSize int, pageNumber int, all bool) ([]models.ChatHistoryRecord, int64, int, error) {
	return models.SearchChatHistoryRecords(s.DB, userID, query, pageSize, pageNumber, all)
}

// GetLatestChatHistoryRecord retrieves the most recent ChatHistoryRecord for a given UserID
func (s *Service) GetLatestChatHistoryRecord(userID uint) (*models.ChatHistoryRecord, error) {
	return models.GetLatestChatHistoryRecord(s.DB, userID)
}

// GetLastCMessagesForSession retrieves the last X CMessage records for a given session ID
func (s *Service) GetLastCMessagesForSession(sessionID string, limit int) ([]models.CMessage, error) {
	return models.GetLastCMessagesForSession(s.DB, sessionID, limit)
}

// GetCMessagesForSessionPaginated retrieves CMessage records for a given session ID with pagination
func (s *Service) GetCMessagesForSessionPaginated(sessionID string, pageSize, pageNumber int) ([]models.CMessage, int64, int, error) {
	offset := (pageNumber - 1) * pageSize

	var messages []models.CMessage
	var totalCount int64

	if err := s.DB.Model(&models.CMessage{}).Where("session = ?", sessionID).Count(&totalCount).Error; err != nil {
		return nil, 0, 0, err
	}

	if err := s.DB.Where("session = ?", sessionID).Order("created_at asc").Offset(offset).Limit(pageSize).Find(&messages).Error; err != nil {
		return nil, 0, 0, err
	}

	totalPages := int((totalCount + int64(pageSize) - 1) / int64(pageSize))

	return messages, totalCount, totalPages, nil
}

// EditUserMessage updates the user message in session, then removes all subsequent messages so conversation can be replayed
func (s *Service) EditUserMessage(sessionID string, messageID uint, newContent string) error {
	var msg models.CMessage
	err := s.DB.First(&msg, messageID).Error
	if err != nil {
		slog.Error("Failed to find message", "messageID", messageID, "error", err)
		return fmt.Errorf("failed to find message: %w", err)
	}

	slog.Debug("Found message", "message", msg)

	if msg.Session != sessionID {
		slog.Error("Session mismatch", "messageSession", msg.Session, "requestedSession", sessionID)
		return gorm.ErrRecordNotFound
	}

	// We'll store user messages as {"role":"human","text":"..."} so that the front-end
	// sees it as a user message after reloading
	type userMessage struct {
		Role string `json:"role"`
		Text string `json:"text"`
	}
	updatedMsg := userMessage{
		Role: "human",
		Text: newContent,
	}
	jsonBytes, err := json.Marshal(updatedMsg)
	if err != nil {
		slog.Error("Failed to marshal user message", "error", err)
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	slog.Debug("Updating message content", "content", string(jsonBytes))

	// update the content
	msg.Content = jsonBytes
	if err := s.DB.Save(&msg).Error; err != nil {
		slog.Error("Failed to save updated message", "error", err)
		return fmt.Errorf("failed to save message: %w", err)
	}

	slog.Debug("Successfully updated message, removing subsequent messages")

	// remove subsequent messages in the same session that are created AFTER this message
	result := s.DB.Where("session = ? AND id > ?", sessionID, messageID).Delete(&models.CMessage{})
	if result.Error != nil {
		slog.Error("Failed to delete subsequent messages", "error", result.Error)
		return fmt.Errorf("failed to delete subsequent messages: %w", result.Error)
	}
	slog.Debug("Deleted subsequent messages", "count", result.RowsAffected)

	return nil
}

package models

import (
	"encoding/json"
	"time"

	"github.com/tmc/langchaingo/llms"
	"gorm.io/gorm"
)

// CMessage is the GORM model for chat messages
type CMessage struct {
	gorm.Model
	ID        uint   `gorm:"primaryKey"`
	Session   string `gorm:"index"`
	Content   []byte
	CreatedAt time.Time
	ChatID    uint `gorm:"index"`
}

type ChatHistoryRecord struct {
	gorm.Model
	ID        uint   `gorm:"primaryKey"`
	SessionID string `gorm:"index"`
	ChatID    uint   `gorm:"index"`
	UserID    uint   `gorm:"index"`
	Name      string
}

// Create a new ChatHistoryRecord
func (chr *ChatHistoryRecord) Create(db *gorm.DB) error {
	return db.Create(chr).Error
}

// Get a ChatHistoryRecord by ID
func (chr *ChatHistoryRecord) Get(db *gorm.DB, id uint) error {
	return db.First(chr, id).Error
}

// Update an existing ChatHistoryRecord
func (chr *ChatHistoryRecord) Update(db *gorm.DB) error {
	return db.Save(chr).Error
}

// Delete a ChatHistoryRecord
func (chr *ChatHistoryRecord) Delete(db *gorm.DB) error {
	return db.Delete(chr).Error
}

// GetBySessionID retrieves a ChatHistoryRecord by SessionID
func (chr *ChatHistoryRecord) GetBySessionID(db *gorm.DB, sessionID string) error {
	return db.Where("session_id = ?", sessionID).First(chr).Error
}

// GetByChatID retrieves a ChatHistoryRecord by ChatID
func (chr *ChatHistoryRecord) GetByChatID(db *gorm.DB, chatID uint) error {
	return db.Where("chat_id = ?", chatID).First(chr).Error
}

func (chr *ChatHistoryRecord) UpdateName(db *gorm.DB, newName string) error {
	return db.Model(chr).Update("name", newName).Error
}

func (cm *CMessage) UnmarshalContent() any {
	var messageContent llms.MessageContent

	err := json.Unmarshal(cm.Content, &messageContent)
	if err == nil {
		return messageContent
	}

	return string(cm.Content)
}

// ListByUserID retrieves all ChatHistoryRecords for a given UserID
func ListChatHistoryRecordsByUserID(db *gorm.DB, userID uint, pageSize int, pageNumber int, all bool) ([]ChatHistoryRecord, int64, int, error) {
	var records []ChatHistoryRecord
	var totalCount int64

	// Subquery to get SessionIDs with more than one CMessage
	subQuery := db.Model(&CMessage{}).
		Select("session").
		Group("session").
		Having("COUNT(*) > 1")

	// Main query
	query := db.Model(&ChatHistoryRecord{}).
		Where("user_id = ?", userID).
		Where("session_id IN (?)", subQuery)

	// Count total records
	if err := query.Count(&totalCount).Error; err != nil {
		return nil, 0, 0, err
	}

	// Calculate total pages
	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	// Apply pagination if not retrieving all records
	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	// Execute the query
	err := query.Find(&records).Error
	return records, totalCount, totalPages, err
}

// ListChatHistoryRecordsByUserIDPaginated retrieves ChatHistoryRecords for a given UserID with pagination
func ListChatHistoryRecordsByUserIDPaginated(db *gorm.DB, userID uint, pageSize int, pageNumber int, all bool) ([]ChatHistoryRecord, int64, int, error) {
	var records []ChatHistoryRecord
	var totalCount int64

	// Subquery to get SessionIDs with more than one CMessage
	subQuery := db.Model(&CMessage{}).
		Select("session").
		Group("session").
		Having("COUNT(*) > 1")

	// Main query
	query := db.Model(&ChatHistoryRecord{}).
		Where("user_id = ?", userID).
		Where("session_id IN (?)", subQuery).
		Order("created_at DESC")

	// Count total records
	if err := query.Count(&totalCount).Error; err != nil {
		return nil, 0, 0, err
	}

	// Calculate total pages
	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	// Apply pagination if not retrieving all records
	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	// Execute the query
	err := query.Find(&records).Error
	return records, totalCount, totalPages, err
}

// SearchChatHistoryRecords searches for ChatHistoryRecords by name for a given UserID with pagination
func SearchChatHistoryRecords(db *gorm.DB, userID uint, query string, pageSize int, pageNumber int, all bool) ([]ChatHistoryRecord, int64, int, error) {
	var records []ChatHistoryRecord
	var totalCount int64
	searchQuery := db.Model(&ChatHistoryRecord{}).Where("user_id = ? AND name LIKE ?", userID, "%"+query+"%")

	if err := searchQuery.Count(&totalCount).Error; err != nil {
		return nil, 0, 0, err
	}

	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	if !all {
		offset := (pageNumber - 1) * pageSize
		searchQuery = searchQuery.Offset(offset).Limit(pageSize)
	}

	err := searchQuery.Find(&records).Error
	return records, totalCount, totalPages, err
}

// GetLatestChatHistoryRecord retrieves the most recent ChatHistoryRecord for a given UserID
func GetLatestChatHistoryRecord(db *gorm.DB, userID uint) (*ChatHistoryRecord, error) {
	var record ChatHistoryRecord
	err := db.Where("user_id = ?", userID).Order("created_at DESC").First(&record).Error
	return &record, err
}

func GetLastCMessagesForSession(db *gorm.DB, sessionID string, limit int) ([]CMessage, error) {
	var messages []CMessage

	err := db.Where("session = ?", sessionID).
		Order("created_at ASC"). // Order by creation time ascending (oldest first)
		Limit(limit).
		Find(&messages).Error

	if err != nil {
		return nil, err
	}

	return messages, nil
}

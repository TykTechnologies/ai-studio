package models

import (
	"time"

	"gorm.io/gorm"
)

// CMessage is the GORM model for chat messages
type CMessage struct {
	gorm.Model
	ID        uint   `gorm:"primaryKey"`
	Session   string `gorm:"index"`
	Content   []byte
	CreatedAt time.Time
	ChatID    *uint `gorm:"index"`
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

// ListByUserID retrieves all ChatHistoryRecords for a given UserID
func ListChatHistoryRecordsByUserID(db *gorm.DB, userID uint) ([]ChatHistoryRecord, error) {
	var records []ChatHistoryRecord
	err := db.Where("user_id = ?", userID).Find(&records).Error
	return records, err
}

// ListByUserIDPaginated retrieves ChatHistoryRecords for a given UserID with pagination
func ListChatHistoryRecordsByUserIDPaginated(db *gorm.DB, userID uint, page, pageSize int) ([]ChatHistoryRecord, int64, error) {
	var records []ChatHistoryRecord
	var total int64

	offset := (page - 1) * pageSize

	err := db.Model(&ChatHistoryRecord{}).Where("user_id = ?", userID).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = db.Where("user_id = ?", userID).Offset(offset).Limit(pageSize).Find(&records).Error
	return records, total, err
}

// SearchChatHistoryRecords searches for ChatHistoryRecords by name for a given UserID
func SearchChatHistoryRecords(db *gorm.DB, userID uint, query string) ([]ChatHistoryRecord, error) {
	var records []ChatHistoryRecord
	err := db.Where("user_id = ? AND name LIKE ?", userID, "%"+query+"%").Find(&records).Error
	return records, err
}

// GetLatestChatHistoryRecord retrieves the most recent ChatHistoryRecord for a given UserID
func GetLatestChatHistoryRecord(db *gorm.DB, userID uint) (*ChatHistoryRecord, error) {
	var record ChatHistoryRecord
	err := db.Where("user_id = ?", userID).Order("created_at DESC").First(&record).Error
	return &record, err
}

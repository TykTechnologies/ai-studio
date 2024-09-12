package chat_session

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
	"gorm.io/gorm"
)

// GormChatMessageHistory is a struct that stores chat messages using GORM
type GormChatMessageHistory struct {
	DB      *gorm.DB
	Limit   int
	Session string
}

// Ensure GormChatMessageHistory implements the ChatMessageHistory interface
var _ schema.ChatMessageHistory = &GormChatMessageHistory{}

// GormChatMessageHistoryOption is a function type for configuring GormChatMessageHistory
type GormChatMessageHistoryOption func(*GormChatMessageHistory)

// WithLimit sets the limit for the number of messages to retrieve
func WithLimit(limit int) GormChatMessageHistoryOption {
	return func(h *GormChatMessageHistory) {
		h.Limit = limit
	}
}

// NewGormChatMessageHistory creates a new GormChatMessageHistory
func NewGormChatMessageHistory(db *gorm.DB, session string, options ...GormChatMessageHistoryOption) *GormChatMessageHistory {
	h := &GormChatMessageHistory{
		DB:      db,
		Limit:   100, // Default limit
		Session: session,
	}

	for _, option := range options {
		option(h)
	}

	return h
}

func (h *GormChatMessageHistory) GetMemoryKey(context.Context) string {
	return "history"
}

// Messages returns all messages stored
func (h *GormChatMessageHistory) Messages(ctx context.Context) ([]llms.ChatMessage, error) {
	var chatMessages []models.CMessage
	result := h.DB.WithContext(ctx).
		Where("session = ?", h.Session).
		Order("created_at ASC").
		Limit(h.Limit).
		Find(&chatMessages)

	if result.Error != nil {
		return nil, result.Error
	}

	var messages []llms.ChatMessage
	for _, msg := range chatMessages {
		switch msg.Type {
		case string(llms.ChatMessageTypeAI):
			messages = append(messages, llms.AIChatMessage{Content: msg.Content})
		case string(llms.ChatMessageTypeHuman):
			messages = append(messages, llms.HumanChatMessage{Content: msg.Content})
		case string(llms.ChatMessageTypeSystem):
			messages = append(messages, llms.SystemChatMessage{Content: msg.Content})
		}
	}

	return messages, nil
}

func (h *GormChatMessageHistory) addMessage(ctx context.Context, text string, role llms.ChatMessageType) error {
	message := models.CMessage{
		Session: h.Session,
		Content: text,
		Type:    string(role),
	}
	return h.DB.WithContext(ctx).Create(&message).Error
}

// AddMessage adds a message to the chat message history
func (h *GormChatMessageHistory) AddMessage(ctx context.Context, message llms.ChatMessage) error {
	return h.addMessage(ctx, message.GetContent(), message.GetType())
}

// AddAIMessage adds an AIMessage to the chat message history
func (h *GormChatMessageHistory) AddAIMessage(ctx context.Context, text string) error {
	return h.addMessage(ctx, text, llms.ChatMessageTypeAI)
}

// AddUserMessage adds a user message to the chat message history
func (h *GormChatMessageHistory) AddUserMessage(ctx context.Context, text string) error {
	return h.addMessage(ctx, text, llms.ChatMessageTypeHuman)
}

// Clear resets messages
func (h *GormChatMessageHistory) Clear(ctx context.Context) error {
	return h.DB.WithContext(ctx).Where("session = ?", h.Session).Delete(&models.CMessage{}).Error
}

// SetMessages resets chat history and bulk inserts new messages into it
func (h *GormChatMessageHistory) SetMessages(ctx context.Context, messages []llms.ChatMessage) error {
	err := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session = ?", h.Session).Delete(&models.CMessage{}).Error; err != nil {
			return err
		}

		for _, msg := range messages {
			chatMessage := models.CMessage{
				Session: h.Session,
				Content: msg.GetContent(),
				Type:    string(msg.GetType()),
			}
			if err := tx.Create(&chatMessage).Error; err != nil {
				return err
			}
		}
		return nil
	})

	return err
}

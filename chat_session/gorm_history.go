package chat_session

import (
	"context"
	"encoding/json"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/tmc/langchaingo/llms"
	"gorm.io/gorm"
)

// GormChatMessageHistory is a struct that stores chat messages using GORM
type GormChatMessageHistory struct {
	DB      *gorm.DB
	Limit   int
	Session string
}

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
func (h *GormChatMessageHistory) Messages(ctx context.Context) ([]llms.MessageContent, error) {
	var chatMessages []models.CMessage
	result := h.DB.WithContext(ctx).
		Where("session = ?", h.Session).
		Order("created_at ASC").
		Limit(h.Limit).
		Find(&chatMessages)

	if result.Error != nil {
		return nil, result.Error
	}

	var messages []llms.MessageContent
	for _, msg := range chatMessages {
		mc := llms.MessageContent{}
		err := json.Unmarshal([]byte(msg.Content), &mc)
		if err != nil {
			return nil, err
		}

		messages = append(messages, mc)
	}

	return messages, nil
}

func (c *GormChatMessageHistory) AddMessage(ctx context.Context, mc llms.MessageContent) error {
	return c.addMessage(ctx, mc)
}

func (h *GormChatMessageHistory) addMessage(ctx context.Context, mc llms.MessageContent) error {
	asJson, err := json.Marshal(mc)
	if err != nil {
		return err
	}
	message := models.CMessage{
		Session: h.Session,
		Content: asJson,
	}
	return h.DB.WithContext(ctx).Create(&message).Error
}

// AddAIMessage adds an AIMessage to the chat message history
func (h *GormChatMessageHistory) AddAIMessage(ctx context.Context, text string) error {
	mc := llms.TextParts(llms.ChatMessageTypeAI, text)
	return h.addMessage(ctx, mc)
}

// AddUserMessage adds a user message to the chat message history
func (h *GormChatMessageHistory) AddUserMessage(ctx context.Context, text string) error {
	mc := llms.TextParts(llms.ChatMessageTypeHuman, text)
	return h.addMessage(ctx, mc)
}

// AddUserMessage adds a user message to the chat message history
func (h *GormChatMessageHistory) AddToolMessage(ctx context.Context, toolResp llms.ToolCallResponse) error {
	mc := llms.MessageContent{
		Role:  llms.ChatMessageTypeTool,
		Parts: []llms.ContentPart{toolResp},
	}

	return h.addMessage(ctx, mc)
}

func (h *GormChatMessageHistory) AddAIToolCall(ctx context.Context, toolCall llms.ToolCall) error {
	mc := llms.MessageContent{
		Role:  llms.ChatMessageTypeAI,
		Parts: []llms.ContentPart{toolCall},
	}

	return h.addMessage(ctx, mc)
}

// Clear resets messages
func (h *GormChatMessageHistory) Clear(ctx context.Context) error {
	return h.DB.WithContext(ctx).Where("session = ?", h.Session).Delete(&models.CMessage{}).Error
}

// SetMessages resets chat history and bulk inserts new messages into it
func (h *GormChatMessageHistory) SetMessages(ctx context.Context, messages []llms.MessageContent) error {
	err := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session = ?", h.Session).Delete(&models.CMessage{}).Error; err != nil {
			return err
		}

		for _, msg := range messages {
			asJson, err := json.Marshal(msg)
			if err != nil {
				return err
			}

			chatMessage := models.CMessage{
				Session: h.Session,
				Content: asJson,
			}
			if err := tx.Create(&chatMessage).Error; err != nil {
				return err
			}
		}
		return nil
	})

	return err
}

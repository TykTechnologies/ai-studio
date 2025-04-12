package chat_session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/tmc/langchaingo/llms"
	"gorm.io/gorm"
)

// GormChatMessageHistory is a struct that stores chat messages using GORM
type GormChatMessageHistory struct {
	DB      *gorm.DB
	Limit   int
	Session string
	ChatID  uint
	UserID  uint
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
func NewGormChatMessageHistory(db *gorm.DB, session string, chatReference uint, userID uint, systemPrompt string, options ...GormChatMessageHistoryOption) *GormChatMessageHistory {
	h := &GormChatMessageHistory{
		DB:      db,
		Limit:   100, // Default limit
		Session: session,
		ChatID:  chatReference,
		UserID:  userID,
	}

	for _, option := range options {
		option(h)
	}

	sessionRecordExists, first, err := h.CheckIfSessionExists(context.Background())
	if err != nil {
		slog.Error("failed to check if session exists", "error", err)
		sessionRecordExists = false
	}

	if sessionRecordExists {
		h.ChatID = first.ChatID
	}

	if systemPrompt != "" {
		err := h.AddSystemMessage(context.Background(), systemPrompt)
		if err != nil {
			slog.Error("failed to add system prompt", "error", err)
		}
	}

	return h
}

func (h *GormChatMessageHistory) GetMemoryKey(context.Context) string {
	return "history"
}

func (h *GormChatMessageHistory) CheckIfSessionExists(ctx context.Context) (bool, *models.ChatHistoryRecord, error) {
	var record models.ChatHistoryRecord
	result := h.DB.WithContext(ctx).
		Where("session_id = ?", h.Session).
		First(&record)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return false, nil, nil
		}
		return false, nil, result.Error
	}

	return true, &record, nil
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
		ChatID:  h.ChatID,
	}
	return h.DB.WithContext(ctx).Create(&message).Error
}

// AddAIMessage adds an AIMessage to the chat message history
func (h *GormChatMessageHistory) AddAIMessage(ctx context.Context, text string) error {
	var mc llms.MessageContent
	// Try to parse the text as JSON first to see if it's a combined message
	err := json.Unmarshal([]byte(text), &mc)
	if err == nil && mc.Role == llms.ChatMessageTypeAI {
		// If it's already a valid MessageContent with AI role, use it directly
		return h.addMessage(ctx, mc)
	}

	// If it's not a valid JSON or not an AI message, create a new text message
	mc = llms.TextParts(llms.ChatMessageTypeAI, text)
	return h.addMessage(ctx, mc)
}

// AddUserMessage adds a user message to the chat message history
func (h *GormChatMessageHistory) AddUserMessage(ctx context.Context, text string) error {
	// Check if this is the first message and if a session record exists
	sessionRecordExists, _, err := h.CheckIfSessionExists(ctx)
	if err != nil {
		slog.Error("failed to check if session exists", "error", err)
	}

	// If no session record exists, create one before adding the message
	if !sessionRecordExists {
		uid := 0
		if h.UserID != 0 {
			uid = int(h.UserID)
		}

		cid := 0
		if h.ChatID != 0 {
			cid = int(h.ChatID)
		}

		// Create a record of this Chat Session
		chr := &models.ChatHistoryRecord{
			SessionID: h.Session,
			ChatID:    uint(cid),
			UserID:    uint(uid),
			Name:      time.Now().Format("3PM on Monday (02/01/06)"),
			Hidden:    false,
		}

		err := h.DB.Create(chr).Error
		if err != nil {
			slog.Error("failed to create chat history record", "error", err)
		}
	}

	mc := llms.TextParts(llms.ChatMessageTypeHuman, text)
	return h.addMessage(ctx, mc)
}

// AddSystemMessage adds a system message to the chat message history
func (h *GormChatMessageHistory) AddSystemMessage(ctx context.Context, text string) error {
	mc := llms.TextParts(llms.ChatMessageTypeSystem, text)
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

func (h *GormChatMessageHistory) GetAssociatedChat(ctx context.Context) (*models.Chat, error) {
	if h.ChatID == 0 {
		return nil, fmt.Errorf("no associated chat for this session")
	}

	chat := &models.Chat{}
	err := chat.Get(h.DB, h.ChatID)
	if err != nil {
		return nil, err
	}

	return chat, nil
}

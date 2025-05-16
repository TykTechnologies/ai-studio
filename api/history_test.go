package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

func TestChatHistoryRecordEndpoints(t *testing.T) {
	t.Skip()
	api, db := setupTestAPI(t)

	group, err := api.service.CreateGroup("Test Group", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)

	// Create a test user
	user, err := api.service.CreateUser("test@example.com", "Test User", "password123", false, true, true, true, false, false)
	assert.NoError(t, err)

	// Create a test chat
	chat, err := api.service.CreateChat("Test Chat", "Test Chat Description", 1, 1, []uint{group.ID}, nil, 1, false, "Test System Prompt", 1, nil)
	assert.NoError(t, err)

	// Test Create ChatHistoryRecord
	createChatHistoryRecordInput := ChatHistoryRecordInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				SessionID string `json:"session_id"`
				ChatID    uint   `json:"chat_id"`
				UserID    uint   `json:"user_id"`
				Name      string `json:"name"`
			} `json:"attributes"`
		}{
			Type: "chat_history_records",
			Attributes: struct {
				SessionID string `json:"session_id"`
				ChatID    uint   `json:"chat_id"`
				UserID    uint   `json:"user_id"`
				Name      string `json:"name"`
			}{
				SessionID: "test-session",
				ChatID:    chat.ID,
				UserID:    user.ID,
				Name:      "Test Chat History",
			},
		},
	}

	// Chat history needs more than 1 message to be created
	for j := 1; j <= 5; j++ {
		message := &models.CMessage{
			Session:   "test-session",
			Content:   []byte("Test Message "),
			ChatID:    chat.ID,
			CreatedAt: time.Now(),
		}
		err := db.Create(message).Error
		assert.NoError(t, err)
	}

	w := performRequest(api.router, "POST", "/api/v1/chat-history-records", createChatHistoryRecordInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response ChatHistoryRecordResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "test-session", response.Attributes.SessionID)
	assert.Equal(t, "Test Chat History", response.Attributes.Name)

	recordID := response.ID

	// Test Get ChatHistoryRecord
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/chat-history-records/%s", recordID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var getResponse ChatHistoryRecordResponse
	err = json.Unmarshal(w.Body.Bytes(), &getResponse)
	assert.NoError(t, err)
	assert.Equal(t, "test-session", getResponse.Attributes.SessionID)

	// Test List ChatHistoryRecords
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/chat-history-records?user_id=%d", user.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse ChatHistoryRecordListResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Len(t, listResponse.Data, 1)
	if len(listResponse.Data) > 0 {
		assert.Equal(t, "Test Chat History", listResponse.Data[0].Attributes.Name)
	}

	// Test Delete ChatHistoryRecord
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/chat-history-records/%s", recordID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify ChatHistoryRecord is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/chat-history-records/%s", recordID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChatHistoryRecordEndpointsErrors(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Get non-existent ChatHistoryRecord
	w := performRequest(api.router, "GET", "/api/v1/chat-history-records/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Create ChatHistoryRecord with invalid input
	invalidCreateInput := ChatHistoryRecordInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				SessionID string `json:"session_id"`
				ChatID    uint   `json:"chat_id"`
				UserID    uint   `json:"user_id"`
				Name      string `json:"name"`
			} `json:"attributes"`
		}{
			Type: "chat_history_records",
			Attributes: struct {
				SessionID string `json:"session_id"`
				ChatID    uint   `json:"chat_id"`
				UserID    uint   `json:"user_id"`
				Name      string `json:"name"`
			}{
				SessionID: "",
				ChatID:    0,
				UserID:    0,
				Name:      "",
			},
		},
	}
	w = performRequest(api.router, "POST", "/api/v1/chat-history-records", invalidCreateInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test List ChatHistoryRecords with invalid user_id
	w = performRequest(api.router, "GET", "/api/v1/chat-history-records?user_id=invalid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Delete non-existent ChatHistoryRecord
	w = performRequest(api.router, "DELETE", "/api/v1/chat-history-records/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test List ChatHistoryRecords with non-existent user_id
	w = performRequest(api.router, "GET", "/api/v1/chat-history-records?user_id=999", nil)
	assert.Equal(t, http.StatusOK, w.Code) // This should return an empty list, not an error

	var emptyResponse ChatHistoryRecordListResponse
	err := json.Unmarshal(w.Body.Bytes(), &emptyResponse)
	assert.NoError(t, err)
	assert.Len(t, emptyResponse.Data, 0)
}

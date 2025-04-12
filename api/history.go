package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// createChatHistoryRecord godoc
// @Summary Create a new chat history record
// @Description Create a new chat history record with the given input
// @Tags chat-history
// @Accept json
// @Produce json
// @Param input body ChatHistoryRecordInput true "Chat History Record Input"
// @Success 201 {object} ChatHistoryRecordResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /chat-history-records [post]
func (a *API) createChatHistoryRecord(c *gin.Context) {
	var input ChatHistoryRecordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: err.Error()}}})
		return
	}

	if input.Data.Attributes.SessionID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Session ID is required"}}})
		return
	}

	if input.Data.Attributes.ChatID == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Chat ID is required"}}})
		return
	}

	if input.Data.Attributes.UserID == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "User ID is required"}}})
		return
	}

	if input.Data.Attributes.Name == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Name is required"}}})
		return
	}

	record, err := a.service.CreateChatHistoryRecord(
		input.Data.Attributes.SessionID,
		input.Data.Attributes.ChatID,
		input.Data.Attributes.UserID,
		input.Data.Attributes.Name,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	c.JSON(http.StatusCreated, ChatHistoryRecordResponse{
		Type: "chat_history_record",
		ID:   strconv.FormatUint(uint64(record.ID), 10),
		Attributes: struct {
			SessionID string `json:"session_id"`
			ChatID    uint   `json:"chat_id"`
			UserID    uint   `json:"user_id"`
			Name      string `json:"name"`
			Hidden    bool   `json:"hidden"`
		}{
			SessionID: record.SessionID,
			ChatID:    record.ChatID,
			UserID:    record.UserID,
			Name:      record.Name,
			Hidden:    record.Hidden,
		},
	})
}

// getChatHistoryRecord godoc
// @Summary Get a chat history record
// @Description Get a chat history record by its ID
// @Tags chat-history
// @Accept json
// @Produce json
// @Param id path int true "Chat History Record ID"
// @Success 200 {object} ChatHistoryRecordResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /chat-history-records/{id} [get]
func (a *API) getChatHistoryRecord(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Invalid ID format"}}})
		return
	}

	record, err := a.service.GetChatHistoryRecordByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Not Found", Detail: "Chat history record not found"}}})
		return
	}

	c.JSON(http.StatusOK, ChatHistoryRecordResponse{
		Type: "chat_history_record",
		ID:   strconv.FormatUint(uint64(record.ID), 10),
		Attributes: struct {
			SessionID string `json:"session_id"`
			ChatID    uint   `json:"chat_id"`
			UserID    uint   `json:"user_id"`
			Name      string `json:"name"`
			Hidden    bool   `json:"hidden"`
		}{
			SessionID: record.SessionID,
			ChatID:    record.ChatID,
			UserID:    record.UserID,
			Name:      record.Name,
			Hidden:    record.Hidden,
		},
	})
}

// listChatHistoryRecords godoc
// @Summary List chat history records
// @Description List chat history records for a given user
// @Tags chat-history
// @Accept json
// @Produce json
// @Param user_id query int true "User ID"
// @Param page query int false "Page number"
// @Param page_size query int false "Page size"
// @Param all query bool false "Retrieve all records"
// @Success 200 {object} ChatHistoryRecordListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /chat-history-records [get]
func (a *API) listChatHistoryRecords(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Query("user_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Invalid user ID format"}}})
		return
	}

	pageSize, pageNumber, all := getPaginationParams(c)

	records, totalCount, totalPages, err := a.service.ListChatHistoryRecordsByUserIDPaginated(uint(userID), pageSize, pageNumber, all)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	response := ChatHistoryRecordListResponse{Data: serializeChatHistoryRecords(records)}

	c.Header("X-Total-Count", strconv.FormatInt(totalCount, 10))
	c.Header("X-Total-Pages", strconv.Itoa(totalPages))
	c.JSON(http.StatusOK, response)
}

// toggleChatHistoryVisibility godoc
// @Summary Toggle visibility of a chat history record
// @Description Mark a chat history record as hidden or visible
// @Tags chat-history
// @Accept json
// @Produce json
// @Param id path int true "Chat History Record ID"
// @Param input body ChatHistoryVisibilityInput true "Visibility Input"
// @Success 200 {object} ChatHistoryRecordResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /chat-history-records/{id}/visibility [patch]
func (a *API) toggleChatHistoryVisibility(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Invalid ID format"}}})
		return
	}

	var input ChatHistoryVisibilityInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: err.Error()}}})
		return
	}

	record, err := a.service.UpdateChatHistoryVisibility(uint(id), input.Data.Attributes.Hidden)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Chat history record not found"}}})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	c.JSON(http.StatusOK, ChatHistoryRecordResponse{
		Type: "chat_history_record",
		ID:   strconv.FormatUint(uint64(record.ID), 10),
		Attributes: struct {
			SessionID string `json:"session_id"`
			ChatID    uint   `json:"chat_id"`
			UserID    uint   `json:"user_id"`
			Name      string `json:"name"`
			Hidden    bool   `json:"hidden"`
		}{
			SessionID: record.SessionID,
			ChatID:    record.ChatID,
			UserID:    record.UserID,
			Name:      record.Name,
			Hidden:    record.Hidden,
		},
	})
}

// deleteChatHistoryRecord godoc
// @Summary Delete a chat history record
// @Description Delete a chat history record by its ID
// @Tags chat-history
// @Accept json
// @Produce json
// @Param id path int true "Chat History Record ID"
// @Success 204 "No Content"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /chat-history-records/{id} [delete]
func (a *API) deleteChatHistoryRecord(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Invalid ID format"}}})
		return
	}

	if err := a.service.DeleteChatHistoryRecord(uint(id)); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Chat history record not found"}}})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	c.Status(http.StatusNoContent)
}

type ChatHistoryVisibilityInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Hidden bool `json:"hidden"`
		} `json:"attributes"`
	} `json:"data"`
}

type CMessageListResponse struct {
	Data []CMessageResponse `json:"data"`
}

func serializeChatHistoryRecords(records []models.ChatHistoryRecord) []ChatHistoryRecordResponse {
	result := make([]ChatHistoryRecordResponse, len(records))
	for i, record := range records {
		result[i] = ChatHistoryRecordResponse{
			Type: "chat_history_record",
			ID:   strconv.FormatUint(uint64(record.ID), 10),
			Attributes: struct {
				SessionID string `json:"session_id"`
				ChatID    uint   `json:"chat_id"`
				UserID    uint   `json:"user_id"`
				Name      string `json:"name"`
				Hidden    bool   `json:"hidden"`
			}{
				SessionID: record.SessionID,
				ChatID:    record.ChatID,
				UserID:    record.UserID,
				Name:      record.Name,
				Hidden:    record.Hidden,
			},
		}
	}
	return result
}

func serializeCMessages(messages []models.CMessage) []CMessageResponse {
	result := make([]CMessageResponse, len(messages))
	for i, msg := range messages {
		result[i] = CMessageResponse{
			Type: "cmessage",
			ID:   strconv.FormatUint(uint64(msg.ID), 10),
			Attributes: struct {
				Session   string    `json:"session"`
				Content   any       `json:"content"`
				CreatedAt time.Time `json:"created_at"`
				ChatID    uint      `json:"chat_id"`
			}{
				Session:   msg.Session,
				Content:   msg.UnmarshalContent(),
				CreatedAt: msg.CreatedAt,
				ChatID:    msg.ChatID,
			},
		}
	}
	return result
}

// getCMessagesForSession godoc
// @Summary Get messages for a session
// @Description Get paginated messages for a given session ID, ordered from oldest to newest
// @Tags common
// @Accept json
// @Produce json
// @Param session_id path string true "Session ID"
// @Param page query int false "Page number (default 1)"
// @Param page_size query int false "Page size (default 10)"
// @Success 200 {object} CMessageListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/sessions/{session_id}/messages [get]
func (a *API) getCMessagesForSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	pageSize, pageNumber, _ := getPaginationParams(c)

	messages, totalCount, totalPages, err := a.service.GetCMessagesForSessionPaginated(sessionID, pageSize, pageNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	response := CMessageListResponse{Data: serializeCMessages(messages)}

	c.Header("X-Total-Count", strconv.FormatInt(totalCount, 10))
	c.Header("X-Total-Pages", strconv.Itoa(totalPages))
	c.JSON(http.StatusOK, response)
}

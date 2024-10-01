package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// @Summary Create a new chat
// @Description Create a new chat with the provided information
// @Tags chats
// @Accept json
// @Produce json
// @Param chat body ChatInput true "Chat information"
// @Success 201 {object} ChatResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /chats [post]
// @Security BearerAuth
func (a *API) createChat(c *gin.Context) {
	var input ChatInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chat, err := a.service.CreateChat(
		input.Data.Attributes.Name,
		input.Data.Attributes.LLMSettingsID,
		input.Data.Attributes.LLMID,
		input.Data.Attributes.GroupIDs,
		input.Data.Attributes.FilterIDs,
		input.Data.Attributes.RagN,
		input.Data.Attributes.ToolSupport,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeChat(chat)})
}

// @Summary Get a chat by ID
// @Description Get details of a chat by its ID
// @Tags chats
// @Accept json
// @Produce json
// @Param id path int true "Chat ID"
// @Success 200 {object} ChatResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /chats/{id} [get]
// @Security BearerAuth
func (a *API) getChat(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid chat ID"}},
		})
		return
	}

	chat, err := a.service.GetChatByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Chat not found"}},
		})
		return
	}

	// Fetch filters associated with the chat
	filters, err := a.service.GetFiltersByChatID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to fetch filters"}},
		})
		return
	}

	response := serializeChat(chat)
	response.Attributes.Filters = serializeFilters(filters)

	c.JSON(http.StatusOK, gin.H{"data": response})
}

// @Summary Update a chat
// @Description Update an existing chat's information
// @Tags chats
// @Accept json
// @Produce json
// @Param id path int true "Chat ID"
// @Param chat body ChatInput true "Updated chat information"
// @Success 200 {object} ChatResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /chats/{id} [patch]
// @Security BearerAuth
func (a *API) updateChat(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid chat ID"}},
		})
		return
	}

	var input ChatInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chat, err := a.service.UpdateChat(
		uint(id),
		input.Data.Attributes.Name,
		input.Data.Attributes.LLMSettingsID,
		input.Data.Attributes.LLMID,
		input.Data.Attributes.GroupIDs,
		input.Data.Attributes.FilterIDs,
		input.Data.Attributes.RagN,
		input.Data.Attributes.ToolSupport,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeChat(chat)})
}

// @Summary Delete a chat
// @Description Delete a chat by its ID
// @Tags chats
// @Accept json
// @Produce json
// @Param id path int true "Chat ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /chats/{id} [delete]
// @Security BearerAuth
func (a *API) deleteChat(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid chat ID"}},
		})
		return
	}

	err = a.service.DeleteChat(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary List all chats
// @Description Get a list of all chats
// @Tags chats
// @Accept json
// @Produce json
// @Success 200 {array} ChatResponse
// @Failure 500 {object} ErrorResponse
// @Router /chats [get]
// @Security BearerAuth
func (a *API) listChats(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)

	chats, totalCount, totalPages, err := a.service.ListChats(pageSize, pageNumber, all)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Header("X-Total-Count", strconv.FormatInt(totalCount, 10))
	c.Header("X-Total-Pages", strconv.Itoa(totalPages))
	c.JSON(http.StatusOK, gin.H{"data": serializeChats(chats)})
}

// @Summary Get chats by group ID
// @Description Get a list of chats associated with a specific group
// @Tags chats
// @Accept json
// @Produce json
// @Param group_id query int true "Group ID"
// @Success 200 {array} ChatResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /chats/by-group [get]
// @Security BearerAuth
func (a *API) getChatsByGroupID(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Query("group_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	chats, err := a.service.GetChatsByGroupID(uint(groupID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeChats(chats)})
}

func serializeChat(chat *models.Chat) ChatResponse {
	return ChatResponse{
		Type: "chats",
		ID:   strconv.FormatUint(uint64(chat.ID), 10),
		Attributes: struct {
			Name          string           `json:"name"`
			LLMSettingsID uint             `json:"llm_settings_id"`
			LLMID         uint             `json:"llm_id"`
			Groups        []GroupResponse  `json:"groups"`
			Filters       []FilterResponse `json:"filters"`
			RagN          int              `json:"rag_n"`
			ToolSupport   bool             `json:"tool_support"`
		}{
			Name:          chat.Name,
			LLMSettingsID: chat.LLMSettingsID,
			LLMID:         chat.LLMID,
			Groups:        serializeGroups(chat.Groups),
			Filters:       serializeFilters(chat.Filters),
			RagN:          chat.RagResultsPerSource,
			ToolSupport:   chat.SupportsTools,
		},
	}
}

func serializeChats(chats models.Chats) []ChatResponse {
	result := make([]ChatResponse, len(chats))
	for i, chat := range chats {
		result[i] = serializeChat(&chat)
	}
	return result
}

func serializeFilters(f []*models.Filter) []FilterResponse {
	arr := make([]models.Filter, len(f))
	for i, filter := range f {
		arr[i] = *filter
	}
	return toFilterResponses(arr)
}

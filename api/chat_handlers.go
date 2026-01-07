package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// updateChatPromptTemplates updates the prompt templates for a chat
func (a *API) updateChatPromptTemplates(c *gin.Context) {
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

	var input struct {
		Templates []models.PromptTemplate `json:"templates"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chat := &models.Chat{}
	if err := chat.Get(a.config.DB, uint(id)); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Chat not found"}},
		})
		return
	}

	if err := chat.UpdatePromptTemplates(a.config.DB, input.Templates); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeChat(chat, a.config.DB)})
}

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
		input.Data.Attributes.Description,
		input.Data.Attributes.LLMSettingsID,
		input.Data.Attributes.LLMID,
		input.Data.Attributes.GroupIDs,
		input.Data.Attributes.FilterIDs,
		input.Data.Attributes.RagN,
		input.Data.Attributes.ToolSupport,
		input.Data.Attributes.SystemPrompt,
		uint(input.Data.Attributes.DefaultDataSourceID),
		input.Data.Attributes.DefaultToolIDs,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeChat(chat, a.config.DB)})
}

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

	response := serializeChat(chat, a.config.DB)
	response.Attributes.Filters = serializeFilters(filters)

	c.JSON(http.StatusOK, gin.H{"data": response})
}

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
		input.Data.Attributes.Description,
		input.Data.Attributes.LLMSettingsID,
		input.Data.Attributes.LLMID,
		input.Data.Attributes.GroupIDs,
		input.Data.Attributes.FilterIDs,
		input.Data.Attributes.RagN,
		input.Data.Attributes.ToolSupport,
		input.Data.Attributes.SystemPrompt,
		uint(input.Data.Attributes.DefaultDataSourceID),
		input.Data.Attributes.DefaultToolIDs,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeChat(chat, a.config.DB)})
}

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
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

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
	c.JSON(http.StatusOK, gin.H{"data": serializeChats(chats, a.config.DB)})
}

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

	c.JSON(http.StatusOK, gin.H{"data": serializeChats(chats, a.config.DB)})
}

func (a *API) addExtraContextToChat(c *gin.Context) {
	chatID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid chat ID"}},
		})
		return
	}

	fileStoreID, err := strconv.ParseUint(c.Param("filestore_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid filestore ID"}},
		})
		return
	}

	err = a.service.AddExtraContextToChat(uint(chatID), uint(fileStoreID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	chat, err := a.service.GetChatByID(uint(chatID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Chat not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeChat(chat, a.config.DB)})
}

func (a *API) removeExtraContextFromChat(c *gin.Context) {
	chatID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid chat ID"}},
		})
		return
	}

	fileStoreID, err := strconv.ParseUint(c.Param("filestore_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid filestore ID"}},
		})
		return
	}

	err = a.service.RemoveExtraContextFromChat(uint(chatID), uint(fileStoreID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	chat, err := a.service.GetChatByID(uint(chatID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Chat not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeChat(chat, a.config.DB)})
}

func (a *API) getChatExtraContext(c *gin.Context) {
	chatID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid chat ID"}},
		})
		return
	}

	fileStores, err := a.service.GetChatExtraContexts(uint(chatID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeFileStores(fileStores)})
}

func (a *API) setChatExtraContext(c *gin.Context) {
	chatID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid chat ID"}},
		})
		return
	}

	var fileStoreIDs []uint
	if err := c.ShouldBindJSON(&fileStoreIDs); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	err = a.service.SetChatExtraContexts(uint(chatID), fileStoreIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	chat, err := a.service.GetChatByID(uint(chatID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Chat not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeChat(chat, a.config.DB)})
}

func serializeChat(chat *models.Chat, db *gorm.DB) ChatResponse {
	extraContext, _ := chat.GetExtraContext(db)
	var defaultDSID int
	if chat.DefaultDataSource != nil {
		defaultDSID = int(*chat.DefaultDataSourceID)
	}

	templates, _ := chat.GetPromptTemplates()

	return ChatResponse{
		Type: "chats",
		ID:   strconv.FormatUint(uint64(chat.ID), 10),
		Attributes: struct {
			Name                string                   `json:"name"`
			Description         string                   `json:"description"`
			LLMSettingsID       string                   `json:"llm_settings_id"`
			LLMID               string                   `json:"llm_id"`
			Groups              []GroupResponse          `json:"groups"`
			Filters             []FilterResponse         `json:"filters"`
			RagN                int                      `json:"rag_n"`
			ToolSupport         bool                     `json:"tool_support"`
			SystemPrompt        string                   `json:"system_prompt"`
			DefaultDataSourceID int                      `json:"default_data_source_id"`
			DefaultDataSource   DatasourceResponse       `json:"default_data_source"`
			ExtraContext        []FileStoreResponse      `json:"extra_context"`
			DefaultTools        []ToolResponse           `json:"default_tools"`
			PromptTemplates     []PromptTemplateResponse `json:"prompt_templates"`
		}{
			Name:                chat.Name,
			Description:         chat.Description,
			LLMSettingsID:       strconv.FormatUint(uint64(chat.LLMSettingsID), 10),
			LLMID:               strconv.FormatUint(uint64(chat.LLMID), 10),
			Groups:              serializeGroups(chat.Groups),
			Filters:             serializeFilters(chat.Filters),
			RagN:                chat.RagResultsPerSource,
			ToolSupport:         chat.SupportsTools,
			SystemPrompt:        chat.SystemPrompt,
			DefaultDataSourceID: defaultDSID,
			DefaultDataSource:   serializeDatasource(chat.DefaultDataSource),
			ExtraContext:        serializeFileStores(extraContext),
			DefaultTools:        serializeToolsPointersSlim(chat.DefaultTools, db),
			PromptTemplates:     serializePromptTemplates(templates),
		},
	}
}

func serializeChats(chats models.Chats, db *gorm.DB) []ChatResponse {
	result := make([]ChatResponse, len(chats))
	for i, chat := range chats {
		result[i] = serializeChat(&chat, db)
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

func serializePromptTemplates(templates []models.PromptTemplate) []PromptTemplateResponse {
	result := make([]PromptTemplateResponse, len(templates))
	for i, template := range templates {
		result[i] = PromptTemplateResponse{
			Type: "prompt_templates",
			ID:   strconv.FormatUint(uint64(template.ID), 10),
			Attributes: struct {
				Name   string `json:"name"`
				Prompt string `json:"prompt"`
				ChatID uint   `json:"chat_id"`
			}{
				Name:   template.Name,
				Prompt: template.Prompt,
				ChatID: 0, // ChatID is no longer needed since templates are part of the chat
			},
		}
	}
	return result
}

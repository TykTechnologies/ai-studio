package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// getCatalogueLLMs godoc
// @Summary Get LLMs in a catalogue
// @Description Get the list of LLMs in a catalogue by catalogue ID, excluding sensitive information
// @Tags common
// @Accept json
// @Produce json
// @Param id path int true "Catalogue ID"
// @Success 200 {array} LLMResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/catalogues/{id}/llms [get]
func (a *API) getCatalogueLLMs(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not found in context"}}})
		return
	}
	currentUser := user.(*models.User)

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Invalid catalogue ID"}}})
		return
	}

	// Check if the user has access to this catalogue
	catalogues, err := currentUser.GetAccessibleCatalogues(a.service.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	hasAccess := false
	for _, cat := range catalogues {
		if cat.ID == uint(id) {
			hasAccess = true
			break
		}
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Forbidden", Detail: "User does not have access to this catalogue"}}})
		return
	}

	llms, err := a.service.GetCatalogueLLMs(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	response := make([]LLMResponse, len(llms))
	for i, llm := range llms {
		response[i] = LLMResponse{
			Type: "llm",
			ID:   strconv.FormatUint(uint64(llm.ID), 10),
			Attributes: struct {
				Name             string `json:"name"`
				APIKey           string `json:"api_key"`
				APIEndpoint      string `json:"api_endpoint"`
				PrivacyScore     int    `json:"privacy_score"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				LogoURL          string `json:"logo_url"`
				Vendor           string `json:"vendor"`
				Active           bool   `json:"active"`
			}{
				Name:             llm.Name,
				PrivacyScore:     llm.PrivacyScore,
				ShortDescription: llm.ShortDescription,
				LongDescription:  llm.LongDescription,
				LogoURL:          llm.LogoURL,
				Vendor:           string(llm.Vendor),
				Active:           llm.Active,
			},
		}
	}

	c.JSON(http.StatusOK, response)
}

// getDataCatalogueDatasources godoc
// @Summary Get datasources in a data catalogue
// @Description Get the list of datasources in a data catalogue by catalogue ID, excluding sensitive information
// @Tags common
// @Accept json
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Success 200 {array} DatasourceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/data-catalogues/{id}/datasources [get]
func (a *API) getDataCatalogueDatasources(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not found in context"}}})
		return
	}
	currentUser := user.(*models.User)

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Invalid data catalogue ID"}}})
		return
	}

	// Check if the user has access to this data catalogue
	dataCatalogues, err := currentUser.GetAccessibleDataCatalogues(a.service.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	hasAccess := false
	for _, cat := range dataCatalogues {
		if cat.ID == uint(id) {
			hasAccess = true
			break
		}
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Forbidden", Detail: "User does not have access to this data catalogue"}}})
		return
	}

	dataCatalogue, err := a.service.GetDataCatalogueByID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	response := make([]DatasourceResponse, len(dataCatalogue.Datasources))
	for i, ds := range dataCatalogue.Datasources {
		response[i] = DatasourceResponse{
			Type: "datasource",
			ID:   strconv.FormatUint(uint64(ds.ID), 10),
			Attributes: struct {
				Name             string        `json:"name"`
				ShortDescription string        `json:"short_description"`
				LongDescription  string        `json:"long_description"`
				Icon             string        `json:"icon"`
				Url              string        `json:"url"`
				PrivacyScore     int           `json:"privacy_score"`
				UserID           uint          `json:"user_id"`
				Tags             []TagResponse `json:"tags"`
				DBConnString     string        `json:"db_conn_string"`
				DBSourceType     string        `json:"db_source_type"`
				DBConnAPIKey     string        `json:"db_conn_api_key"`
				DBName           string        `json:"db_name"`
				EmbedVendor      string        `json:"embed_vendor"`
				EmbedUrl         string        `json:"embed_url"`
				EmbedAPIKey      string        `json:"embed_api_key"`
				EmbedModel       string        `json:"embed_model"`
				Active           bool          `json:"active"`
			}{
				Name:             ds.Name,
				ShortDescription: ds.ShortDescription,
				LongDescription:  ds.LongDescription,
				Icon:             ds.Icon,
				PrivacyScore:     ds.PrivacyScore,
				UserID:           ds.UserID,
				Tags:             serializeTags(ds.Tags),
				DBSourceType:     ds.DBSourceType,
				DBName:           ds.DBName,
				EmbedVendor:      string(ds.EmbedVendor),
				EmbedModel:       ds.EmbedModel,
				Active:           ds.Active,
			},
		}
	}

	c.JSON(http.StatusOK, response)
}

// getCommonToolCatalogueTools godoc
// @Summary Get tools in a tool catalogue
// @Description Get the list of tools in a tool catalogue by catalogue ID, excluding sensitive information
// @Tags common
// @Accept json
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Success 200 {array} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/tool-catalogues/{id}/tools [get]
func (a *API) getCommonToolCatalogueTools(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not found in context"}}})
		return
	}
	currentUser := user.(*models.User)

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Invalid tool catalogue ID"}}})
		return
	}

	// Check if the user has access to this tool catalogue
	toolCatalogues, err := currentUser.GetAccessibleToolCatalogues(a.service.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	hasAccess := false
	for _, cat := range toolCatalogues {
		if cat.ID == uint(id) {
			hasAccess = true
			break
		}
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Forbidden", Detail: "User does not have access to this tool catalogue"}}})
		return
	}

	toolCatalogue, err := a.service.GetToolCatalogueByID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	response := make([]ToolResponse, len(toolCatalogue.Tools))
	for i, tool := range toolCatalogue.Tools {
		response[i] = ToolResponse{
			Type: "tool",
			ID:   strconv.FormatUint(uint64(tool.ID), 10),
			Attributes: struct {
				Name           string   `json:"name"`
				Description    string   `json:"description"`
				ToolType       string   `json:"tool_type"`
				OASSpec        []byte   `json:"oas_spec"`
				PrivacyScore   int      `json:"privacy_score"`
				Operations     []string `json:"operations"`
				AuthKey        string   `json:"auth_key"`
				AuthSchemaName string   `json:"auth_schema_name"`
			}{
				Name:         tool.Name,
				Description:  tool.Description,
				ToolType:     tool.ToolType,
				PrivacyScore: tool.PrivacyScore,
				Operations:   tool.GetOperations(),
			},
		}
	}

	c.JSON(http.StatusOK, response)
}

// getUserChatHistoryRecords godoc
// @Summary Get chat history records for a user
// @Description Get the chat history records for a specific user
// @Tags common
// @Accept json
// @Produce json
// @Param user_id path int true "User ID"
// @Success 200 {array} ChatHistoryRecordResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/users/{user_id}/chat-history-records [get]
func (a *API) getUserChatHistoryRecords(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not found in context"}}})
		return
	}
	currentUser := user.(*models.User)

	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Invalid user ID"}}})
		return
	}

	// Ensure the user is requesting their own chat history
	if currentUser.ID != uint(userID) {
		c.JSON(http.StatusForbidden, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Forbidden", Detail: "You can only access your own chat history"}}})
		return
	}

	records, _, _, err := models.ListChatHistoryRecordsByUserID(a.service.DB, uint(userID), 1, 1, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	response := make([]ChatHistoryRecordResponse, len(records))
	for i, record := range records {
		response[i] = ChatHistoryRecordResponse{
			Type: "chat_history_record",
			ID:   strconv.FormatUint(uint64(record.ID), 10),
			Attributes: struct {
				SessionID string `json:"session_id"`
				ChatID    uint   `json:"chat_id"`
				UserID    uint   `json:"user_id"`
				Name      string `json:"name"`
			}{
				SessionID: record.SessionID,
				ChatID:    record.ChatID,
				UserID:    record.UserID,
				Name:      record.Name,
			},
		}
	}

	c.JSON(http.StatusOK, response)
}

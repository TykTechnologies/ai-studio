package api

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
				Name             string           `json:"name"`
				APIKey           string           `json:"api_key"`
				APIEndpoint      string           `json:"api_endpoint"`
				PrivacyScore     int              `json:"privacy_score"`
				ShortDescription string           `json:"short_description"`
				LongDescription  string           `json:"long_description"`
				LogoURL          string           `json:"logo_url"`
				Vendor           string           `json:"vendor"`
				Active           bool             `json:"active"`
				Filters          []FilterResponse `json:"filters"`
				DefaultModel     string           `json:"default_model"`
				AllowedModels    []string         `json:"allowed_models"`
			}{
				Name:             llm.Name,
				PrivacyScore:     llm.PrivacyScore,
				ShortDescription: llm.ShortDescription,
				LongDescription:  llm.LongDescription,
				LogoURL:          llm.LogoURL,
				Vendor:           string(llm.Vendor),
				Active:           llm.Active,
				DefaultModel:     llm.DefaultModel,
				AllowedModels:    llm.AllowedModels,
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
				Name             string              `json:"name"`
				ShortDescription string              `json:"short_description"`
				LongDescription  string              `json:"long_description"`
				Icon             string              `json:"icon"`
				Url              string              `json:"url"`
				PrivacyScore     int                 `json:"privacy_score"`
				UserID           uint                `json:"user_id"`
				Tags             []TagResponse       `json:"tags"`
				DBConnString     string              `json:"db_conn_string"`
				DBSourceType     string              `json:"db_source_type"`
				DBConnAPIKey     string              `json:"db_conn_api_key"`
				DBName           string              `json:"db_name"`
				EmbedVendor      string              `json:"embed_vendor"`
				EmbedUrl         string              `json:"embed_url"`
				EmbedAPIKey      string              `json:"embed_api_key"`
				EmbedModel       string              `json:"embed_model"`
				Active           bool                `json:"active"`
				Files            []FileStoreResponse `json:"files"` // Added Files field
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
				Name           string              `json:"name"`
				Description    string              `json:"description"`
				ToolType       string              `json:"tool_type"`
				OASSpec        string              `json:"oas_spec"`
				PrivacyScore   int                 `json:"privacy_score"`
				Operations     []string            `json:"operations"`
				AuthKey        string              `json:"auth_key"`
				AuthSchemaName string              `json:"auth_schema_name"`
				FileStores     []FileStoreResponse `json:"file_stores"`
				Filters        []FilterResponse    `json:"filters"`
				Dependencies   []ToolResponse      `json:"dependencies"`
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

// createUserApp godoc
// @Summary Create a new app for the authenticated user
// @Description Create a new app associated with the authenticated user, checking for catalogue access and privacy score compatibility
// @Tags common
// @Accept json
// @Produce json
// @Param app body CreateAppRequest true "App creation request"
// @Success 201 {object} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/apps [post]
func (a *API) createUserApp(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not found in context"}}})
		return
	}
	currentUser := user.(*models.User)

	var req CreateAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: err.Error()}}})
		return
	}

	// Check if user has access to the specified datasources and LLMs
	accessibleDataSources, err := currentUser.GetAccessibleDataSources(a.service.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: "Failed to retrieve accessible data sources"}}})
		return
	}

	accessibleLLMs, err := currentUser.GetAccessibleLLMs(a.service.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: "Failed to retrieve accessible LLMs"}}})
		return
	}

	// Validate access to specified datasources and LLMs
	for _, dsID := range req.DataSourceIDs {
		if !containsDataSource(accessibleDataSources, dsID) {
			c.JSON(http.StatusForbidden, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "User does not have access to one or more specified data sources"}}})
			return
		}
	}

	for _, llmID := range req.LLMIDs {
		if !containsLLM(accessibleLLMs, llmID) {
			c.JSON(http.StatusForbidden, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "User does not have access to one or more specified LLMs"}}})
			return
		}
	}

	// Create the app
	app, err := a.service.CreateApp(req.Name, req.Description, currentUser.ID, req.DataSourceIDs, req.LLMIDs)
	if err != nil {
		// Check for specific error types and return appropriate responses
		if errors.Is(err, services.ERRPrivacyScoreMismatch) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Data source privacy score cannot be higher than LLM privacy score"}}})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}}})
		}
		return
	}

	// Send email notification to admin
	if err := a.sendAdminAppNotification(app); err != nil {
		// Log the error, but don't return it to the user
		log.Printf("Failed to send admin notification: %v", err)
	}

	// Prepare the response
	response := AppResponse{
		Type: "app",
		ID:   strconv.FormatUint(uint64(app.ID), 10),
		Attributes: struct {
			Name          string `json:"name"`
			Description   string `json:"description"`
			UserID        uint   `json:"user_id"`
			CredentialID  uint   `json:"credential_id"`
			DatasourceIDs []uint `json:"datasource_ids"`
			LLMIDs        []uint `json:"llm_ids"`
		}{
			Name:         app.Name,
			Description:  app.Description,
			UserID:       app.UserID,
			CredentialID: app.CredentialID,
		},
	}

	c.JSON(http.StatusCreated, response)
}

// Helper functions to check if a data source or LLM is in the accessible list
func containsDataSource(dataSources []models.Datasource, id uint) bool {
	for _, ds := range dataSources {
		if ds.ID == id {
			return true
		}
	}
	return false
}

func containsLLM(llms []models.LLM, id uint) bool {
	for _, llm := range llms {
		if llm.ID == id {
			return true
		}
	}
	return false
}

// CreateAppRequest represents the request body for creating a new app
type CreateAppRequest struct {
	Name          string `json:"name" binding:"required"`
	Description   string `json:"description" binding:"required"`
	DataSourceIDs []uint `json:"data_source_ids" binding:"required"`
	LLMIDs        []uint `json:"llm_ids" binding:"required"`
}

func (a *API) sendAdminAppNotification(app *models.App) error {
	subject := "New App Created"
	appDetailsURL := fmt.Sprintf("%s/admin/apps/%d", a.config.FrontendURL, app.ID)

	user, err := a.service.GetUserByID(app.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user by ID: %w", err)
	}

	var body string
	tmpl, err := template.ParseFiles("./templates/admin-app-notification.tmpl")
	if err != nil {
		// If template is not found, use a simple string
		body = fmt.Sprintf("A new app has been created:\n\nName: %s\nDescription: %s\nCreated by User ID: %d\n\nView app details: %s",
			app.Name, app.Description, app.UserID, appDetailsURL)
	} else {
		var buf bytes.Buffer
		err = tmpl.Execute(&buf, map[string]interface{}{
			"AppName":        app.Name,
			"AppDescription": app.Description,
			"UserName":       user.Name,
			"AppDetailsURL":  appDetailsURL,
		})
		if err != nil {
			return fmt.Errorf("failed to execute email template: %w", err)
		}
		body = buf.String()
	}

	return a.auth.SendEmail(a.config.AdminEmail, subject, body)
}

// getUserAccessibleDataSources godoc
// @Summary Get accessible data sources for the authenticated user
// @Description Get the list of data sources accessible to the authenticated user
// @Tags common
// @Accept json
// @Produce json
// @Success 200 {array} DatasourceResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/accessible-datasources [get]
func (a *API) getUserAccessibleDataSources(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not found in context"}}})
		return
	}
	currentUser := user.(*models.User)

	dataSources, err := currentUser.GetAccessibleDataSources(a.service.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	response := make([]DatasourceResponse, len(dataSources))
	for i, ds := range dataSources {
		response[i] = DatasourceResponse{
			Type: "datasource",
			ID:   strconv.FormatUint(uint64(ds.ID), 10),
			Attributes: struct {
				Name             string              `json:"name"`
				ShortDescription string              `json:"short_description"`
				LongDescription  string              `json:"long_description"`
				Icon             string              `json:"icon"`
				Url              string              `json:"url"`
				PrivacyScore     int                 `json:"privacy_score"`
				UserID           uint                `json:"user_id"`
				Tags             []TagResponse       `json:"tags"`
				DBConnString     string              `json:"db_conn_string"`
				DBSourceType     string              `json:"db_source_type"`
				DBConnAPIKey     string              `json:"db_conn_api_key"`
				DBName           string              `json:"db_name"`
				EmbedVendor      string              `json:"embed_vendor"`
				EmbedUrl         string              `json:"embed_url"`
				EmbedAPIKey      string              `json:"embed_api_key"`
				EmbedModel       string              `json:"embed_model"`
				Active           bool                `json:"active"`
				Files            []FileStoreResponse `json:"files"` // Added Files field
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

// getUserAccessibleLLMs godoc
// @Summary Get accessible LLMs for the authenticated user
// @Description Get the list of LLMs accessible to the authenticated user
// @Tags common
// @Accept json
// @Produce json
// @Success 200 {array} LLMResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/accessible-llms [get]
func (a *API) getUserAccessibleLLMs(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not found in context"}}})
		return
	}
	currentUser := user.(*models.User)

	llms, err := currentUser.GetAccessibleLLMs(a.service.DB)
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
				Name             string           `json:"name"`
				APIKey           string           `json:"api_key"`
				APIEndpoint      string           `json:"api_endpoint"`
				PrivacyScore     int              `json:"privacy_score"`
				ShortDescription string           `json:"short_description"`
				LongDescription  string           `json:"long_description"`
				LogoURL          string           `json:"logo_url"`
				Vendor           string           `json:"vendor"`
				Active           bool             `json:"active"`
				Filters          []FilterResponse `json:"filters"`
				DefaultModel     string           `json:"default_model"`
				AllowedModels    []string         `json:"allowed_models"`
			}{
				Name:             llm.Name,
				PrivacyScore:     llm.PrivacyScore,
				ShortDescription: llm.ShortDescription,
				LongDescription:  llm.LongDescription,
				LogoURL:          llm.LogoURL,
				Vendor:           string(llm.Vendor),
				Active:           llm.Active,
				DefaultModel:     llm.DefaultModel,
				AllowedModels:    llm.AllowedModels,
			},
		}
	}

	c.JSON(http.StatusOK, response)
}

// getUserApps godoc
// @Summary Get apps for the authenticated user
// @Description Get the list of apps created by the authenticated user
// @Tags common
// @Accept json
// @Produce json
// @Param page query int false "Page number"
// @Param page_size query int false "Page size"
// @Param all query bool false "Fetch all records"
// @Success 200 {object} AppListResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/apps [get]
func (a *API) getUserApps(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not found in context"}}})
		return
	}
	currentUser := user.(*models.User)

	pageSize, pageNumber, all := getPaginationParams(c)

	apps, totalCount, totalPages, err := a.service.ListAppsByUserID(currentUser.ID, pageSize, pageNumber, all)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	response := make([]AppResponse, len(apps))
	for i, app := range apps {
		response[i] = AppResponse{
			Type: "app",
			ID:   strconv.FormatUint(uint64(app.ID), 10),
			Attributes: struct {
				Name          string `json:"name"`
				Description   string `json:"description"`
				UserID        uint   `json:"user_id"`
				CredentialID  uint   `json:"credential_id"`
				DatasourceIDs []uint `json:"datasource_ids"`
				LLMIDs        []uint `json:"llm_ids"`
			}{
				Name:         app.Name,
				Description:  app.Description,
				UserID:       app.UserID,
				CredentialID: app.CredentialID,
				DatasourceIDs: func() []uint {
					ids := make([]uint, len(app.Datasources))
					for i, ds := range app.Datasources {
						ids[i] = ds.ID
					}
					return ids
				}(),
				LLMIDs: func() []uint {
					ids := make([]uint, len(app.LLMs))
					for i, llm := range app.LLMs {
						ids[i] = llm.ID
					}
					return ids
				}(),
			},
		}
	}

	c.JSON(http.StatusOK, AppListResponse{
		Data: response,
		Meta: struct {
			TotalCount int64 `json:"total_count"`
			TotalPages int   `json:"total_pages"`
			PageSize   int   `json:"page_size"`
			PageNumber int   `json:"page_number"`
		}{
			TotalCount: totalCount,
			TotalPages: totalPages,
			PageSize:   pageSize,
			PageNumber: pageNumber,
		},
	})
}

// getUserAppDetails godoc
// @Summary Get details of a specific app for the authenticated user
// @Description Get the details of a specific app, including its credential, for the authenticated user
// @Tags common
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Success 200 {object} AppDetailResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/apps/{id} [get]
func (a *API) getUserAppDetails(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not found in context"}}})
		return
	}
	currentUser := user.(*models.User)

	appID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Invalid app ID"}}})
		return
	}

	app, err := a.service.GetAppByID(uint(appID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "App not found"}}})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}}})
		}
		return
	}

	// Check if the current user owns the app
	if app.UserID != currentUser.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Forbidden", Detail: "You don't have permission to access this app"}}})
		return
	}

	// Fetch the associated credential
	credential, err := a.service.GetCredentialByID(app.CredentialID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: "Failed to retrieve app credential"}}})
		return
	}

	response := AppDetailResponse{
		Type: "app",
		ID:   strconv.FormatUint(uint64(app.ID), 10),
		Attributes: struct {
			Name          string           `json:"name"`
			Description   string           `json:"description"`
			UserID        uint             `json:"user_id"`
			CredentialID  uint             `json:"credential_id"`
			DatasourceIDs []uint           `json:"datasource_ids"`
			LLMIDs        []uint           `json:"llm_ids"`
			Credential    CredentialDetail `json:"credential"`
		}{
			Name:         app.Name,
			Description:  app.Description,
			UserID:       app.UserID,
			CredentialID: app.CredentialID,
			DatasourceIDs: func() []uint {
				ids := make([]uint, len(app.Datasources))
				for i, ds := range app.Datasources {
					ids[i] = ds.ID
				}
				return ids
			}(),
			LLMIDs: func() []uint {
				ids := make([]uint, len(app.LLMs))
				for i, llm := range app.LLMs {
					ids[i] = llm.ID
				}
				return ids
			}(),
			Credential: CredentialDetail{
				KeyID:  credential.KeyID,
				Secret: credential.Secret,
				Active: credential.Active,
			},
		},
	}

	c.JSON(http.StatusOK, response)
}

// Add this to your common.go file

// deleteUserApp godoc
// @Summary Delete an app owned by the authenticated user
// @Description Delete an app if it's owned by the authenticated user
// @Tags common
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/apps/{id} [delete]
func (a *API) deleteUserApp(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not found in context"}}})
		return
	}
	currentUser := user.(*models.User)

	appID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Invalid app ID"}}})
		return
	}

	app, err := a.service.GetAppByID(uint(appID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "App not found"}}})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}}})
		}
		return
	}

	// Check if the current user owns the app
	if app.UserID != currentUser.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Forbidden", Detail: "You don't have permission to delete this app"}}})
		return
	}

	// Delete the app
	if err := a.service.DeleteApp(uint(appID)); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: "Failed to delete the app"}}})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: "App successfully deleted",
	})
}

// Add this struct if you haven't already defined it
type SuccessResponse struct {
	Message string `json:"message"`
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
func (a *API) listChatHistoryRecordsForMe(c *gin.Context) {
	user, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "Not Authorized"}}})
		return
	}

	userID := user.(*models.User).ID

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

// getUserAccessibleTools godoc
// @Summary Get accessible tools for the authenticated user
// @Description Get the list of tools accessible to the authenticated user based on their group memberships
// @Tags common
// @Accept json
// @Produce json
// @Success 200 {array} ToolResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/accessible-tools [get]
func (a *API) getUserAccessibleTools(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		slog.Error("user not found in context", "user", user)
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not found in context"}}})
		return
	}
	currentUser := user.(*models.User)

	tools, err := a.service.GetAccessibleToolsForUser(currentUser.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	response := make([]ToolResponse, len(tools))
	for i, tool := range tools {
		response[i] = ToolResponse{
			Type: "tool",
			ID:   strconv.FormatUint(uint64(tool.ID), 10),
			Attributes: struct {
				Name           string              `json:"name"`
				Description    string              `json:"description"`
				ToolType       string              `json:"tool_type"`
				OASSpec        string              `json:"oas_spec"`
				PrivacyScore   int                 `json:"privacy_score"`
				Operations     []string            `json:"operations"`
				AuthKey        string              `json:"auth_key"`
				AuthSchemaName string              `json:"auth_schema_name"`
				FileStores     []FileStoreResponse `json:"file_stores"`
				Filters        []FilterResponse    `json:"filters"`
				Dependencies   []ToolResponse      `json:"dependencies"`
			}{
				Name:         tool.Name,
				Description:  tool.Description,
				ToolType:     tool.ToolType,
				PrivacyScore: tool.PrivacyScore,
				Operations:   tool.GetOperations(),
				// Note: We're not including OASSpec and AuthKey for security reasons
			},
		}
	}

	c.JSON(http.StatusOK, response)
}

// getLastCMessagesForSession godoc
// @Summary Get last messages for a session
// @Description Get the last X messages for a given session ID, ordered from oldest to newest
// @Tags common
// @Accept json
// @Produce json
// @Param session_id path string true "Session ID"
// @Param limit query int false "Number of messages to retrieve (default 10)"
// @Success 200 {array} CMessageResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/sessions/{session_id}/messages [get]
func (a *API) getLastCMessagesForSession(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not found in context"}}})
		return
	}
	currentUser := user.(*models.User)

	sessionID := c.Param("session_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	// Check if the user has access to this session
	chatHistoryRecord, err := a.service.GetChatHistoryRecordBySessionID(sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Session not found"}}})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}}})
		}
		return
	}

	if chatHistoryRecord.UserID != currentUser.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Forbidden", Detail: "You don't have permission to access this session"}}})
		return
	}

	messages, err := a.service.GetLastCMessagesForSession(sessionID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	response := make([]CMessageResponse, len(messages))
	for i, msg := range messages {
		response[i] = CMessageResponse{
			Type: "cmessage",
			ID:   strconv.FormatUint(uint64(msg.ID), 10),
			Attributes: struct {
				Session   string    `json:"session"`
				Content   string    `json:"content"`
				CreatedAt time.Time `json:"created_at"`
				ChatID    uint      `json:"chat_id"`
			}{
				Session:   msg.Session,
				Content:   string(msg.Content),
				CreatedAt: msg.CreatedAt,
				ChatID:    msg.ChatID,
			},
		}
	}

	c.JSON(http.StatusOK, response)
}

// updateChatHistoryRecordName godoc
// @Summary Update the name of a chat history record
// @Description Update the name of a chat history record for the authenticated user
// @Tags chat-history
// @Accept json
// @Produce json
// @Param session_id path string true "Session ID"
// @Param name body string true "New name for the chat history record"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/chat-history-records/{session_id}/name [put]
func (a *API) updateChatHistoryRecordName(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not found in context"}}})
		return
	}
	currentUser := user.(*models.User)

	sessionID := c.Param("session_id")

	var requestBody struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: err.Error()}}})
		return
	}

	// Get the ChatHistoryRecord
	chatHistoryRecord := &models.ChatHistoryRecord{}
	err := chatHistoryRecord.GetBySessionID(a.service.DB, sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Chat history record not found"}}})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}}})
		}
		return
	}

	// Check if the current user owns this chat history record
	if chatHistoryRecord.UserID != currentUser.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Forbidden", Detail: "You don't have permission to update this chat history record"}}})
		return
	}

	// Update the name
	err = chatHistoryRecord.UpdateName(a.service.DB, requestBody.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: "Failed to update chat history record name"}}})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: "Chat history record name updated successfully",
	})
}

// getChatDefaults godoc
// @Summary Get default tools and datasource for a specific chat
// @Description Get the default tools and datasource configured for a specific chat with redacted sensitive information
// @Tags common
// @Accept json
// @Produce json
// @Param id path int true "Chat ID"
// @Success 200 {object} ChatDefaultsResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/chats/{id}/defaults [get]
func (a *API) getChatDefaults(c *gin.Context) {
	_, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not found in context"}}})
		return
	}

	chatID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Invalid chat ID"}}})
		return
	}

	// Get the chat
	chat, err := a.service.GetChatByID(uint(chatID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Chat not found"}}})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}}})
		}
		return
	}

	// Create simplified tool responses
	simplifiedTools := make([]SimplifiedToolResponse, 0)
	if chat.DefaultTools != nil {
		for _, tool := range chat.DefaultTools {
			simplifiedTools = append(simplifiedTools, SimplifiedToolResponse{
				ID:   tool.ID,
				Name: tool.Name,
			})
		}
	}

	// Create simplified datasource response
	var simplifiedDataSource *SimplifiedDataSourceResponse
	if chat.DefaultDataSource != nil {
		simplifiedDataSource = &SimplifiedDataSourceResponse{
			ID:   chat.DefaultDataSource.ID,
			Name: chat.DefaultDataSource.Name,
		}
	}

	response := ChatDefaultsResponse{
		Type: "chat_defaults",
		ID:   strconv.FormatUint(chatID, 10),
		Attributes: struct {
			Name                string                        `json:"name"`
			DefaultDataSource   *SimplifiedDataSourceResponse `json:"default_data_source"`
			DefaultTools        []SimplifiedToolResponse      `json:"default_tools"`
			SupportsTools       bool                          `json:"supports_tools"`
			RagResultsPerSource int                           `json:"rag_results_per_source"`
		}{
			Name:                chat.Name,
			DefaultDataSource:   simplifiedDataSource,
			DefaultTools:        simplifiedTools,
			SupportsTools:       chat.SupportsTools,
			RagResultsPerSource: chat.RagResultsPerSource,
		},
	}

	c.JSON(http.StatusOK, response)
}

type ChatDefaultsResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name                string                        `json:"name"`
		DefaultDataSource   *SimplifiedDataSourceResponse `json:"default_data_source"`
		DefaultTools        []SimplifiedToolResponse      `json:"default_tools"`
		SupportsTools       bool                          `json:"supports_tools"`
		RagResultsPerSource int                           `json:"rag_results_per_source"`
	} `json:"attributes"`
}

// SimplifiedToolResponse represents a redacted version of tool information
type SimplifiedToolResponse struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// SimplifiedDataSourceResponse represents a redacted version of datasource information
type SimplifiedDataSourceResponse struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

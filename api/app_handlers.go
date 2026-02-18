package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// @Summary Create a new app
// @Description Create a new app with the provided information
// @Tags apps
// @Accept json
// @Produce json
// @Param app body AppInput true "App information"
// @Success 201 {object} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps [post]
// @Security BearerAuth
func (a *API) createApp(c *gin.Context) {
	var input AppInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	if input.Data.Attributes.Name == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Name is required"}},
		})
		return
	}

	datasourceIDs := input.Data.Attributes.DatasourceIDs
	llmIDs := input.Data.Attributes.LLMIDs
	toolIDs := input.Data.Attributes.ToolIDs // Added toolIDs
	metadata := input.Data.Attributes.Metadata

	// Convert plugin resource inputs to service selections
	var pluginResources []services.PluginResourceSelection
	for _, pr := range input.Data.Attributes.PluginResources {
		pluginResources = append(pluginResources, services.PluginResourceSelection{
			PluginID:         pr.PluginID,
			ResourceTypeSlug: pr.ResourceTypeSlug,
			InstanceIDs:      pr.InstanceIDs,
		})
	}

	// Use namespace-aware service method if namespace is provided
	var app *models.App
	var err error
	if len(pluginResources) > 0 {
		// Use the extended method that handles plugin resources
		app, err = a.service.CreateAppWithResources(
			input.Data.Attributes.Name,
			input.Data.Attributes.Description,
			input.Data.Attributes.UserID,
			datasourceIDs,
			llmIDs,
			toolIDs,
			input.Data.Attributes.MonthlyBudget,
			input.Data.Attributes.BudgetStartDate,
			metadata,
			pluginResources,
		)
	} else if input.Data.Attributes.Namespace != "" {
		app, err = a.service.CreateAppWithNamespace(
			input.Data.Attributes.Name,
			input.Data.Attributes.Description,
			input.Data.Attributes.UserID,
			datasourceIDs,
			llmIDs,
			toolIDs, // Pass toolIDs to service method
			input.Data.Attributes.MonthlyBudget,
			input.Data.Attributes.BudgetStartDate,
			input.Data.Attributes.Namespace,
			metadata, // Pass metadata
		)
	} else {
		app, err = a.service.CreateApp(
			input.Data.Attributes.Name,
			input.Data.Attributes.Description,
			input.Data.Attributes.UserID,
			datasourceIDs,
			llmIDs,
			toolIDs, // Pass toolIDs to service method
			input.Data.Attributes.MonthlyBudget,
			input.Data.Attributes.BudgetStartDate,
			metadata, // Pass metadata
		)
	}
	if err != nil {
		if err == services.ERRPrivacyScoreMismatch {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Privacy Score Mismatch", Detail: "Datasources have higher privacy requirements than the selected LLMs. Please select LLMs with equal or higher privacy scores."}},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": a.serializeAppWithPluginResources(app)})
}

// @Summary Get an app by ID
// @Description Get details of an app by its ID
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Success 200 {object} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /apps/{id} [get]
// @Security BearerAuth
func (a *API) getApp(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app ID"}},
		})
		return
	}

	app, err := a.service.GetAppByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "App not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": a.serializeAppWithPluginResources(app)})
}

// @Summary Update an app
// @Description Update an existing app's information
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Param app body AppInput true "Updated app information"
// @Success 200 {object} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/{id} [patch]
// @Security BearerAuth
func (a *API) updateApp(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app ID"}},
		})
		return
	}

	var input AppInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	datasourceIDs := input.Data.Attributes.DatasourceIDs
	llmIDs := input.Data.Attributes.LLMIDs
	toolIDs := input.Data.Attributes.ToolIDs // Added toolIDs
	metadata := input.Data.Attributes.Metadata

	// Convert plugin resource inputs to service selections
	var pluginResources []services.PluginResourceSelection
	for _, pr := range input.Data.Attributes.PluginResources {
		pluginResources = append(pluginResources, services.PluginResourceSelection{
			PluginID:         pr.PluginID,
			ResourceTypeSlug: pr.ResourceTypeSlug,
			InstanceIDs:      pr.InstanceIDs,
		})
	}

	var app *models.App
	if len(pluginResources) > 0 {
		app, err = a.service.UpdateAppWithResources(
			uint(id),
			input.Data.Attributes.Name,
			input.Data.Attributes.Description,
			input.Data.Attributes.UserID,
			datasourceIDs,
			llmIDs,
			toolIDs,
			input.Data.Attributes.MonthlyBudget,
			input.Data.Attributes.BudgetStartDate,
			metadata,
			pluginResources,
		)
	} else {
		app, err = a.service.UpdateApp(
			uint(id),
			input.Data.Attributes.Name,
			input.Data.Attributes.Description,
			input.Data.Attributes.UserID,
			datasourceIDs,
			llmIDs,
			toolIDs,
			input.Data.Attributes.MonthlyBudget,
			input.Data.Attributes.BudgetStartDate,
			metadata,
		)
	}
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "App not found"}},
			})
			return
		}
		if err == services.ERRPrivacyScoreMismatch {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Privacy Score Mismatch", Detail: "Datasources have higher privacy requirements than the selected LLMs. Please select LLMs with equal or higher privacy scores."}},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": a.serializeAppWithPluginResources(app)})
}

// @Summary Delete an app
// @Description Delete an app by its ID
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/{id} [delete]
// @Security BearerAuth
func (a *API) deleteApp(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app ID"}},
		})
		return
	}

	err = a.service.DeleteApp(uint(id))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "App not found"}},
			})
			return
		}

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

// @Summary Get apps by user ID
// @Description Get a list of apps for a specific user
// @Tags apps
// @Accept json
// @Produce json
// @Param userId path int true "User ID"
// @Success 200 {array} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/{user_id}/apps [get]
// @Security BearerAuth
func (a *API) getAppsByUserID(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid user ID"}},
		})
		return
	}

	apps, err := a.service.GetAppsByUserID(uint(userID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeApps(apps)})
}

// @Summary Count apps by user ID
// @Description Get the total number of apps for a specific user
// @Tags apps
// @Accept json
// @Produce json
// @Param user_id path int true "User ID"
// @Success 200 {object} map[string]int64
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/{user_id}/apps/count [get]
// @Security BearerAuth
func (a *API) countAppsByUserID(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid user ID"}},
		})
		return
	}

	count, err := a.service.CountAppsByUserID(uint(userID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}

func serializeApp(app *models.App) AppResponse {
	resp := AppResponse{
		Type: "app",
		ID:   strconv.FormatUint(uint64(app.ID), 10),
	}
	resp.Attributes.Name = app.Name
	resp.Attributes.Description = app.Description
	resp.Attributes.UserID = app.UserID
	resp.Attributes.CredentialID = app.CredentialID
	resp.Attributes.DatasourceIDs = getDatasourceIDs(app.Datasources)
	resp.Attributes.LLMIDs = getLLMIDs(app.LLMs)
	resp.Attributes.ToolIDs = getToolIDs(app.Tools)
	resp.Attributes.MonthlyBudget = app.MonthlyBudget
	resp.Attributes.BudgetStartDate = app.BudgetStartDate
	resp.Attributes.IsOrphaned = app.IsOrphaned
	resp.Attributes.Metadata = app.Metadata
	resp.Attributes.Namespace = app.Namespace
	return resp
}

// serializeAppWithPluginResources adds plugin resource associations to the response.
// This is called by handlers that have access to the service layer.
func (a *API) serializeAppWithPluginResources(app *models.App) AppResponse {
	resp := serializeApp(app)

	// Fetch plugin resource associations
	aprs, err := a.service.GetAppPluginResources(app.ID)
	if err != nil || len(aprs) == 0 {
		return resp
	}

	// Group by resource type
	grouped := make(map[uint]*PluginResourceOutput)
	for _, apr := range aprs {
		key := apr.PluginResourceTypeID
		if _, exists := grouped[key]; !exists {
			typeName := ""
			pluginID := uint(0)
			slug := ""
			if apr.PluginResourceType != nil {
				typeName = apr.PluginResourceType.Name
				pluginID = apr.PluginResourceType.PluginID
				slug = apr.PluginResourceType.Slug
			}
			grouped[key] = &PluginResourceOutput{
				PluginID:         pluginID,
				ResourceTypeSlug: slug,
				ResourceTypeName: typeName,
				InstanceIDs:      []string{},
			}
		}
		grouped[key].InstanceIDs = append(grouped[key].InstanceIDs, apr.InstanceID)
	}

	for _, pr := range grouped {
		resp.Attributes.PluginResources = append(resp.Attributes.PluginResources, *pr)
	}

	return resp
}

func getDatasourceIDs(datasources []models.Datasource) []uint {
	ids := make([]uint, len(datasources))
	for i, ds := range datasources {
		ids[i] = ds.ID
	}
	return ids
}

func getLLMIDs(llms []models.LLM) []uint {
	ids := make([]uint, len(llms))
	for i, llm := range llms {
		ids[i] = llm.ID
	}
	return ids
}

func getToolIDs(tools []models.Tool) []uint {
	ids := make([]uint, len(tools))
	for i, tool := range tools {
		ids[i] = tool.ID
	}
	return ids
}

func serializeApps(apps []models.App) []AppResponse {
	responses := make([]AppResponse, len(apps))
	for i, app := range apps {
		responses[i] = serializeApp(&app)
	}
	return responses
}

// createErrorResponse is a helper function to create ErrorResponse structs
func createErrorResponse(title, detail string) ErrorResponse {
	return ErrorResponse{
		Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: title, Detail: detail}},
	}
}

// @Summary Get app by name
// @Description Get details of an app by its name
// @Tags apps
// @Accept json
// @Produce json
// @Param name query string true "App name"
// @Success 200 {object} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /apps/by-name [get]
// @Security BearerAuth
func (a *API) getAppByName(c *gin.Context) {
	name := c.Query("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "App name is required"}},
		})
		return
	}

	app, err := a.service.GetAppByName(name)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "App not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeApp(app)})
}

// @Summary Activate app credential
// @Description Activate the credential associated with an app
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/{id}/activate-credential [post]
// @Security BearerAuth
func (a *API) activateAppCredential(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app ID"}},
		})
		return
	}

	err = a.service.ActivateAppCredential(uint(id))
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

// @Summary Deactivate app credential
// @Description Deactivate the credential associated with an app
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/{id}/deactivate-credential [post]
// @Security BearerAuth
func (a *API) deactivateAppCredential(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app ID"}},
		})
		return
	}

	err = a.service.DeactivateAppCredential(uint(id))
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

// @Summary Reset app budget
// @Description Reset the budget period for an app by setting start date to today
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/{id}/reset-budget [post]
// @Security BearerAuth
func (a *API) resetAppBudget(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app ID"}},
		})
		return
	}

	err = a.service.ResetAppBudget(uint(id))
	if err != nil {
		// Check if it's a "no budget configured" error
		if err.Error() == "app does not have a budget configured" {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: err.Error()}},
			})
			return
		}
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

// @Summary List all apps
// @Description Get a list of all apps, optionally filtered by search term
// @Tags apps
// @Accept json
// @Produce json
// @Param page query int false "Page number"
// @Param page_size query int false "Page size"
// @Param search query string false "Search term (searches name, description, and user)"
// @Success 200 {array} AppResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps [get]
// @Security BearerAuth
func (a *API) listApps(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)
	sort := c.Query("sort")
	searchTerm := c.Query("search")

	var apps models.Apps
	var totalCount int64
	var totalPages int
	var err error

	if searchTerm != "" {
		apps, totalCount, totalPages, err = a.service.SearchApps(searchTerm, pageSize, pageNumber, all, sort)
	} else {
		apps, totalCount, totalPages, err = a.service.ListAppsWithPagination(pageSize, pageNumber, all, sort)
	}

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
	c.JSON(http.StatusOK, gin.H{"data": serializeApps(apps)})
}

// @Summary Search apps
// @Description Search for apps based on a search term
// @Tags apps
// @Accept json
// @Produce json
// @Param search_term query string true "Search term"
// @Param page query int false "Page number"
// @Param page_size query int false "Page size"
// @Success 200 {array} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/search [get]
// @Security BearerAuth
func (a *API) searchApps(c *gin.Context) {
	searchTerm := c.Query("search_term")
	if searchTerm == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Search term is required"}},
		})
		return
	}

	pageSize, pageNumber, all := getPaginationParams(c)
	sort := c.Query("sort")

	apps, totalCount, totalPages, err := a.service.SearchApps(searchTerm, pageSize, pageNumber, all, sort)
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
	c.JSON(http.StatusOK, gin.H{"data": serializeApps(apps)})
}

// @Summary Count all apps
// @Description Get the total number of apps
// @Tags apps
// @Accept json
// @Produce json
// @Success 200 {object} map[string]int64
// @Failure 500 {object} ErrorResponse
// @Router /apps/count [get]
// @Security BearerAuth
func (a *API) countApps(c *gin.Context) {
	count, err := a.service.CountApps()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}

// @Summary Add a tool to an app
// @Description Associate a tool with an app
// @Tags apps
// @Accept json
// @Produce json
// @Param app_id path int true "App ID"
// @Param tool_id path int true "Tool ID"
// @Success 200 {object} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/{id}/tools/{tool_id} [post]
// @Security BearerAuth
func (a *API) addToolToApp(c *gin.Context) {
	appID, err := strconv.ParseUint(c.Param("id"), 10, 32) // Changed "app_id" to "id"
	if err != nil {
		c.JSON(http.StatusBadRequest, createErrorResponse("Bad Request", "Invalid App ID"))
		return
	}
	toolID, err := strconv.ParseUint(c.Param("tool_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, createErrorResponse("Bad Request", "Invalid Tool ID"))
		return
	}

	app, err := a.service.AddToolToApp(uint(appID), uint(toolID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, createErrorResponse("Not Found", "App or Tool not found"))
			return
		}
		// Consider other specific errors, e.g., if the tool is already added
		c.JSON(http.StatusInternalServerError, createErrorResponse("Internal Server Error", err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeApp(app)})
}

// @Summary Remove a tool from an app
// @Description Disassociate a tool from an app
// @Tags apps
// @Produce json
// @Param app_id path int true "App ID"
// @Param tool_id path int true "Tool ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/{id}/tools/{tool_id} [delete]
// @Security BearerAuth
func (a *API) removeToolFromApp(c *gin.Context) {
	appID, err := strconv.ParseUint(c.Param("id"), 10, 32) // Changed "app_id" to "id"
	if err != nil {
		c.JSON(http.StatusBadRequest, createErrorResponse("Bad Request", "Invalid App ID"))
		return
	}
	toolID, err := strconv.ParseUint(c.Param("tool_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, createErrorResponse("Bad Request", "Invalid Tool ID"))
		return
	}

	err = a.service.RemoveToolFromApp(uint(appID), uint(toolID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, createErrorResponse("Not Found", "App or Tool not found, or tool not associated with app"))
			return
		}
		c.JSON(http.StatusInternalServerError, createErrorResponse("Internal Server Error", err.Error()))
		return
	}
	c.Status(http.StatusNoContent)
}

// @Summary Get tools for an app
// @Description Retrieve all tools associated with a specific app
// @Tags apps
// @Produce json
// @Param app_id path int true "App ID"
// @Success 200 {object} AppResponse // Should be []ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/{id}/tools [get]
// @Security BearerAuth
func (a *API) getAppTools(c *gin.Context) {
	appID, err := strconv.ParseUint(c.Param("id"), 10, 32) // Changed "app_id" to "id"
	if err != nil {
		c.JSON(http.StatusBadRequest, createErrorResponse("Bad Request", "Invalid App ID"))
		return
	}

	tools, err := a.service.GetAppTools(uint(appID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, createErrorResponse("Not Found", "App not found"))
			return
		}
		c.JSON(http.StatusInternalServerError, createErrorResponse("Internal Server Error", err.Error()))
		return
	}

	// Convert tools to response format with full details
	response := make([]gin.H, len(tools))
	for i, tool := range tools {
		response[i] = gin.H{
			"type": "tools",
			"id":   tool.ID,
			"attributes": gin.H{
				"name":          tool.Name,
				"description":   tool.Description,
				"tool_type":     tool.ToolType,
				"privacy_score": tool.PrivacyScore,
				"created_at":    tool.CreatedAt,
				"updated_at":    tool.UpdatedAt,
			},
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": response})
}

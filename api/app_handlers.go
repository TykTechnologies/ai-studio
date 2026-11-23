package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
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
	// Bound with the body kept, so workspace_id can be read from it too.
	if err := c.ShouldBindBodyWith(&input, binding.JSON); err != nil {
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

	// The Workspace the App lands in (decision D5); binding servers and
	// routers and publishing it live are checked there.
	workspaceID, ok := a.resolveCreateFromBody(c, "apps")
	if !ok {
		return
	}

	datasourceIDs := input.Data.Attributes.DatasourceIDs
	llmIDs := input.Data.Attributes.LLMIDs
	toolIDs := input.Data.Attributes.ToolIDs // Added toolIDs
	metadata := input.Data.Attributes.Metadata
	if !a.requireAppMemberRead(c, llmIDs, toolIDs, datasourceIDs) {
		return
	}

	// Convert plugin resource inputs to service selections
	var pluginResources []services.PluginResourceSelection
	for _, pr := range input.Data.Attributes.PluginResources {
		pluginResources = append(pluginResources, services.PluginResourceSelection{
			PluginID:         pr.PluginID,
			ResourceTypeSlug: pr.ResourceTypeSlug,
			InstanceIDs:      pr.InstanceIDs,
		})
	}
	if !a.requireNewPluginResourcesReadable(c, nil, pluginResources) {
		return
	}

	// MCP server bindings are validated before the app exists so a refusal
	// leaves nothing behind.
	if input.Data.Attributes.MCPServerIDs != nil {
		actorID, actorAdmin := a.adminAppActorIn(c, workspaceID)
		if !a.validateAppMCPBindings(c, actorID, actorAdmin, llmIDs, *input.Data.Attributes.MCPServerIDs) {
			return
		}
	}
	// So are router grants.
	routerOpts, ok := a.adminAppRouterOptions(c, 0, input.Data.Attributes.ModelRouterIDs, input.Data.Attributes.SemanticRouterIDs)
	if !ok {
		return
	}

	// Apps default to active. Asking for an active app explicitly needs
	// apps:publish; a caller without it gets an inactive app unless they
	// asked for one anyway.
	wantActive := true
	if input.Data.Attributes.IsActive != nil {
		wantActive = *input.Data.Attributes.IsActive
		if !a.requirePublishToCreateLiveIn(c, "apps", workspaceID, wantActive) {
			return
		}
	} else if !a.canPublishAppIn(c, workspaceID) {
		wantActive = false
	}

	appOpts := []services.AppOption{services.WithAppWorkspace(workspaceID)}
	if input.Data.Attributes.TeamID != nil {
		if !a.authorizeAppTeam(c, input.Data.Attributes.UserID, input.Data.Attributes.TeamID) {
			return
		}
		appOpts = append(appOpts, services.WithAppTeam(*input.Data.Attributes.TeamID))
	}
	tracingOpt, ok := a.appTracingOption(c, input.Data.Attributes.Tracing, models.AppTracingSettings{})
	if !ok {
		return
	}
	appOpts = append(appOpts, tracingOpt)

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
			append(routerOpts, appOpts...)...,
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
			append(routerOpts, appOpts...)...,
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
			append(routerOpts, appOpts...)...,
		)
	}
	if isWorkspaceNotFound(err) {
		invalidWorkspace(c)
		return
	}
	if err != nil {
		if errors.Is(err, services.ERRPrivacyScoreMismatch) {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Privacy Score Mismatch", Detail: err.Error()}},
			})
			return
		}
		if errors.Is(err, services.ErrResourceNotAppGranted) {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Resource Not Available To Apps", Detail: err.Error()}},
			})
			return
		}
		if he, ok := asHelperError(err); ok {
			helpers.SendErrorResponse(c, he)
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

	// The column defaults to true on insert; write the value decided above.
	if !wantActive {
		if app, err = a.service.SetAppActive(app.ID, false, currentUserID(c)); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: err.Error()}},
			})
			return
		}
	}

	if input.Data.Attributes.MCPServerIDs != nil {
		var ok bool
		if app, ok = a.applyAppMCPServers(c, app, *input.Data.Attributes.MCPServerIDs); !ok {
			return
		}
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
	if err := c.ShouldBindBodyWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	// A budget key left out keeps its value. A nil MonthlyBudget means "no
	// limit", so reading an omitted key as nil lifted the App's limit. If the
	// keys cannot be read, refuse the request rather than guess.
	present, err := boundAttributeKeys(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}
	// Apps are hidden outside the caller's Workspaces (workspace_full): a
	// hidden App answers as a missing one. Write in its Workspace; a
	// changed workspace_id is a move (write in both).
	stored, err := a.service.GetAppByID(uint(id))
	if err != nil {
		appNotFound(c)
		return
	}
	if !a.requireVisible(c, "apps", stored.WorkspaceID, appNotFound) {
		return
	}
	requested, ok := boundWorkspaceID(c)
	if !ok {
		return
	}
	moveTo, ok := a.checkObjectUpdate(c, "apps", stored.WorkspaceID, requested)
	if !ok {
		return
	}
	dest := stored.WorkspaceID // where the App is after this request
	if moveTo != nil {
		dest = *moveTo
	}

	// The same goes for every other attribute: PATCH changes only the keys
	// the body holds, so a metadata-only PATCH must not blank the App's
	// name, owner or LLM access (dg5).
	if !hasAllAppAttributes(present) {
		existing, err := a.service.GetAppByID(uint(id))
		if err != nil {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "App not found"}},
			})
			return
		}
		keepOmittedAppAttributes(&input, present, existing)
	}

	datasourceIDs := input.Data.Attributes.DatasourceIDs
	llmIDs := input.Data.Attributes.LLMIDs
	toolIDs := input.Data.Attributes.ToolIDs // Added toolIDs
	metadata := input.Data.Attributes.Metadata
	if !a.requireAppMemberRead(c, llmIDs, toolIDs, datasourceIDs) {
		return
	}

	// Flipping the active switch is the publish action on apps; an omitted
	// switch keeps its value.
	if input.Data.Attributes.IsActive != nil {
		existing, err := a.service.GetAppByID(uint(id))
		if err != nil {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "App not found"}},
			})
			return
		}
		if !a.requirePublishIfChangedIn(c, "apps", existing.WorkspaceID, existing.IsActive, *input.Data.Attributes.IsActive) {
			return
		}
	}

	// Convert plugin resource inputs to service selections
	var pluginResources []services.PluginResourceSelection
	for _, pr := range input.Data.Attributes.PluginResources {
		pluginResources = append(pluginResources, services.PluginResourceSelection{
			PluginID:         pr.PluginID,
			ResourceTypeSlug: pr.ResourceTypeSlug,
			InstanceIDs:      pr.InstanceIDs,
		})
	}
	if !a.requireNewPluginResourcesReadable(c, &stored.ID, pluginResources) {
		return
	}

	if input.Data.Attributes.MCPServerIDs != nil {
		actorID, actorAdmin := a.adminAppActorIn(c, dest)
		if !a.validateAppMCPBindings(c, actorID, actorAdmin, llmIDs, *input.Data.Attributes.MCPServerIDs) {
			return
		}
	}
	routerOpts, ok := a.adminAppRouterOptions(c, stored.ID, input.Data.Attributes.ModelRouterIDs, input.Data.Attributes.SemanticRouterIDs)
	if !ok {
		return
	}

	var appOpts []services.AppOption
	if input.Data.Attributes.TeamID != nil {
		if !a.authorizeAppTeam(c, input.Data.Attributes.UserID, input.Data.Attributes.TeamID) {
			return
		}
		appOpts = append(appOpts, services.WithAppTeam(*input.Data.Attributes.TeamID))
	}
	if input.Data.Attributes.Tracing != nil {
		existing, err := a.service.GetAppByID(uint(id))
		if err != nil {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "App not found"}},
			})
			return
		}
		tracingOpt, ok := a.appTracingOption(c, input.Data.Attributes.Tracing, existing.Tracing)
		if !ok {
			return
		}
		appOpts = append(appOpts, tracingOpt)
	}

	// A change to the App's resources by anyone but a full administrator
	// clears its attested guardrail capability.
	appOpts = append(appOpts, capabilityActorOption(c))

	// The move goes first; a failed update puts it back.
	finishMove, ok := a.beginMove(c, &models.App{}, stored.ID, stored.WorkspaceID, moveTo)
	if !ok {
		return
	}
	updated := false
	defer func() { finishMove(updated) }()

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
			append(routerOpts, appOpts...)...,
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
			append(routerOpts, appOpts...)...,
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
		if errors.Is(err, services.ERRPrivacyScoreMismatch) {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Privacy Score Mismatch", Detail: err.Error()}},
			})
			return
		}
		if errors.Is(err, services.ErrResourceNotAppGranted) {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Resource Not Available To Apps", Detail: err.Error()}},
			})
			return
		}
		if he, ok := asHelperError(err); ok {
			helpers.SendErrorResponse(c, he)
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
	if moveTo != nil {
		app.WorkspaceID = *moveTo
	}
	updated = true

	if input.Data.Attributes.IsActive != nil && app.IsActive != *input.Data.Attributes.IsActive {
		reset := app.GuardrailCapabilityReset
		if app, err = a.service.SetAppActive(app.ID, *input.Data.Attributes.IsActive, currentUserID(c)); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: err.Error()}},
			})
			return
		}
		// A reset by an earlier step of the same request stays reported,
		// as in applyAppMCPServers.
		app.GuardrailCapabilityReset = app.GuardrailCapabilityReset || reset
	}

	if input.Data.Attributes.MCPServerIDs != nil {
		var ok bool
		if app, ok = a.applyAppMCPServers(c, app, *input.Data.Attributes.MCPServerIDs); !ok {
			return
		}
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
// @Param user_id path int true "User ID"
// @Success 200 {array} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/{user_id}/apps [get]
// @Security BearerAuth
func (a *API) getAppsByUserID(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid user ID"}},
		})
		return
	}

	scopes, ok := a.listWorkspaceScopes(c, "apps", "apps.workspace_id")
	if !ok {
		return
	}
	apps, err := a.service.GetAppsByUserID(uint(userID), scopes...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": a.serializeAppsIn(apps)})
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
	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid user ID"}},
		})
		return
	}

	scopes, ok := a.listWorkspaceScopes(c, "apps", "apps.workspace_id")
	if !ok {
		return
	}
	count, err := a.service.CountAppsByUserID(uint(userID), scopes...)
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
	resp.Attributes.WorkspaceID = app.WorkspaceID
	resp.Attributes.UserID = app.UserID
	resp.Attributes.CredentialID = app.CredentialID
	// The credential is preloaded by every app query (App.Get, Apps.List*,
	// Apps.Search), so this is one join, not a query per row. It stays null
	// when the app has no credential or the credential was not loaded.
	if app.CredentialID != 0 && app.Credential.ID == app.CredentialID {
		active := app.Credential.Active
		resp.Attributes.CredentialActive = &active
	}
	resp.Attributes.DatasourceIDs = getDatasourceIDs(app.Datasources)
	resp.Attributes.LLMIDs = getLLMIDs(app.LLMs)
	resp.Attributes.ToolIDs = getToolIDs(app.Tools)
	resp.Attributes.MCPServerIDs, resp.Attributes.MCPServers = appMCPServerOutputs(app.MCPServers)
	resp.Attributes.ModelRouterIDs, resp.Attributes.ModelRouters = appModelRouterOutputs(app.ModelRouters)
	resp.Attributes.SemanticRouterIDs, resp.Attributes.SemanticRouters = appSemanticRouterOutputs(app.SemanticRouters)
	resp.Attributes.MonthlyBudget = app.MonthlyBudget
	resp.Attributes.BudgetStartDate = app.BudgetStartDate
	resp.Attributes.TeamID = app.TeamID
	resp.Attributes.IsActive = app.IsActive
	resp.Attributes.IsOrphaned = app.IsOrphaned
	resp.Attributes.Metadata = app.Metadata
	resp.Attributes.Namespace = app.Namespace
	resp.Attributes.Tracing = app.Tracing
	resp.Attributes.GuardrailCapability = app.GuardrailCapability
	resp.Attributes.GuardrailCapabilityReset = app.GuardrailCapabilityReset
	return resp
}

// serializeAppWithPluginResources adds plugin resource associations to the response.
// This is called by handlers that have access to the service layer.
func (a *API) serializeAppWithPluginResources(app *models.App) AppResponse {
	resp := serializeApp(app)
	resp.Attributes.WorkspaceID = a.responseWorkspaceID(app.WorkspaceID)

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
				typeName = sanitizeString(apr.PluginResourceType.Name)
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

	// Only among the Apps the caller may see: a hidden App answers as a
	// missing one.
	scopes, ok := a.listWorkspaceScopes(c, "apps", "apps.workspace_id")
	if !ok {
		return
	}
	app, err := a.service.GetAppByName(name, scopes...)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "App not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": a.serializeAppIn(app)})
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

	// Only the Apps the caller may see (workspace_full), plus the optional
	// ?workspace_id= filter; totals and pages follow.
	scopes, ok := a.listWorkspaceScopes(c, "apps", "apps.workspace_id")
	if !ok {
		return
	}

	var apps models.Apps
	var totalCount int64
	var totalPages int
	var err error

	if searchTerm != "" {
		apps, totalCount, totalPages, err = a.service.SearchApps(searchTerm, pageSize, pageNumber, all, sort, scopes...)
	} else {
		apps, totalCount, totalPages, err = a.service.ListAppsWithPagination(pageSize, pageNumber, all, sort, scopes...)
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
	c.JSON(http.StatusOK, gin.H{"data": a.serializeAppsIn(apps)})
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

	scopes, ok := a.listWorkspaceScopes(c, "apps", "apps.workspace_id")
	if !ok {
		return
	}
	apps, totalCount, totalPages, err := a.service.SearchApps(searchTerm, pageSize, pageNumber, all, sort, scopes...)
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
	c.JSON(http.StatusOK, gin.H{"data": a.serializeAppsIn(apps)})
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
	scopes, ok := a.listWorkspaceScopes(c, "apps", "apps.workspace_id")
	if !ok {
		return
	}
	count, err := a.service.CountApps(scopes...)
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

	if !a.requireMemberRead(c, "tools") {
		return
	}
	app, err := a.service.AddToolToApp(uint(appID), uint(toolID), capabilityActorOption(c))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, createErrorResponse("Not Found", "App or Tool not found"))
			return
		}
		// Consider other specific errors, e.g., if the tool is already added
		c.JSON(http.StatusInternalServerError, createErrorResponse("Internal Server Error", err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": a.serializeAppIn(app)})
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

	reset, err := a.service.RemoveToolFromAppReportingReset(uint(appID), uint(toolID), capabilityActorOption(c))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, createErrorResponse("Not Found", "App or Tool not found, or tool not associated with app"))
			return
		}
		c.JSON(http.StatusInternalServerError, createErrorResponse("Internal Server Error", err.Error()))
		return
	}
	if reset {
		// The removal cleared the App's attested guardrail capability: say
		// so (200 with a body instead of the usual 204).
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"guardrail_capability_reset": true}})
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

// appPatchAttributes are the App attributes a PATCH that leaves them out
// keeps; the others are already optional in AppInput.
var appPatchAttributes = []string{"name", "description", "user_id", "datasource_ids", "llm_ids",
	"tool_ids", "metadata", "monthly_budget", "budget_start_date"}

func hasAllAppAttributes(present map[string]json.RawMessage) bool {
	for _, k := range appPatchAttributes {
		if _, ok := present[k]; !ok {
			return false
		}
	}
	return true
}

// keepOmittedAppAttributes sets every attribute the request body left out
// to the App's stored value.
func keepOmittedAppAttributes(input *AppInput, present map[string]json.RawMessage, existing *models.App) {
	attrs := &input.Data.Attributes
	has := func(k string) bool { _, ok := present[k]; return ok }
	if !has("name") {
		attrs.Name = existing.Name
	}
	if !has("description") {
		attrs.Description = existing.Description
	}
	if !has("user_id") {
		attrs.UserID = existing.UserID
	}
	if !has("datasource_ids") {
		attrs.DatasourceIDs = make([]uint, 0, len(existing.Datasources))
		for _, d := range existing.Datasources {
			attrs.DatasourceIDs = append(attrs.DatasourceIDs, d.ID)
		}
	}
	if !has("llm_ids") {
		attrs.LLMIDs = make([]uint, 0, len(existing.LLMs))
		for _, l := range existing.LLMs {
			attrs.LLMIDs = append(attrs.LLMIDs, l.ID)
		}
	}
	if !has("tool_ids") {
		attrs.ToolIDs = make([]uint, 0, len(existing.Tools))
		for _, t := range existing.Tools {
			attrs.ToolIDs = append(attrs.ToolIDs, t.ID)
		}
	}
	if !has("metadata") {
		attrs.Metadata = existing.Metadata
	}
	if !has("monthly_budget") {
		attrs.MonthlyBudget = existing.MonthlyBudget
	}
	if !has("budget_start_date") {
		attrs.BudgetStartDate = existing.BudgetStartDate
	}
}

// boundAttributeKeys returns the data.attributes keys of a body bound with
// ShouldBindBodyWith, so an omitted key can be told from an explicit null.
// It fails rather than report "nothing present" when the body is unavailable,
// since callers read absence as "keep the stored value".
func boundAttributeKeys(c *gin.Context) (map[string]json.RawMessage, error) {
	raw, _ := c.Get(gin.BodyBytesKey)
	body, ok := raw.([]byte)
	if !ok {
		return nil, errors.New("request body is unavailable")
	}
	present := llmPatchAttributeKeys(body)
	if present == nil {
		return nil, errors.New("request body has no data.attributes object")
	}
	return present, nil
}

// canPublishAppIn is canPublish("apps") for an App in a Workspace: with
// Workspace checks on, publish must cover that Workspace, never merely
// somewhere.
func (a *API) canPublishAppIn(c *gin.Context, workspaceID uint) bool {
	if a.workspaceCheckMode(c) == workspaceCheck {
		return a.holdsIn(c, workspaceID, authz.Publish("apps"))
	}
	return a.canPublish(c, "apps")
}

// appNotFound is the admin App routes' not-found answer, also given for an
// App the caller may not see (onObject, requireVisible).
var appNotFound = notFoundBody("App not found")

// serializeAppIn is serializeApp with the App's Workspace read as the API
// means it (a stored 0 is the Default).
func (a *API) serializeAppIn(app *models.App) AppResponse {
	resp := serializeApp(app)
	resp.Attributes.WorkspaceID = a.responseWorkspaceID(app.WorkspaceID)
	return resp
}

func (a *API) serializeAppsIn(apps []models.App) []AppResponse {
	out := make([]AppResponse, len(apps))
	for i := range apps {
		out[i] = a.serializeAppIn(&apps[i])
	}
	return out
}

// requireAppMemberRead: linking LLMs, tools and datasources to an App needs
// read on each kind it names, in some scope (they are read-global, D4), as
// for catalogue members (requireMemberRead).
// requireNewPluginResourcesReadable refuses, with 403, an App write that adds
// a plugin resource instance the caller could not have picked: adding one
// that is not already on the App needs plugins:read, the permission that
// lists resource types and their instances. Plugin resource instances are
// global and can carry upstream credentials, so without this an App writer
// in one Workspace could attach any instance by guessing its ids
// (midsommar-clean-6bg5.82).
//
// Instances already on the App need nothing more: a save that sends them
// back keeps them, and one that leaves some out removes those. appID is nil
// on create, where every instance is new. A resource type the caller names
// that does not exist counts as new, so the answer does not tell a caller
// without plugins:read which types exist.
func (a *API) requireNewPluginResourcesReadable(c *gin.Context, appID *uint, selections []services.PluginResourceSelection) bool {
	if len(selections) == 0 || a.workspaceCheckMode(c) == workspaceOpen || authz.Can(c, authz.Read("plugins")) {
		return true
	}
	for _, sel := range selections {
		if len(sel.InstanceIDs) == 0 {
			continue
		}
		existing := map[string]bool{}
		if appID != nil {
			if prt, err := a.service.GetPluginResourceTypeByPluginAndSlug(sel.PluginID, sel.ResourceTypeSlug); err == nil {
				var rows []models.AppPluginResource
				if err := a.service.DB.Where("app_id = ? AND plugin_resource_type_id = ?", *appID, prt.ID).Find(&rows).Error; err != nil {
					simpleError(c, http.StatusInternalServerError, "Internal Server Error", "could not load the App's plugin resources")
					return false
				}
				for _, r := range rows {
					existing[r.InstanceID] = true
				}
			}
		}
		for _, id := range sel.InstanceIDs {
			if !existing[id] {
				c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(authz.Read("plugins")))
				return false
			}
		}
	}
	return true
}

func (a *API) requireAppMemberRead(c *gin.Context, llmIDs, toolIDs, datasourceIDs []uint) bool {
	for _, m := range []struct {
		ids      []uint
		resource string
	}{{llmIDs, "llms"}, {toolIDs, "tools"}, {datasourceIDs, "datasources"}} {
		if len(m.ids) > 0 && !a.requireMemberRead(c, m.resource) {
			return false
		}
	}
	return true
}

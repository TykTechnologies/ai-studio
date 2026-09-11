package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Dedicated activate/deactivate routes. Each is annotated with the publish
// permission of its resource (api.go), so a reviewer role that holds publish
// but not write can release an object, and a writer without publish cannot
// reach them. The PATCH/PUT update routes keep write and additionally guard
// the live switch with requirePublishIfChanged (authz_publish.go).

func publishTargetID(c *gin.Context, what string) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid " + what + " ID"}},
		})
		return 0, false
	}
	return uint(id), true
}

func writePublishError(c *gin.Context, err error, what string) {
	if err == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: what + " not found"}},
		})
		return
	}
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}},
	})
}

// --- LLMs ---

// @Summary Activate an LLM provider
// @Description Sets the provider active so the gateway serves it. Requires llms:publish.
// @Tags llms
// @Param id path int true "LLM ID"
// @Success 200 {object} LLMResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /llms/{id}/activate [post]
// @Security BearerAuth
func (a *API) activateLLM(c *gin.Context) { a.setLLMActive(c, true) }

// @Summary Deactivate an LLM provider
// @Description Sets the provider inactive. Requires llms:publish.
// @Tags llms
// @Param id path int true "LLM ID"
// @Success 200 {object} LLMResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /llms/{id}/deactivate [post]
// @Security BearerAuth
func (a *API) deactivateLLM(c *gin.Context) { a.setLLMActive(c, false) }

func (a *API) setLLMActive(c *gin.Context, active bool) {
	id, ok := publishTargetID(c, "LLM")
	if !ok {
		return
	}
	if active && !a.publishGateOpen(c, models.GovernedObjectTypeLLM, models.BuiltinObjectID(id), nil) {
		return
	}
	llm, err := a.service.SetLLMActive(id, active, currentUserID(c))
	if err != nil {
		writePublishError(c, err, "LLM")
		return
	}
	if a.proxy != nil {
		a.proxy.Reload()
	}
	c.JSON(http.StatusOK, gin.H{"data": a.withLLMGovernedMetadataOne(a.serializeLLM(llm))})
}

// --- Tools ---

// @Summary Activate a tool
// @Description Sets the tool active so it is served. Requires tools:publish.
// @Tags tools
// @Param id path int true "Tool ID"
// @Success 200 {object} ToolResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /tools/{id}/activate [post]
// @Security BearerAuth
func (a *API) activateTool(c *gin.Context) { a.setToolActive(c, true) }

// @Summary Deactivate a tool
// @Description Sets the tool inactive. Requires tools:publish.
// @Tags tools
// @Param id path int true "Tool ID"
// @Success 200 {object} ToolResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /tools/{id}/deactivate [post]
// @Security BearerAuth
func (a *API) deactivateTool(c *gin.Context) { a.setToolActive(c, false) }

func (a *API) setToolActive(c *gin.Context, active bool) {
	id, ok := publishTargetID(c, "tool")
	if !ok {
		return
	}
	if active && !a.publishGateOpen(c, models.GovernedObjectTypeTool, models.BuiltinObjectID(id), nil) {
		return
	}
	tool, err := a.service.SetToolActive(id, active, currentUserID(c))
	if err != nil {
		writePublishError(c, err, "Tool")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": a.withToolGovernedMetadataOne(serializeTool(tool, a.config.DB))})
}

// --- Datasources ---

// @Summary Activate a datasource
// @Description Sets the datasource active so it is served. Requires datasources:publish.
// @Tags datasources
// @Param id path int true "Datasource ID"
// @Success 200 {object} DatasourceResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /datasources/{id}/activate [post]
// @Security BearerAuth
func (a *API) activateDatasource(c *gin.Context) { a.setDatasourceActive(c, true) }

// @Summary Deactivate a datasource
// @Description Sets the datasource inactive. Requires datasources:publish.
// @Tags datasources
// @Param id path int true "Datasource ID"
// @Success 200 {object} DatasourceResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /datasources/{id}/deactivate [post]
// @Security BearerAuth
func (a *API) deactivateDatasource(c *gin.Context) { a.setDatasourceActive(c, false) }

func (a *API) setDatasourceActive(c *gin.Context, active bool) {
	id, ok := publishTargetID(c, "datasource")
	if !ok {
		return
	}
	if active && !a.publishGateOpen(c, models.GovernedObjectTypeDatasource, models.BuiltinObjectID(id), nil) {
		return
	}
	ds, err := a.service.SetDatasourceActive(id, active, currentUserID(c))
	if err != nil {
		writePublishError(c, err, "Datasource")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": a.withDatasourceGovernedMetadataOne(serializeDatasource(ds))})
}

// --- Apps ---

// @Summary Activate an app
// @Description Sets the app active so its credential is accepted by the gateway. Requires apps:publish.
// @Tags apps
// @Param id path int true "App ID"
// @Success 200 {object} AppResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /apps/{id}/activate [post]
// @Security BearerAuth
func (a *API) activateApp(c *gin.Context) { a.setAppActive(c, true) }

// @Summary Deactivate an app
// @Description Sets the app inactive. Requires apps:publish.
// @Tags apps
// @Param id path int true "App ID"
// @Success 200 {object} AppResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /apps/{id}/deactivate [post]
// @Security BearerAuth
func (a *API) deactivateApp(c *gin.Context) { a.setAppActive(c, false) }

func (a *API) setAppActive(c *gin.Context, active bool) {
	id, ok := publishTargetID(c, "app")
	if !ok {
		return
	}
	app, err := a.service.SetAppActive(id, active, currentUserID(c))
	if err != nil {
		writePublishError(c, err, "App")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": a.serializeAppWithPluginResources(app)})
}

// --- Plugins ---

// @Summary Enable a plugin
// @Description Sets the plugin active and, for Studio plugins, loads it. Requires plugins:publish.
// @Tags plugins
// @Param id path int true "Plugin ID"
// @Success 200 {object} PluginResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/enable [post]
// @Security BearerAuth
func (a *API) enablePlugin(c *gin.Context) { a.setPluginActive(c, true) }

// @Summary Disable a plugin
// @Description Sets the plugin inactive and unloads it. Requires plugins:publish.
// @Tags plugins
// @Param id path int true "Plugin ID"
// @Success 200 {object} PluginResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/disable [post]
// @Security BearerAuth
func (a *API) disablePlugin(c *gin.Context) { a.setPluginActive(c, false) }

func (a *API) setPluginActive(c *gin.Context, active bool) {
	id, ok := publishTargetID(c, "plugin")
	if !ok {
		return
	}
	original, err := a.service.PluginService.GetPlugin(id)
	if err != nil {
		writePublishError(c, gorm.ErrRecordNotFound, "Plugin")
		return
	}
	load := active
	plugin, err := a.service.PluginService.UpdatePlugin(id, &services.UpdatePluginRequest{IsActive: &active, LoadImmediately: &load})
	if err != nil {
		writePublishError(c, err, "Plugin")
		return
	}
	a.applyPluginActivation(plugin, original.IsActive, active)
	if a.service.SystemEvents != nil {
		a.service.SystemEvents.EmitPluginUpdated(plugin, plugin.ID, 0)
	}
	c.JSON(http.StatusOK, gin.H{"data": serializePlugin(plugin)})
}

// --- Metadata schemas ---

// @Summary Activate a governed metadata schema
// @Description Requires metadata:publish.
// @Tags governed-metadata
// @Param id path int true "Schema ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/metadata/schemas/{id}/activate [post]
func (a *API) activateMetadataSchema(c *gin.Context) { a.setMetadataSchemaActive(c, true) }

// @Summary Deactivate a governed metadata schema
// @Description Requires metadata:publish.
// @Tags governed-metadata
// @Param id path int true "Schema ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/metadata/schemas/{id}/deactivate [post]
func (a *API) deactivateMetadataSchema(c *gin.Context) { a.setMetadataSchemaActive(c, false) }

func (a *API) setMetadataSchemaActive(c *gin.Context, active bool) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	svc := a.governedMetadata()
	schema, err := svc.GetSchema(id)
	if writeGovernedMetadataError(c, err) {
		return
	}
	if schema.Active != active {
		schema.Active = active
		if err := svc.UpdateSchema(schema); writeGovernedMetadataError(c, err) {
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeMetadataSchema(schema)})
}

var _ = models.MetadataSchema{}

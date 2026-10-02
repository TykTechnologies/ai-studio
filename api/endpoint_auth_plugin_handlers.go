package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// EndpointAuthPluginsRequest replaces an endpoint's auth plugin list.
type EndpointAuthPluginsRequest struct {
	// PluginIDs in execution order; empty detaches every auth plugin.
	PluginIDs []uint `json:"plugin_ids"`
}

// getEndpointAuthPlugins returns the handler listing the auth plugins of one
// kind of endpoint (models.EndpointType*).
func (a *API) getEndpointAuthPlugins(objectType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			jsonError(c, http.StatusBadRequest, "Bad Request", "invalid id")
			return
		}
		plugins, err := a.service.PluginService.GetEndpointAuthPlugins(objectType, uint(id))
		if err != nil {
			respondEndpointAuthPluginError(c, err)
			return
		}
		response := make([]PluginResponse, len(plugins))
		for i := range plugins {
			response[i] = serializePlugin(&plugins[i])
		}
		c.JSON(http.StatusOK, gin.H{"data": response})
	}
}

// updateEndpointAuthPlugins returns the handler replacing the auth plugin
// list of one kind of endpoint.
func (a *API) updateEndpointAuthPlugins(objectType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			jsonError(c, http.StatusBadRequest, "Bad Request", "invalid id")
			return
		}
		var req EndpointAuthPluginsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			jsonError(c, http.StatusBadRequest, "Bad Request", err.Error())
			return
		}
		if err := a.service.PluginService.SetEndpointAuthPlugins(objectType, uint(id), req.PluginIDs); err != nil {
			respondEndpointAuthPluginError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "auth plugins updated"})
	}
}

func respondEndpointAuthPluginError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrEndpointNotFound):
		jsonError(c, http.StatusNotFound, "Not Found", err.Error())
	case errors.Is(err, services.ErrInvalidEndpointAuthPlugin):
		jsonError(c, http.StatusBadRequest, "Bad Request", err.Error())
	default:
		jsonError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
	}
}

// Swagger stubs: the routes share the two handlers above.

// @Summary List a datasource's auth plugins
// @Description Auth plugins that authenticate gateway requests to the datasource, in execution order
// @Tags datasources
// @Produce json
// @Param id path int true "Datasource ID"
// @Success 200 {array} PluginResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/datasources/{id}/auth-plugins [get]
// @Security BearerAuth
func _docDatasourceAuthPluginsGet() {}

// @Summary Set a datasource's auth plugins
// @Description Replace the auth plugins (in execution order) that authenticate gateway requests to the datasource. When any are set, app keys are refused on it.
// @Tags datasources
// @Accept json
// @Produce json
// @Param id path int true "Datasource ID"
// @Param body body EndpointAuthPluginsRequest true "Plugin IDs in execution order"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/datasources/{id}/auth-plugins [put]
// @Security BearerAuth
func _docDatasourceAuthPluginsPut() {}

// @Summary List a tool's auth plugins
// @Tags tools
// @Produce json
// @Param id path int true "Tool ID"
// @Success 200 {array} PluginResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/tools/{id}/auth-plugins [get]
// @Security BearerAuth
func _docToolAuthPluginsGet() {}

// @Summary Set a tool's auth plugins
// @Description Replace the auth plugins that authenticate gateway requests to the tool (REST and MCP). When any are set, app keys are refused on it.
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Param body body EndpointAuthPluginsRequest true "Plugin IDs in execution order"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/tools/{id}/auth-plugins [put]
// @Security BearerAuth
func _docToolAuthPluginsPut() {}

// @Summary List a model router's auth plugins
// @Tags model-routers
// @Produce json
// @Param id path int true "Model router ID"
// @Success 200 {array} PluginResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/model-routers/{id}/auth-plugins [get]
// @Security BearerAuth
func _docModelRouterAuthPluginsGet() {}

// @Summary Set a model router's auth plugins
// @Tags model-routers
// @Accept json
// @Produce json
// @Param id path int true "Model router ID"
// @Param body body EndpointAuthPluginsRequest true "Plugin IDs in execution order"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/model-routers/{id}/auth-plugins [put]
// @Security BearerAuth
func _docModelRouterAuthPluginsPut() {}

// @Summary List a semantic router's auth plugins
// @Tags semantic-routers
// @Produce json
// @Param id path int true "Semantic router ID"
// @Success 200 {array} PluginResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/semantic-routers/{id}/auth-plugins [get]
// @Security BearerAuth
func _docSemanticRouterAuthPluginsGet() {}

// @Summary Set a semantic router's auth plugins
// @Tags semantic-routers
// @Accept json
// @Produce json
// @Param id path int true "Semantic router ID"
// @Param body body EndpointAuthPluginsRequest true "Plugin IDs in execution order"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/semantic-routers/{id}/auth-plugins [put]
// @Security BearerAuth
func _docSemanticRouterAuthPluginsPut() {}

// @Summary List a custom-endpoint plugin's auth plugins
// @Tags plugins
// @Produce json
// @Param id path int true "Plugin ID"
// @Success 200 {array} PluginResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/auth-plugins [get]
// @Security BearerAuth
func _docPluginAuthPluginsGet() {}

// @Summary Set a custom-endpoint plugin's auth plugins
// @Description Replace the auth plugins that authenticate requests to the plugin's /plugins/{slug}/ endpoints that require auth. When any are set, app keys are refused on them.
// @Tags plugins
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Param body body EndpointAuthPluginsRequest true "Plugin IDs in execution order"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/auth-plugins [put]
// @Security BearerAuth
func _docPluginAuthPluginsPut() {}

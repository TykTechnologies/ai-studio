package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// PluginUpgradePreviewRequest selects the version to preview.
type PluginUpgradePreviewRequest struct {
	Version string `json:"version"` // empty = latest upgrade candidate
}

// PluginUpgradeResponse is returned by a completed upgrade.
type PluginUpgradeResponse struct {
	Data    PluginResponse                `json:"data"`
	Upgrade *services.PluginUpgradeResult `json:"upgrade"`
}

func (a *API) pluginUpgradeService() *services.PluginUpgradeService {
	if a.service.PluginUpgradeService == nil {
		a.service.PluginUpgradeService = services.NewPluginUpgradeService(
			a.service.DB,
			a.service.PluginService,
			a.service.MarketplaceService,
			a.service.AIStudioPluginManager,
			a.service.PluginManifestService,
		)
	}
	return a.service.PluginUpgradeService
}

func upgradeErrorBody(title, detail string) gin.H {
	return gin.H{"errors": []gin.H{{"title": title, "detail": detail}}}
}

// respondPluginUpgradeError maps upgrade failures to responses the UI can act on.
func respondPluginUpgradeError(c *gin.Context, err error) {
	var scopesErr *services.UpgradeScopesNotApprovedError
	var rolledBack *services.UpgradeRolledBackError

	switch {
	case errors.As(err, &scopesErr):
		body := upgradeErrorBody("Scopes Not Approved", err.Error())
		body["missing_scopes"] = scopesErr.Missing
		c.JSON(http.StatusConflict, body)
	case errors.As(err, &rolledBack):
		status := http.StatusUnprocessableEntity
		if rolledBack.RollbackErr != nil {
			status = http.StatusInternalServerError
		}
		body := upgradeErrorBody("Upgrade Rolled Back", err.Error())
		body["rolled_back"] = rolledBack.RollbackErr == nil
		c.JSON(status, body)
	case errors.Is(err, services.ErrUpgradeNotFromMarketplace):
		c.JSON(http.StatusBadRequest, upgradeErrorBody("Bad Request", err.Error()))
	case errors.Is(err, services.ErrUpgradeVersionNotFound):
		c.JSON(http.StatusNotFound, upgradeErrorBody("Not Found", err.Error()))
	case errors.Is(err, services.ErrUpgradeSameVersion), errors.Is(err, services.ErrUpgradeDowngradeNotAllowed):
		c.JSON(http.StatusConflict, upgradeErrorBody("Conflict", err.Error()))
	case errors.Is(err, services.ErrUpgradeManifestMismatch):
		c.JSON(http.StatusUnprocessableEntity, upgradeErrorBody("Unprocessable Entity", err.Error()))
	case errors.Is(err, services.ErrUpgradeEnterpriseOnly):
		c.JSON(http.StatusForbidden, upgradeErrorBody("Forbidden", err.Error()))
	case errors.Is(err, services.ErrUpgradePluginNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, upgradeErrorBody("Not Found", "Plugin not found"))
	case errors.Is(err, services.ErrUpgradeTargetUnavailable):
		// The registry, the signature check or the artifact itself: not
		// something a retry of the same request is likely to fix.
		c.JSON(http.StatusUnprocessableEntity, upgradeErrorBody("Target Version Unavailable", err.Error()))
	default:
		c.JSON(http.StatusInternalServerError, upgradeErrorBody("Internal Server Error", err.Error()))
	}
}

func parsePluginIDParam(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, upgradeErrorBody("Bad Request", "Invalid plugin ID"))
		return 0, false
	}
	return uint(id), true
}

// @Summary Preview a plugin upgrade
// @Description Resolve a marketplace version for an installed plugin, pull and probe its artifact, and report what would change: scopes, hooks, config schema fit, changelog. Nothing is written.
// @Tags plugins,marketplace
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Param body body PluginUpgradePreviewRequest false "Target version (defaults to the latest)"
// @Success 200 {object} services.PluginUpgradePreview
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/upgrade/preview [post]
// @Security BearerAuth
func (a *API) previewPluginUpgrade(c *gin.Context) {
	id, ok := parsePluginIDParam(c)
	if !ok {
		return
	}

	var req PluginUpgradePreviewRequest
	// The body is optional.
	_ = c.ShouldBindJSON(&req)

	preview, err := a.pluginUpgradeService().Preview(c.Request.Context(), id, req.Version)
	if err != nil {
		log.Warn().Err(err).Uint("plugin_id", id).Str("version", req.Version).Msg("Plugin upgrade preview failed")
		respondPluginUpgradeError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": preview})
}

// @Summary Upgrade a plugin from the marketplace
// @Description Move an installed marketplace plugin to another published version in place. Configuration and plugin data are kept. Scopes the new version adds must be listed in approved_scopes. If the new version does not start, the previous version is restored.
// @Tags plugins,marketplace
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Param body body services.PluginUpgradeRequest true "Target version and approved scopes"
// @Success 200 {object} PluginUpgradeResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/upgrade [post]
// @Security BearerAuth
func (a *API) upgradePlugin(c *gin.Context) {
	id, ok := parsePluginIDParam(c)
	if !ok {
		return
	}

	var req services.PluginUpgradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, upgradeErrorBody("Bad Request", "Invalid request body"))
		return
	}

	result, err := a.pluginUpgradeService().Upgrade(c.Request.Context(), id, &req)
	if err != nil {
		log.Warn().Err(err).Uint("plugin_id", id).Str("version", req.Version).Msg("Plugin upgrade failed")
		respondPluginUpgradeError(c, err)
		return
	}

	// Edge gateways learn about the new artifact the same way they learn
	// about any plugin change: the namespace goes Pending until pushed.
	if a.service.SystemEvents != nil {
		a.service.SystemEvents.EmitPluginUpdated(result.Plugin, result.Plugin.ID, 0)
	}

	c.JSON(http.StatusOK, PluginUpgradeResponse{
		Data:    a.serializePluginWithMarketplace(result.Plugin),
		Upgrade: result,
	})
}

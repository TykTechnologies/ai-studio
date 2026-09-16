package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/gin-gonic/gin"
)

// tykMCPService returns the service attached to services.Service, or the
// per-API community stub set up in NewAPI (tests that never call
// InitTykMCP): every call on the stub is refused with ErrEnterpriseFeature,
// exactly like a CE build.
func (a *API) tykMCPService() tykmcp.Service {
	if a.service != nil && a.service.TykMCP != nil {
		return a.service.TykMCP
	}
	return a.tykMCPFallback
}

// tykMCPActor reads the authenticated user and whether they hold the
// execute permission on the given resource.
func tykMCPActor(c *gin.Context, resource string) (tykmcp.Actor, bool) {
	user, exists := c.Get("user")
	if !exists {
		return tykmcp.Actor{}, false
	}
	u, ok := user.(*models.User)
	if !ok || u == nil {
		return tykmcp.Actor{}, false
	}
	return tykmcp.Actor{UserID: u.ID, Email: u.Email, Name: u.Name, CanExecute: authz.Can(c, authz.Execute(resource))}, true
}

func requireTykMCPActor(c *gin.Context, resource string) (tykmcp.Actor, bool) {
	actor, ok := tykMCPActor(c, resource)
	if !ok {
		webhookError(c, http.StatusUnauthorized, "Unauthorized", "User not found in context")
	}
	return actor, ok
}

// tykMCPErrorResponse maps service errors onto the API error envelope.
func tykMCPErrorResponse(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, tykmcp.ErrEnterpriseFeature):
		webhookError(c, http.StatusForbidden, "Enterprise Feature", err.Error())
	case errors.Is(err, tykmcp.ErrDisabled):
		webhookError(c, http.StatusConflict, "Tyk MCP Integration Disabled", err.Error())
	case errors.Is(err, tykmcp.ErrNotFound):
		webhookError(c, http.StatusNotFound, "Not Found", err.Error())
	case errors.Is(err, tykmcp.ErrNotVisible):
		webhookError(c, http.StatusNotFound, "Not Found", err.Error())
	case errors.Is(err, tykmcp.ErrConflict), errors.Is(err, tykmcp.ErrDashboardConflict):
		webhookError(c, http.StatusConflict, "Conflict", err.Error())
	case errors.Is(err, tykmcp.ErrInvalidState), errors.Is(err, tykmcp.ErrInUse):
		webhookError(c, http.StatusConflict, "Invalid State", err.Error())
	case errors.Is(err, tykmcp.ErrSameActivator):
		webhookError(c, http.StatusForbidden, "Forbidden", err.Error())
	case errors.Is(err, tykmcp.ErrCapabilityUnavailable):
		webhookError(c, http.StatusUnprocessableEntity, "Capability Unavailable", err.Error())
	case errors.Is(err, tykmcp.ErrURLPolicy), errors.Is(err, tykmcp.ErrInvalidBundle), errors.Is(err, tykmcp.ErrNotBrokerable):
		webhookError(c, http.StatusUnprocessableEntity, "Unprocessable Entity", err.Error())
	case errors.Is(err, tykmcp.ErrValidation), errors.Is(err, models.ErrSecretsKeyRequired):
		webhookError(c, http.StatusBadRequest, "Bad Request", err.Error())
	case errors.Is(err, tykmcp.ErrDashboard):
		webhookError(c, http.StatusBadGateway, "Tyk Dashboard Error", err.Error())
	default:
		webhookError(c, http.StatusInternalServerError, "Internal Server Error", fallback)
	}
}

func tykMCPIDParam(c *gin.Context, name string) (uint, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 32)
	if err != nil || id == 0 {
		webhookBadRequest(c, "invalid id")
		return 0, false
	}
	return uint(id), true
}

// --- Status ---

// getTykMCPStatus godoc
// @Summary Tyk MCP integration availability
// @Description Reports whether the Tyk Dashboard MCP integration is available (Enterprise), enabled and how many connections exist
// @Tags TykMCP
// @Produce json
// @Success 200 {object} tykmcp.Status
// @Router /tyk-mcp/status [get]
func (a *API) getTykMCPStatus(c *gin.Context) {
	c.JSON(http.StatusOK, a.tykMCPService().Status())
}

// --- Connections ---

// listTykConnections godoc
// @Summary List Tyk Dashboard connections
// @Tags TykMCP
// @Produce json
// @Success 200 {array} models.TykConnectionResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /tyk-connections [get]
func (a *API) listTykConnections(c *gin.Context) {
	list, err := a.tykMCPService().ListConnections(c.Request.Context())
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to list connections")
		return
	}
	if list == nil {
		list = []models.TykConnectionResponse{}
	}
	c.JSON(http.StatusOK, list)
}

// createTykConnection godoc
// @Summary Create a Tyk Dashboard connection
// @Description Stores a pending connection. The access token is encrypted at rest and never returned.
// @Tags TykMCP
// @Accept json
// @Produce json
// @Param body body tykmcp.ConnectionInput true "Connection"
// @Success 201 {object} models.TykConnectionResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /tyk-connections [post]
func (a *API) createTykConnection(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "tyk-connections")
	if !ok {
		return
	}
	var in tykmcp.ConnectionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	view, err := a.tykMCPService().CreateConnection(c.Request.Context(), actor, in)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to create connection")
		return
	}
	c.JSON(http.StatusCreated, view)
}

// getTykConnection godoc
// @Summary Get a Tyk Dashboard connection
// @Tags TykMCP
// @Produce json
// @Param id path int true "Connection ID"
// @Success 200 {object} models.TykConnectionResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /tyk-connections/{id} [get]
func (a *API) getTykConnection(c *gin.Context) {
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	view, err := a.tykMCPService().GetConnection(c.Request.Context(), id)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to get connection")
		return
	}
	c.JSON(http.StatusOK, view)
}

// updateTykConnection godoc
// @Summary Update a Tyk Dashboard connection
// @Description Applies a patch under optimistic locking (lock_version). Changing the URL, token or mode of an active connection re-probes it immediately.
// @Tags TykMCP
// @Accept json
// @Produce json
// @Param id path int true "Connection ID"
// @Param body body tykmcp.ConnectionPatch true "Patch"
// @Success 200 {object} models.TykConnectionResponse
// @Failure 409 {object} models.ErrorResponse
// @Router /tyk-connections/{id} [patch]
func (a *API) updateTykConnection(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "tyk-connections")
	if !ok {
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	var p tykmcp.ConnectionPatch
	if err := c.ShouldBindJSON(&p); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	view, err := a.tykMCPService().UpdateConnection(c.Request.Context(), actor, id, p)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to update connection")
		return
	}
	c.JSON(http.StatusOK, view)
}

// deleteTykConnection godoc
// @Summary Delete a Tyk Dashboard connection
// @Description Refuses while live credentials exist unless force=true, which revokes them first.
// @Tags TykMCP
// @Param id path int true "Connection ID"
// @Param force query bool false "Revoke live credentials first"
// @Success 204
// @Failure 409 {object} models.ErrorResponse
// @Router /tyk-connections/{id} [delete]
func (a *API) deleteTykConnection(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "tyk-connections")
	if !ok {
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	force := c.Query("force") == "true"
	if force && !actor.CanExecute {
		webhookError(c, http.StatusForbidden, "Forbidden", "revoking live credentials needs the execute permission")
		return
	}
	if err := a.tykMCPService().DeleteConnection(c.Request.Context(), actor, id, force); err != nil {
		tykMCPErrorResponse(c, err, "Failed to delete connection")
		return
	}
	c.Status(http.StatusNoContent)
}

type tykConnectionActionInput struct {
	Reason string `json:"reason"`
}

// activateTykConnection godoc
// @Summary Activate a Tyk Dashboard connection
// @Description Probes the Dashboard and makes the connection usable. Recorded in the audit trail.
// @Tags TykMCP
// @Produce json
// @Param id path int true "Connection ID"
// @Success 200 {object} models.TykConnectionResponse
// @Failure 422 {object} models.ErrorResponse
// @Router /tyk-connections/{id}/activate [post]
func (a *API) activateTykConnection(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "tyk-connections")
	if !ok {
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	view, err := a.tykMCPService().ActivateConnection(c.Request.Context(), actor, id)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to activate connection")
		return
	}
	c.JSON(http.StatusOK, view)
}

// disableTykConnection godoc
// @Summary Disable a Tyk Dashboard connection
// @Description Stops syncing and suspends every key minted on the connection.
// @Tags TykMCP
// @Accept json
// @Produce json
// @Param id path int true "Connection ID"
// @Success 200 {object} models.TykConnectionResponse
// @Router /tyk-connections/{id}/disable [post]
func (a *API) disableTykConnection(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "tyk-connections")
	if !ok {
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	var in tykConnectionActionInput
	if err := bindOptionalJSON(c, &in); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	view, err := a.tykMCPService().DisableConnection(c.Request.Context(), actor, id, in.Reason)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to disable connection")
		return
	}
	c.JSON(http.StatusOK, view)
}

// probeTykConnection godoc
// @Summary Re-run the capability probe on a connection
// @Tags TykMCP
// @Produce json
// @Param id path int true "Connection ID"
// @Success 200 {object} tykmcp.ProbeResult
// @Router /tyk-connections/{id}/probe [post]
func (a *API) probeTykConnection(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "tyk-connections")
	if !ok {
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	res, err := a.tykMCPService().ProbeConnection(c.Request.Context(), actor, id)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to probe connection")
		return
	}
	c.JSON(http.StatusOK, res)
}

// probeTykConnectionInput godoc
// @Summary Test unsaved connection settings
// @Description Runs the capability probe against form values without persisting anything.
// @Tags TykMCP
// @Accept json
// @Produce json
// @Param body body tykmcp.ConnectionInput true "Connection"
// @Success 200 {object} tykmcp.ProbeResult
// @Router /tyk-connections/probe [post]
func (a *API) probeTykConnectionInput(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "tyk-connections")
	if !ok {
		return
	}
	var in tykmcp.ConnectionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	if (in.AllowInternalHost || in.MDCBAllowInternalHost) && !actor.CanExecute {
		webhookError(c, http.StatusForbidden, "Forbidden", "allowing internal hosts needs the execute permission on connections")
		return
	}
	res, err := a.tykMCPService().ProbeInput(c.Request.Context(), in)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to probe connection")
		return
	}
	c.JSON(http.StatusOK, res)
}

// syncTykConnection godoc
// @Summary Request an immediate sync of a connection
// @Description Schedules the next sync for now; with wait=true the sync runs on this node and the run record is returned
// @Tags TykMCP
// @Param id path int true "Connection ID"
// @Param wait query bool false "run inline"
// @Success 202
// @Router /tyk-connections/{id}/sync [post]
func (a *API) syncTykConnection(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "tyk-connections")
	if !ok {
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	if c.Query("wait") == "true" {
		run, err := a.tykMCPService().RunSync(c.Request.Context(), actor, id)
		if err != nil {
			if run != nil {
				c.JSON(http.StatusBadGateway, gin.H{"run": run, "errors": []gin.H{{"title": "Tyk Dashboard Error", "detail": err.Error()}}})
				return
			}
			tykMCPErrorResponse(c, err, "Sync failed")
			return
		}
		c.JSON(http.StatusOK, run)
		return
	}
	if err := a.tykMCPService().TriggerSync(c.Request.Context(), actor, id); err != nil {
		tykMCPErrorResponse(c, err, "Failed to request sync")
		return
	}
	c.Status(http.StatusAccepted)
}

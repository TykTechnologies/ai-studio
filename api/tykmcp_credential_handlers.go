package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/gin-gonic/gin"
)

// --- Admin: /api/v1/mcp-credentials ---

// listMCPCredentials godoc
// @Summary List Tyk keys minted for Apps
// @Tags TykMCP
// @Produce json
// @Param app_id query int false "App"
// @Param connection_id query int false "Connection"
// @Param server_id query int false "Apps bound to this MCP server"
// @Param status query string false "status"
// @Param drift query string false "drift"
// @Param page query int false "page"
// @Param page_size query int false "page size"
// @Success 200 {object} tykmcp.CredentialList
// @Router /mcp-credentials [get]
func (a *API) listMCPCredentials(c *gin.Context) {
	f := tykmcp.CredentialFilter{Status: strings.TrimSpace(c.Query("status")), Drift: strings.TrimSpace(c.Query("drift"))}
	var ok bool
	if f.AppID, ok = queryUint(c, "app_id"); !ok {
		webhookBadRequest(c, "invalid app_id")
		return
	}
	if f.ConnectionID, ok = queryUint(c, "connection_id"); !ok {
		webhookBadRequest(c, "invalid connection_id")
		return
	}
	if f.ServerID, ok = queryUint(c, "server_id"); !ok {
		webhookBadRequest(c, "invalid server_id")
		return
	}
	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Page = n
		} else {
			webhookBadRequest(c, "invalid page")
			return
		}
	}
	if v := c.Query("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= tykmcp.MaxPageSize {
			f.PageSize = n
		} else {
			webhookBadRequest(c, "invalid page_size")
			return
		}
	}
	list, err := a.tykMCPService().ListCredentials(c.Request.Context(), f)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to list credentials")
		return
	}
	c.JSON(http.StatusOK, list)
}

// getMCPCredential godoc
// @Summary Get one minted credential (never the key)
// @Tags TykMCP
// @Produce json
// @Param id path string true "Credential ID"
// @Success 200 {object} models.MCPCredentialResponse
// @Router /mcp-credentials/{id} [get]
func (a *API) getMCPCredential(c *gin.Context) {
	view, err := a.tykMCPService().GetCredential(c.Request.Context(), c.Param("id"))
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to get credential")
		return
	}
	c.JSON(http.StatusOK, view)
}

// mintMCPCredential godoc
// @Summary Mint a Tyk key for an App on a connection (administrator)
// @Description The key is returned exactly once in this response and never stored.
// @Tags TykMCP
// @Accept json
// @Produce json
// @Param body body tykmcp.MintInput true "App and connection"
// @Success 201 {object} tykmcp.MintedCredential
// @Failure 422 {object} models.ErrorResponse
// @Router /mcp-credentials [post]
func (a *API) mintMCPCredential(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "mcp-credentials")
	if !ok {
		return
	}
	if !a.tykMCPService().Status().Available {
		tykMCPErrorResponse(c, tykmcp.ErrEnterpriseFeature, "")
		return
	}
	var in tykmcp.MintInput
	if err := c.ShouldBindJSON(&in); err != nil || in.AppID == 0 || in.ConnectionID == 0 {
		webhookBadRequest(c, "app_id and connection_id are required")
		return
	}
	minted, err := a.tykMCPService().MintCredential(c.Request.Context(), actor, in)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to mint credential")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, minted)
}

type mcpCredentialActionInput struct {
	Reason string `json:"reason"`
}

func (a *API) credentialAction(c *gin.Context, fallback string, fn func(actor tykmcp.Actor, id string, reason string) (*models.MCPCredentialResponse, error)) {
	actor, ok := requireTykMCPActor(c, "mcp-credentials")
	if !ok {
		return
	}
	var in mcpCredentialActionInput
	if err := bindOptionalJSON(c, &in); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	reason, ok := cleanReason(in.Reason, 255)
	if !ok {
		webhookBadRequest(c, "reason is too long")
		return
	}
	view, err := fn(actor, c.Param("id"), reason)
	if err != nil {
		tykMCPErrorResponse(c, err, fallback)
		return
	}
	c.JSON(http.StatusOK, view)
}

// rotateMCPCredential godoc
// @Summary Rotate a minted credential: mint a new key, revoke the old one
// @Tags TykMCP
// @Produce json
// @Param id path string true "Credential ID"
// @Success 201 {object} tykmcp.MintedCredential
// @Router /mcp-credentials/{id}/rotate [post]
func (a *API) rotateMCPCredential(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "mcp-credentials")
	if !ok {
		return
	}
	minted, err := a.tykMCPService().RotateCredential(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to rotate credential")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, minted)
}

// suspendMCPCredential godoc
// @Summary Switch a minted key off on the Dashboard
// @Tags TykMCP
// @Param id path string true "Credential ID"
// @Success 200 {object} models.MCPCredentialResponse
// @Router /mcp-credentials/{id}/suspend [post]
func (a *API) suspendMCPCredential(c *gin.Context) {
	a.credentialAction(c, "Failed to suspend credential", func(actor tykmcp.Actor, id, reason string) (*models.MCPCredentialResponse, error) {
		return a.tykMCPService().SuspendCredential(c.Request.Context(), actor, id, reason)
	})
}

// resumeMCPCredential godoc
// @Summary Switch a suspended key back on
// @Tags TykMCP
// @Param id path string true "Credential ID"
// @Success 200 {object} models.MCPCredentialResponse
// @Router /mcp-credentials/{id}/resume [post]
func (a *API) resumeMCPCredential(c *gin.Context) {
	a.credentialAction(c, "Failed to resume credential", func(actor tykmcp.Actor, id, _ string) (*models.MCPCredentialResponse, error) {
		return a.tykMCPService().ResumeCredential(c.Request.Context(), actor, id)
	})
}

// revokeMCPCredential godoc
// @Summary Revoke a minted key
// @Tags TykMCP
// @Param id path string true "Credential ID"
// @Success 200 {object} models.MCPCredentialResponse
// @Router /mcp-credentials/{id}/revoke [post]
func (a *API) revokeMCPCredential(c *gin.Context) {
	a.credentialAction(c, "Failed to revoke credential", func(actor tykmcp.Actor, id, reason string) (*models.MCPCredentialResponse, error) {
		return a.tykMCPService().RevokeCredential(c.Request.Context(), actor, id, reason)
	})
}

// applyMCPCredentialDrift godoc
// @Summary Apply a pending widening policy change to a key
// @Tags TykMCP
// @Param id path string true "Credential ID"
// @Success 200 {object} models.MCPCredentialResponse
// @Router /mcp-credentials/{id}/apply-drift [post]
func (a *API) applyMCPCredentialDrift(c *gin.Context) {
	a.credentialAction(c, "Failed to apply change", func(actor tykmcp.Actor, id, _ string) (*models.MCPCredentialResponse, error) {
		return a.tykMCPService().ApplyDrift(c.Request.Context(), actor, id)
	})
}

// getMCPAccessReport godoc
// @Summary Who has access to which MCP server
// @Tags TykMCP
// @Produce json
// @Param connection query int false "Connection"
// @Param server query int false "MCP server"
// @Param user query int false "User"
// @Param app query int false "App"
// @Param include_revoked query bool false "include closed grants"
// @Success 200 {array} tykmcp.AccessReportRow
// @Router /mcp-access-report [get]
func (a *API) getMCPAccessReport(c *gin.Context) {
	f := tykmcp.ReportFilter{IncludeRevoked: c.Query("include_revoked") == "true"}
	var ok bool
	for name, dst := range map[string]*uint{"connection": &f.ConnectionID, "server": &f.ServerID, "user": &f.UserID, "app": &f.AppID} {
		if *dst, ok = queryUint(c, name); !ok {
			webhookBadRequest(c, "invalid "+name)
			return
		}
	}
	rows, err := a.tykMCPService().AccessReport(c.Request.Context(), f)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to build the access report")
		return
	}
	if rows == nil {
		rows = []tykmcp.AccessReportRow{}
	}
	c.JSON(http.StatusOK, rows)
}

// --- Portal: /common/apps/:id/mcp/credentials ---

// portalAppForMCP loads the App and checks the caller owns it (or is an admin).
func (a *API) portalAppForMCP(c *gin.Context) (*models.App, *models.User, bool) {
	user, ok := portalUser(c)
	if !ok {
		return nil, nil, false
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", "Invalid app ID")
		return nil, nil, false
	}
	app, err := a.service.GetAppByID(uint(id))
	if err != nil || (app.UserID != user.ID && !user.IsAdmin) {
		simpleError(c, http.StatusNotFound, "Not Found", "App not found")
		return nil, nil, false
	}
	return app, user, true
}

type portalMintInput struct {
	ConnectionID uint `json:"connection_id"`
}

// mintUserAppMCPCredential godoc
// @Summary Request a Tyk access key for one of my apps
// @Description Mints a key for the App on the given connection. The key is shown once.
// @Tags common
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Param body body portalMintInput true "Connection"
// @Success 201 {object} tykmcp.MintedCredential
// @Router /common/apps/{id}/mcp/credentials [post]
func (a *API) mintUserAppMCPCredential(c *gin.Context) {
	app, user, ok := a.portalAppForMCP(c)
	if !ok {
		return
	}
	var in portalMintInput
	if err := c.ShouldBindJSON(&in); err != nil || in.ConnectionID == 0 {
		simpleError(c, http.StatusBadRequest, "Bad Request", "connection_id is required")
		return
	}
	actor := tykmcp.Actor{UserID: user.ID, Email: user.Email, Name: user.Name}
	minted, err := a.tykMCPService().MintCredential(c.Request.Context(), actor, tykmcp.MintInput{AppID: app.ID, ConnectionID: in.ConnectionID})
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to mint credential")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, minted)
}

// portalCredentialOwned checks the credential belongs to the caller's App.
func (a *API) portalCredentialOwned(c *gin.Context, app *models.App) (string, bool) {
	cid := c.Param("cid")
	view, err := a.tykMCPService().GetCredential(c.Request.Context(), cid)
	if err != nil || view.AppID != app.ID {
		simpleError(c, http.StatusNotFound, "Not Found", "Credential not found")
		return "", false
	}
	return cid, true
}

// rotateUserAppMCPCredential godoc
// @Summary Rotate the Tyk access key of one of my apps
// @Tags common
// @Produce json
// @Param id path int true "App ID"
// @Param cid path string true "Credential ID"
// @Success 201 {object} tykmcp.MintedCredential
// @Router /common/apps/{id}/mcp/credentials/{cid}/rotate [post]
func (a *API) rotateUserAppMCPCredential(c *gin.Context) {
	app, user, ok := a.portalAppForMCP(c)
	if !ok {
		return
	}
	cid, ok := a.portalCredentialOwned(c, app)
	if !ok {
		return
	}
	actor := tykmcp.Actor{UserID: user.ID, Email: user.Email, Name: user.Name}
	minted, err := a.tykMCPService().RotateCredential(c.Request.Context(), actor, cid)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to rotate credential")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, minted)
}

// revokeUserAppMCPCredential godoc
// @Summary Revoke the Tyk access key of one of my apps
// @Tags common
// @Produce json
// @Param id path int true "App ID"
// @Param cid path string true "Credential ID"
// @Success 200 {object} models.MCPCredentialResponse
// @Router /common/apps/{id}/mcp/credentials/{cid}/revoke [post]
func (a *API) revokeUserAppMCPCredential(c *gin.Context) {
	app, user, ok := a.portalAppForMCP(c)
	if !ok {
		return
	}
	cid, ok := a.portalCredentialOwned(c, app)
	if !ok {
		return
	}
	actor := tykmcp.Actor{UserID: user.ID, Email: user.Email, Name: user.Name}
	view, err := a.tykMCPService().RevokeCredential(c.Request.Context(), actor, cid, "revoked by the app owner")
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to revoke credential")
		return
	}
	c.JSON(http.StatusOK, view)
}

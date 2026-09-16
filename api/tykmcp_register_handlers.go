package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/gin-gonic/gin"
)

func isDryRun(c *gin.Context) bool {
	v, _ := strconv.ParseBool(c.Query("dry_run"))
	return v
}

// registerMCPServer godoc
// @Summary Create an MCP proxy on the Tyk Dashboard (full-mode connections)
// @Description With ?dry_run=1 the Dashboard validates the rendered definition and a masked preview is returned; otherwise the proxy is created and catalogued.
// @Tags TykMCP
// @Accept json
// @Produce json
// @Param dry_run query bool false "validate only"
// @Param body body tykmcp.RegisterInput true "Proxy to create"
// @Success 200 {object} tykmcp.RegisterPreview "dry run"
// @Success 201 {object} models.MCPServerResponse
// @Router /mcp-servers/register [post]
func (a *API) registerMCPServer(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "mcp-servers")
	if !ok {
		return
	}
	if !a.tykMCPService().Status().Available {
		tykMCPErrorResponse(c, tykmcp.ErrEnterpriseFeature, "")
		return
	}
	var in tykmcp.RegisterInput
	if err := c.ShouldBindJSON(&in); err != nil || in.ConnectionID == 0 {
		webhookBadRequest(c, "connection_id and the proxy fields are required")
		return
	}
	dryRun := isDryRun(c)
	preview, srv, err := a.tykMCPService().RegisterServer(c.Request.Context(), actor, in, dryRun)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to register MCP server")
		return
	}
	if dryRun {
		c.JSON(http.StatusOK, preview)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"server": srv, "warnings": preview.Warnings, "endpoint_url": preview.EndpointURL})
}

// pushMCPServer godoc
// @Summary Replace a server's definition on the Tyk Dashboard
// @Description Checks the live hash, restores masked secrets from the live document, then updates. ?dry_run=1 validates only.
// @Tags TykMCP
// @Accept json
// @Produce json
// @Param id path int true "Server ID"
// @Param dry_run query bool false "validate only"
// @Param body body tykmcp.PushInput true "Edited definition"
// @Success 200 {object} models.MCPServerResponse
// @Router /mcp-servers/{id}/push [post]
func (a *API) pushMCPServer(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "mcp-servers")
	if !ok {
		return
	}
	if !a.tykMCPService().Status().Available {
		tykMCPErrorResponse(c, tykmcp.ErrEnterpriseFeature, "")
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	var in tykmcp.PushInput
	if err := c.ShouldBindJSON(&in); err != nil || len(in.Definition) == 0 {
		webhookBadRequest(c, "definition is required")
		return
	}
	dryRun := isDryRun(c)
	preview, srv, err := a.tykMCPService().PushServer(c.Request.Context(), actor, id, in, dryRun)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to push MCP server definition")
		return
	}
	if dryRun {
		c.JSON(http.StatusOK, preview)
		return
	}
	c.JSON(http.StatusOK, gin.H{"server": srv, "warnings": preview.Warnings, "endpoint_url": preview.EndpointURL})
}

// listTykSourceAPIs godoc
// @Summary Tyk OAS APIs that can back a REST-to-MCP proxy
// @Tags TykMCP
// @Produce json
// @Param id path int true "Connection ID"
// @Param q query string false "search"
// @Success 200 {array} tykmcp.SourceAPI
// @Router /tyk-connections/{id}/apis [get]
func (a *API) listTykSourceAPIs(c *gin.Context) {
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	list, err := a.tykMCPService().ListSourceAPIs(c.Request.Context(), id, strings.TrimSpace(c.Query("q")))
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to list APIs")
		return
	}
	if list == nil {
		list = []tykmcp.SourceAPI{}
	}
	c.JSON(http.StatusOK, list)
}

// listTykSourceOperations godoc
// @Summary Operations of a Tyk OAS API
// @Tags TykMCP
// @Produce json
// @Param id path int true "Connection ID"
// @Param api_id path string true "Tyk API id"
// @Success 200 {array} tykmcp.SourceOperation
// @Router /tyk-connections/{id}/apis/{api_id}/operations [get]
func (a *API) listTykSourceOperations(c *gin.Context) {
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	list, err := a.tykMCPService().ListSourceOperations(c.Request.Context(), id, c.Param("api_id"))
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to list operations")
		return
	}
	if list == nil {
		list = []tykmcp.SourceOperation{}
	}
	c.JSON(http.StatusOK, list)
}

// listTykGatewayTags godoc
// @Summary Deployment targets known for a connection
// @Tags TykMCP
// @Produce json
// @Param id path int true "Connection ID"
// @Success 200 {array} tykmcp.GatewayTagOption
// @Router /tyk-connections/{id}/gateway-tags [get]
func (a *API) listTykGatewayTags(c *gin.Context) {
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	list, err := a.tykMCPService().GatewayTagOptions(c.Request.Context(), id)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to list gateway tags")
		return
	}
	c.JSON(http.StatusOK, list)
}

// createTykPolicy godoc
// @Summary Create a Studio-managed Tyk policy (minimal creator)
// @Tags TykMCP
// @Accept json
// @Produce json
// @Param id path int true "Connection ID"
// @Param body body tykmcp.PolicyInput true "Policy"
// @Success 201 {object} models.TykPolicyResponse
// @Router /tyk-connections/{id}/policies [post]
func (a *API) createTykPolicy(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "mcp-servers")
	if !ok {
		return
	}
	if !a.tykMCPService().Status().Available {
		tykMCPErrorResponse(c, tykmcp.ErrEnterpriseFeature, "")
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	var in tykmcp.PolicyInput
	if err := c.ShouldBindJSON(&in); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	pol, err := a.tykMCPService().CreatePolicy(c.Request.Context(), actor, id, in)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to create policy")
		return
	}
	c.JSON(http.StatusCreated, pol)
}

// updateTykPolicy godoc
// @Summary Edit a Studio-managed Tyk policy
// @Tags TykMCP
// @Accept json
// @Produce json
// @Param id path int true "Connection ID"
// @Param pid path string true "Tyk policy id"
// @Param body body tykmcp.PolicyInput true "Name and limits"
// @Success 200 {object} models.TykPolicyResponse
// @Router /tyk-connections/{id}/policies/{pid} [patch]
func (a *API) updateTykPolicy(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "mcp-servers")
	if !ok {
		return
	}
	if !a.tykMCPService().Status().Available {
		tykMCPErrorResponse(c, tykmcp.ErrEnterpriseFeature, "")
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	var in tykmcp.PolicyInput
	if err := c.ShouldBindJSON(&in); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	pol, err := a.tykMCPService().UpdatePolicy(c.Request.Context(), actor, id, c.Param("pid"), in)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to update policy")
		return
	}
	c.JSON(http.StatusOK, pol)
}

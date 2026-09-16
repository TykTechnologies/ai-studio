package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/gin-gonic/gin"
)

func queryUint(c *gin.Context, name string) (uint, bool) {
	v := strings.TrimSpace(c.Query(name))
	if v == "" {
		return 0, true
	}
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint(n), true
}

// listMCPServers godoc
// @Summary List MCP servers
// @Description MCP proxies catalogued from Tyk Dashboards, with filters and paging
// @Tags TykMCP
// @Produce json
// @Param connection_id query int false "Connection"
// @Param state query string false "dashboard_state"
// @Param origin query string false "origin"
// @Param kind query string false "kind"
// @Param published query bool false "published in the portal"
// @Param q query string false "search"
// @Param page query int false "page (1-based)"
// @Param page_size query int false "page size"
// @Success 200 {object} tykmcp.ServerList
// @Router /mcp-servers [get]
func (a *API) listMCPServers(c *gin.Context) {
	f := tykmcp.ServerFilter{
		State:  strings.TrimSpace(c.Query("state")),
		Origin: strings.TrimSpace(c.Query("origin")),
		Kind:   strings.TrimSpace(c.Query("kind")),
		Search: strings.TrimSpace(c.Query("q")),
	}
	var ok bool
	if f.ConnectionID, ok = queryUint(c, "connection_id"); !ok {
		webhookBadRequest(c, "invalid connection_id")
		return
	}
	if v := c.Query("published"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			webhookBadRequest(c, "invalid published")
			return
		}
		f.Published = &b
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
	list, err := a.tykMCPService().ListServers(c.Request.Context(), f)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to list MCP servers")
		return
	}
	c.JSON(http.StatusOK, list)
}

// getMCPServer godoc
// @Summary Get an MCP server
// @Tags TykMCP
// @Produce json
// @Param id path int true "Server ID"
// @Success 200 {object} models.MCPServerResponse
// @Router /mcp-servers/{id} [get]
func (a *API) getMCPServer(c *gin.Context) {
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	view, err := a.tykMCPService().GetServer(c.Request.Context(), id)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to get MCP server")
		return
	}
	c.JSON(http.StatusOK, view)
}

// updateMCPServer godoc
// @Summary Update an MCP server's presentation and governance fields
// @Tags TykMCP
// @Accept json
// @Produce json
// @Param id path int true "Server ID"
// @Param body body tykmcp.ServerPatch true "Patch"
// @Success 200 {object} models.MCPServerResponse
// @Router /mcp-servers/{id} [patch]
func (a *API) updateMCPServer(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "mcp-servers")
	if !ok {
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	var p tykmcp.ServerPatch
	if err := c.ShouldBindJSON(&p); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	view, err := a.tykMCPService().UpdateServer(c.Request.Context(), actor, id, p)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to update MCP server")
		return
	}
	c.JSON(http.StatusOK, view)
}

// deleteMCPServer godoc
// @Summary Delete a missing or pending MCP server record
// @Tags TykMCP
// @Param id path int true "Server ID"
// @Success 204
// @Router /mcp-servers/{id} [delete]
func (a *API) deleteMCPServer(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "mcp-servers")
	if !ok {
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	force, _ := strconv.ParseBool(c.Query("force"))
	if err := a.tykMCPService().DeleteServer(c.Request.Context(), actor, id, force); err != nil {
		tykMCPErrorResponse(c, err, "Failed to delete MCP server")
		return
	}
	c.Status(http.StatusNoContent)
}

// publishMCPServer godoc
// @Summary Publish an MCP server to the portal
// @Tags TykMCP
// @Param id path int true "Server ID"
// @Success 200 {object} models.MCPServerResponse
// @Router /mcp-servers/{id}/activate [post]
func (a *API) publishMCPServer(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "mcp-servers")
	if !ok {
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	view, err := a.tykMCPService().PublishServer(c.Request.Context(), actor, id)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to publish MCP server")
		return
	}
	c.JSON(http.StatusOK, view)
}

// unpublishMCPServer godoc
// @Summary Unpublish an MCP server from the portal
// @Tags TykMCP
// @Param id path int true "Server ID"
// @Success 200 {object} models.MCPServerResponse
// @Router /mcp-servers/{id}/deactivate [post]
func (a *API) unpublishMCPServer(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "mcp-servers")
	if !ok {
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	view, err := a.tykMCPService().UnpublishServer(c.Request.Context(), actor, id)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to unpublish MCP server")
		return
	}
	c.JSON(http.StatusOK, view)
}

type mcpServerGroupsInput struct {
	GroupIDs []uint `json:"group_ids"`
}

// setMCPServerGroups godoc
// @Summary Set the teams that can see an MCP server
// @Tags TykMCP
// @Accept json
// @Param id path int true "Server ID"
// @Success 200 {object} models.MCPServerResponse
// @Router /mcp-servers/{id}/groups [put]
func (a *API) setMCPServerGroups(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "mcp-servers")
	if !ok {
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	var in mcpServerGroupsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	view, err := a.tykMCPService().SetServerGroups(c.Request.Context(), actor, id, in.GroupIDs)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to set MCP server teams")
		return
	}
	c.JSON(http.StatusOK, view)
}

type mcpServerBundleInput struct {
	Pins []tykmcp.PinInput `json:"pins"`
}

// setMCPServerBundle godoc
// @Summary Pin the policy bundle of an MCP server
// @Description One access policy (ACL) plus optional consumption policies (rate limit / quota partitions), validated against Tyk's partition rules
// @Tags TykMCP
// @Accept json
// @Param id path int true "Server ID"
// @Success 200 {object} models.MCPServerResponse
// @Failure 422 {object} models.ErrorResponse
// @Router /mcp-servers/{id}/bundle [put]
func (a *API) setMCPServerBundle(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "mcp-servers")
	if !ok {
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	var in mcpServerBundleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	view, err := a.tykMCPService().SetServerBundle(c.Request.Context(), actor, id, in.Pins)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to set MCP server bundle")
		return
	}
	c.JSON(http.StatusOK, view)
}

// listTykPolicies godoc
// @Summary List cached Tyk policies of a connection
// @Tags TykMCP
// @Produce json
// @Param id path int true "Connection ID"
// @Param mcp_only query bool false "only policies granting an MCP proxy"
// @Param api_id query string false "only policies granting this api id"
// @Param q query string false "search"
// @Success 200 {array} models.TykPolicyResponse
// @Router /tyk-connections/{id}/policies [get]
func (a *API) listTykPolicies(c *gin.Context) {
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	f := tykmcp.PolicyFilter{APIID: strings.TrimSpace(c.Query("api_id")), Search: strings.TrimSpace(c.Query("q"))}
	f.MCPOnly = c.Query("mcp_only") == "true"
	list, err := a.tykMCPService().ListPolicies(c.Request.Context(), id, f)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to list policies")
		return
	}
	if list == nil {
		list = []models.TykPolicyResponse{}
	}
	c.JSON(http.StatusOK, list)
}

// listTykSyncRuns godoc
// @Summary List recent sync runs of a connection
// @Tags TykMCP
// @Produce json
// @Param id path int true "Connection ID"
// @Param limit query int false "max rows"
// @Success 200 {array} models.MCPSyncRun
// @Router /tyk-connections/{id}/sync-runs [get]
func (a *API) listTykSyncRuns(c *gin.Context) {
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	limit := 0
	if v := c.Query("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	runs, err := a.tykMCPService().ListSyncRuns(c.Request.Context(), id, limit)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to list sync runs")
		return
	}
	c.JSON(http.StatusOK, runs)
}

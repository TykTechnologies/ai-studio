package api

import (
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/gin-gonic/gin"
)

// The Tools import wizard imports OpenAPI definitions from a Tyk Dashboard
// through the same Tyk connections the MCP integration uses. These routes
// are scoped to the tools permission (importing creates a tool) so a tool
// author never needs the Sensitive tyk-connections permission; they expose a
// slim projection of each connection and nothing secret. CE answers 403
// Enterprise Feature through the community stub, like every tykmcp route.

// ToolImportConnection is what the Tools import wizard needs to know about
// a connection: enough to pick one and to explain why it may not work.
type ToolImportConnection struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	DashboardURL   string `json:"dashboard_url"`
	Status         string `json:"status"`
	Degraded       bool   `json:"degraded"`
	DegradedReason string `json:"degraded_reason,omitempty"`
	EffectiveMode  string `json:"effective_mode"`
	// APIsRead is the probed state of the apis_read capability
	// (ok | denied | unverified | no), empty when never probed.
	APIsRead string `json:"apis_read"`
}

func toolImportConnection(c models.TykConnectionResponse) ToolImportConnection {
	out := ToolImportConnection{
		ID: c.ID, Name: c.Name, DashboardURL: c.DashboardURL, Status: c.Status,
		Degraded: c.Degraded, DegradedReason: c.DegradedReason, EffectiveMode: c.EffectiveMode,
	}
	if cap, ok := c.Capabilities[models.TykCapAPIsRead]; ok {
		out.APIsRead = cap.State
	}
	return out
}

// listToolImportConnections godoc
// @Summary Tyk connections the Tools import can read APIs from
// @Description Slim projection of every non-disabled Tyk connection, for the Tools import wizard.
// @Tags Tools
// @Produce json
// @Success 200 {array} ToolImportConnection
// @Failure 403 {object} models.ErrorResponse "Enterprise feature"
// @Router /tools/import/tyk/connections [get]
func (a *API) listToolImportConnections(c *gin.Context) {
	list, err := a.tykMCPService().ListConnections(c.Request.Context())
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to list connections")
		return
	}
	out := []ToolImportConnection{}
	for _, conn := range list {
		if conn.Status == models.TykConnectionDisabled {
			continue
		}
		out = append(out, toolImportConnection(conn))
	}
	c.JSON(http.StatusOK, out)
}

// listToolImportAPIs godoc
// @Summary Tyk OAS APIs on a connection
// @Tags Tools
// @Produce json
// @Param id path int true "Connection ID"
// @Param q query string false "search"
// @Success 200 {array} tykmcp.SourceAPI
// @Router /tools/import/tyk/connections/{id}/apis [get]
func (a *API) listToolImportAPIs(c *gin.Context) {
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

// getToolImportAPIDocument godoc
// @Summary One Tyk OAS API definition, credentials masked
// @Tags Tools
// @Produce json
// @Param id path int true "Connection ID"
// @Param api_id path string true "Tyk API id"
// @Success 200 {object} tykmcp.SourceAPIDocument
// @Router /tools/import/tyk/connections/{id}/apis/{api_id} [get]
func (a *API) getToolImportAPIDocument(c *gin.Context) {
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	doc, err := a.tykMCPService().GetSourceAPIDocument(c.Request.Context(), id, c.Param("api_id"))
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to fetch the API definition")
		return
	}
	c.JSON(http.StatusOK, doc)
}

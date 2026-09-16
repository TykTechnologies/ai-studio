package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/gin-gonic/gin"
)

// listPortalMCPConnections godoc
// @Summary Tyk connections a portal user may submit an MCP server to
// @Tags common
// @Produce json
// @Success 200 {array} tykmcp.SubmissionConnection
// @Router /common/mcp/connections [get]
func (a *API) listPortalMCPConnections(c *gin.Context) {
	if _, ok := portalUser(c); !ok {
		return
	}
	list, err := a.tykMCPService().SubmissionConnections(c.Request.Context())
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to list connections")
		return
	}
	if list == nil {
		list = []tykmcp.SubmissionConnection{}
	}
	c.JSON(http.StatusOK, list)
}

// testMCPSubmission validates an mcp_server submission: a Dashboard dry run
// on a full-mode connection, nothing else. The submitter's upstream URL is
// never contacted by Studio.
func (a *API) testMCPSubmission(c *gin.Context, submission *models.Submission) {
	actor, ok := requireTykMCPActor(c, "mcp-servers")
	if !ok {
		return
	}
	in := tykmcp.RegisterInputFromPayload(submission.ResourcePayload)
	conn, err := a.tykMCPService().GetConnection(c.Request.Context(), in.ConnectionID)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to load the connection")
		return
	}
	if conn.EffectiveMode != models.TykConnectionModeFull {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"type":         "mcp_server",
			"skipped":      true,
			"check_kind":   "none",
			"check_label":  "No Dashboard validation",
			"check_detail": "Connection " + conn.Name + " runs in " + conn.EffectiveMode + " mode, so the platform team creates this proxy from the handoff package; the Dashboard validates it then.",
		}})
		return
	}
	preview, _, err := a.tykMCPService().RegisterServer(c.Request.Context(), actor, in, true)
	if err != nil {
		tykMCPErrorResponse(c, err, "The Tyk Dashboard rejected the definition")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"type":         "mcp_server",
		"check_kind":   CheckSpecValidation,
		"check_label":  "Definition validated by the Tyk Dashboard",
		"check_detail": "The rendered MCP proxy definition passed the Dashboard's validation. The upstream MCP server was not contacted.",
		"preview":      preview,
	}})
}

// getMCPServerHandoff godoc
// @Summary Handoff package for a server awaiting the platform team
// @Description ?include_secrets=true adds the upstream credential and needs the execute permission; the download is audited.
// @Tags TykMCP
// @Produce json
// @Param id path int true "Server ID"
// @Param include_secrets query bool false "include the upstream credential"
// @Success 200 {object} tykmcp.HandoffPackage
// @Router /mcp-servers/{id}/handoff [get]
func (a *API) getMCPServerHandoff(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "mcp-servers")
	if !ok {
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	include, _ := strconv.ParseBool(c.Query("include_secrets"))
	pkg, err := a.tykMCPService().HandoffPackage(c.Request.Context(), actor, id, include)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to build the handoff package")
		return
	}
	if include {
		c.Header("Cache-Control", "no-store")
	}
	c.JSON(http.StatusOK, pkg)
}

type linkMCPServerInput struct {
	TykAPIID string `json:"tyk_api_id"`
}

// linkMCPServer godoc
// @Summary Link a pending server to the proxy the platform team created
// @Tags TykMCP
// @Accept json
// @Produce json
// @Param id path int true "Pending server ID"
// @Param body body linkMCPServerInput true "Imported proxy"
// @Success 200 {object} models.MCPServerResponse
// @Router /mcp-servers/{id}/link [post]
func (a *API) linkMCPServer(c *gin.Context) {
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
	var in linkMCPServerInput
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.TykAPIID) == "" {
		webhookBadRequest(c, "tyk_api_id is required")
		return
	}
	srv, err := a.tykMCPService().LinkServer(c.Request.Context(), actor, id, in.TykAPIID)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to link the server")
		return
	}
	c.JSON(http.StatusOK, srv)
}

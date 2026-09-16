package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// mcpServerCatalogItem renders a Tyk-managed MCP server for the portal
// catalog. The upstream URL, the definition and the connection token never
// appear here; what a developer needs is the endpoint, the auth mode and
// the primitives.
func mcpServerCatalogItem(srv *models.MCPServer) CatalogItem {
	auth := srv.AuthDetails()
	brokerable := srv.Brokerable
	tags := srv.GatewayTags().Tags
	if tags == nil {
		tags = []string{}
	}
	prims := srv.Primitives()
	for i := range prims {
		prims[i].Description = cleanText(prims[i].Description)
	}
	item := CatalogItem{Type: CatalogItemMCPServer, ID: uintID(srv.ID), Attributes: CatalogItemAttributes{
		Name:                cleanText(srv.Name),
		ShortDescription:    cleanText(srv.Description),
		LongDescription:     cleanText(srv.LongDescription),
		LogoURL:             srv.LogoURL,
		Kind:                srv.Kind,
		KindLabel:           mcpKindLabel(srv.Kind),
		PrivacyScore:        srv.PrivacyScore,
		CommunitySubmitted:  srv.CommunitySubmitted,
		Tags:                cleanTexts(srv.Tags()),
		Catalogs:            []CatalogRef{},
		CreatedAt:           timePtr(srv.CreatedAt),
		UpdatedAt:           timePtr(srv.UpdatedAt),
		AccessGrantedViaApp: true,
		AuthMode:            srv.AuthMode,
		AuthHeader:          auth.HeaderName,
		EndpointURL:         srv.EndpointURL,
		EndpointURLs:        srv.EndpointURLs(),
		Primitives:          prims,
		GatewayTags:         tags,
		Brokerable:          &brokerable,
		OAuth:               auth.PRM,
	}}
	if item.Attributes.EndpointURLs == nil {
		item.Attributes.EndpointURLs = map[string]string{}
	}
	return item
}

func mcpKindLabel(kind string) string {
	switch kind {
	case models.MCPServerKindRestToMCP:
		return "REST API to MCP"
	case models.MCPServerKindRemote:
		return "Remote MCP server"
	}
	return kind
}

// getPortalCatalogMCPServer godoc
// @Summary One MCP server from the portal catalog
// @Description The catalog entry for a Tyk-managed MCP server the user can see: endpoint, auth mode, primitives.
// @Tags common
// @Produce json
// @Param id path int true "MCP server ID"
// @Success 200 {object} CatalogItemResponse
// @Failure 404 {object} ErrorResponse
// @Router /common/catalog/mcp-servers/{id} [get]
func (a *API) getPortalCatalogMCPServer(c *gin.Context) {
	item, ok := a.findCatalogItem(c, CatalogItemMCPServer, c.Param("id"))
	if !ok {
		return
	}
	c.JSON(http.StatusOK, CatalogItemResponse{Data: *item})
}

// mcpBindingError maps App binding refusals onto the API error envelope.
// Returns false when err was not one of them.
func mcpBindingError(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, services.ERRPrivacyScoreMismatch):
		simpleError(c, http.StatusBadRequest, "Privacy Score Mismatch", err.Error())
	case errors.Is(err, services.ErrMCPServerNotVisible):
		simpleError(c, http.StatusForbidden, "Forbidden", "User does not have access to one or more specified MCP servers")
	default:
		return false
	}
	return true
}

// AppMCPServerOutput is the slim projection of a bound MCP server in App
// responses.
type AppMCPServerOutput struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	ConnectionID *uint  `json:"connection_id"`
	AuthMode     string `json:"auth_mode"`
	EndpointURL  string `json:"endpoint_url"`
	Brokerable   bool   `json:"brokerable"`
}

func appMCPServerOutputs(servers []models.MCPServer) ([]uint, []AppMCPServerOutput) {
	ids := make([]uint, 0, len(servers))
	out := make([]AppMCPServerOutput, 0, len(servers))
	for _, s := range servers {
		ids = append(ids, s.ID)
		out = append(out, AppMCPServerOutput{ID: s.ID, Name: s.Name, Slug: s.Slug, ConnectionID: s.ConnectionID, AuthMode: s.AuthMode, EndpointURL: s.EndpointURL, Brokerable: s.Brokerable})
	}
	return ids, out
}

// getUserAppMCP godoc
// @Summary The MCP servers an App reaches and their grant state
// @Tags common
// @Produce json
// @Param id path int true "App ID"
// @Success 200 {object} tykmcp.AppMCPSummary
// @Failure 404 {object} ErrorResponse
// @Router /common/apps/{id}/mcp [get]
func (a *API) getUserAppMCP(c *gin.Context) {
	user, ok := portalUser(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", "Invalid app ID")
		return
	}
	app, err := a.service.GetAppByID(uint(id))
	if err != nil || (app.UserID != user.ID && !user.IsAdmin) {
		simpleError(c, http.StatusNotFound, "Not Found", "App not found")
		return
	}
	summary, err := a.tykMCPService().AppMCPSummary(c.Request.Context(), app.ID)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to load MCP access")
		return
	}
	c.JSON(http.StatusOK, summary)
}

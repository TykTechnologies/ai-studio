package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/gin-gonic/gin"
)

// Workspaces B4f: LLM, data and tool catalogues are fully scoped (hidden,
// 404, outside the caller's Workspaces). Route middleware applies the
// object checks to every route on a catalogue named by :id; the handlers
// apply the landing rule on create, the move rule on update and the list
// scopes. Portal and chat visibility (group_access, the portal catalog)
// is unchanged.

func catalogueNotFound(c *gin.Context) {
	simpleError(c, http.StatusNotFound, "Not Found", "Catalogue not found")
}

func dataCatalogueNotFound(c *gin.Context) {
	simpleError(c, http.StatusNotFound, "Not Found", "Data catalogue not found")
}

func toolCatalogueNotFound(c *gin.Context) {
	simpleError(c, http.StatusNotFound, "Not Found", "Tool Catalogue not found")
}

// requireMemberRead: adding a building block (an LLM, datasource, tool or
// MCP server; read-global, D4) to a catalogue needs read on it in some
// scope.
func (a *API) requireMemberRead(c *gin.Context, resource string) bool {
	if a.workspaceCheckMode(c) == workspaceOpen {
		return true
	}
	if authz.CanAnywhere(c, authz.Read(resource)) {
		return true
	}
	c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(authz.Read(resource)))
	return false
}

// moveIfRequested completes an update of a catalogue stored in stored: a
// requested workspace_id that differs is a move, checked (requireMove)
// before the update is written; the caller writes the move after.
func (a *API) catalogueMoveTarget(c *gin.Context, resource string, stored uint) (*uint, bool) {
	requested, ok := boundWorkspaceID(c)
	if !ok {
		return nil, false
	}
	return a.checkObjectUpdate(c, resource, stored, requested)
}

// Team (group) <-> catalogue links (B4f). Teams are global; their links to
// catalogues follow the catalogue's visibility: a caller can link or unlink
// only catalogues they can see, a save keeps links to catalogues they
// cannot see, and responses leave hidden catalogues out. Portal access
// (group_access) reads the links as stored and is unchanged.

// catalogueKind names one catalogue type for the team-link helpers.
type catalogueKind struct {
	resource string
	model    interface{}
	notFound string
}

var (
	llmCatalogueKind  = catalogueKind{"catalogues", &models.Catalogue{}, "Catalogue not found"}
	dataCatalogueKind = catalogueKind{"data-catalogues", &models.DataCatalogue{}, "Data catalogue not found"}
	toolCatalogueKind = catalogueKind{"tool-catalogues", &models.ToolCatalogue{}, "Tool catalogue not found"}
)

// requireCatalogueVisible answers 404 (and returns false) when the
// catalogue does not exist or the caller cannot see it.
func (a *API) requireCatalogueVisible(c *gin.Context, k catalogueKind, id uint) bool {
	if !a.workspaceChecksOn(c) {
		return true
	}
	ws, found, err := a.storedWorkspaceID(k.model, id)
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		c.Abort()
		return false
	}
	if !found || !a.visibleIn(c, k.resource, ws) {
		if !c.IsAborted() {
			simpleError(c, http.StatusNotFound, "Not Found", k.notFound)
			c.Abort()
		}
		return false
	}
	return true
}

// mergeTeamCatalogueLinks is mergeVisibleLinks for one catalogue kind of a
// team: the visible part of submitted plus the hidden part of stored.
func (a *API) mergeTeamCatalogueLinks(c *gin.Context, k catalogueKind, submitted, stored []uint) ([]uint, bool) {
	return a.mergeVisibleLinks(c, k.resource, k.model, submitted, stored, k.notFound)
}

// visibleDataCatalogues drops the data catalogues the caller cannot see.
func (a *API) visibleDataCatalogues(c *gin.Context, cats []models.DataCatalogue) []models.DataCatalogue {
	out := make([]models.DataCatalogue, 0, len(cats))
	for _, x := range cats {
		if a.visibleIn(c, "data-catalogues", x.WorkspaceID) {
			out = append(out, x)
		}
	}
	return out
}

// visibleToolCatalogues drops the tool catalogues the caller cannot see.
func (a *API) visibleToolCatalogues(c *gin.Context, cats []models.ToolCatalogue) []models.ToolCatalogue {
	out := make([]models.ToolCatalogue, 0, len(cats))
	for _, x := range cats {
		if a.visibleIn(c, "tool-catalogues", x.WorkspaceID) {
			out = append(out, x)
		}
	}
	return out
}

// hideTeamCatalogues drops, before a team is serialized, the catalogues the
// caller cannot see.
func (a *API) hideTeamCatalogues(c *gin.Context, g *models.Group) {
	if g == nil || !a.workspaceChecksOn(c) {
		return
	}
	g.Catalogues = a.visibleCatalogues(c, g.Catalogues)
	g.DataCatalogues = a.visibleDataCatalogues(c, g.DataCatalogues)
	g.ToolCatalogues = a.visibleToolCatalogues(c, g.ToolCatalogues)
}

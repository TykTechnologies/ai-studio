package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
)

// Link-replacing saves under Workspaces. A save that replaces a set of
// links (a router's catalogues, a catalogue's routers) must not let the
// caller name an object they cannot see, and must not remove links to
// objects they cannot see (they cannot know those links exist). The set
// written is the visible part of the request plus the hidden part of what
// is stored.

// workspaceOf reads the stored workspace_id of the given objects of model
// (soft-deleted ones excluded); ids absent from the result do not exist.
func (a *API) workspaceOf(model interface{}, ids []uint) (map[uint]uint, error) {
	out := map[uint]uint{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID          uint
		WorkspaceID uint
	}
	if err := a.service.DB.Model(model).Select("id", "workspace_id").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = r.WorkspaceID
	}
	return out, nil
}

// linkedIDs reads the ids linked to owner through a join table.
func (a *API) linkedIDs(table, ownerCol, otherCol string, owner uint) ([]uint, error) {
	var ids []uint
	err := a.service.DB.Table(table).Where(ownerCol+" = ?", owner).Pluck(otherCol, &ids).Error
	return ids, err
}

// mergeVisibleLinks validates submitted links of resource (stored as model)
// and keeps the hidden part of the existing ones. A submitted id that does
// not exist or that the caller cannot see answers 404 with notFound (the
// same answer for both). It returns the set to write and false when it has
// answered.
func (a *API) mergeVisibleLinks(c *gin.Context, resource string, model interface{}, submitted, existing []uint, notFound string) ([]uint, bool) {
	if !a.workspaceChecksOn(c) {
		return submitted, true // TestMode or legacy: the service validates, as before
	}
	all := append(append([]uint{}, submitted...), existing...)
	ws, err := a.workspaceOf(model, all)
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return nil, false
	}
	seen := map[uint]bool{}
	out := make([]uint, 0, len(all))
	for _, id := range submitted {
		w, ok := ws[id]
		if !ok || !a.visibleIn(c, resource, w) {
			if !c.IsAborted() {
				simpleError(c, http.StatusNotFound, "Not Found", notFound)
			}
			return nil, false
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range existing {
		w, ok := ws[id]
		if ok && !seen[id] && !a.visibleIn(c, resource, w) {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, !c.IsAborted()
}

// visibleCatalogues drops the LLM catalogues the caller cannot see, so a
// router response never names them.
func (a *API) visibleCatalogues(c *gin.Context, cats []models.Catalogue) []models.Catalogue {
	out := make([]models.Catalogue, 0, len(cats))
	for _, cat := range cats {
		if a.visibleIn(c, "catalogues", cat.WorkspaceID) {
			out = append(out, cat)
		}
	}
	return out
}

// holdsAnywhere reports whether the caller holds p globally or in any
// Workspace, open in TestMode and for legacy callers like holds. It is the
// read rule for read-global building blocks (D4).
func (a *API) holdsAnywhere(c *gin.Context, p authz.Permission) bool {
	switch a.workspaceCheckMode(c) {
	case workspaceOpen:
		return true
	case workspaceDeny:
		return false
	}
	return authz.CanAnywhere(c, p)
}

// requireRouterLLMRead: a router whose pools name LLMs needs llms:read in
// some scope (LLMs are read-global, D4). 403 otherwise.
func (a *API) requireRouterLLMRead(c *gin.Context, namesLLMs bool) bool {
	if !namesLLMs || a.holdsAnywhere(c, authz.Read("llms")) {
		return true
	}
	c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(authz.Read("llms")))
	return false
}

// workspaceChecksOn reports whether the Workspace object checks apply to
// the request (false in TestMode and for legacy callers, where the helpers
// allow everything). Handlers use it to skip lookups that exist only for
// those checks.
func (a *API) workspaceChecksOn(c *gin.Context) bool {
	return a.workspaceCheckMode(c) != workspaceOpen
}

// workspaceListFilters returns the list scopes for resource: the caller's
// Workspace scope and the ?workspace_id= filter, or none when neither
// filters anything (so a list keeps its unfiltered path). false when it has
// answered 400.
func (a *API) workspaceListFilters(c *gin.Context, resource string) ([]func(*gorm.DB) *gorm.DB, bool) {
	var out []func(*gorm.DB) *gorm.DB
	if _, present := c.GetQuery("workspace_id"); present {
		filter, ok := a.workspaceFilterParam(c, "")
		if !ok {
			return nil, false
		}
		out = append(out, filter)
	}
	if a.workspaceChecksOn(c) && authz.ResourceScope(resource).ReadScoped() {
		if all, _ := authz.WorkspacesWith(c, authz.Read(resource)); !all {
			out = append(out, a.workspaceListScope(c, resource, ""))
		}
	}
	return out, true
}

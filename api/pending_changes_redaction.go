package api

import (
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// pendingTypeResource maps a stamped pending-change type to its catalogue
// resource.
var pendingTypeResource = map[string]string{
	"llm":             "llms",
	"app":             "apps",
	"filter":          "filters",
	"tool":            "tools",
	"datasource":      "datasources",
	"model_router":    "model-routers",
	"semantic_router": "semantic-routers",
	"embedder":        "embedders",
}

// redactPendingChanges hides from a Workspace-scoped caller what it could
// not otherwise see (midsommar-clean-oztk.44): the id and name of a change
// on a read-scoped type (Apps, routers) in a Workspace its read there does
// not cover, and the names of Workspaces it has no binding in. Counts are
// left as they are (the accepted exception), and read-global and unstamped
// types keep their names (D4). A caller with global access sees everything.
func (a *API) redactPendingChanges(c *gin.Context, pc *services.PendingChanges) {
	if pc == nil || a.workspaceCheckMode(c) == workspaceOpen {
		return
	}
	named := map[uint]bool{}
	mayName := func(ws uint) bool {
		ok, seen := named[ws]
		if !seen {
			ok = a.mayNameWorkspace(c, ws)
			named[ws] = ok
		}
		return ok
	}
	for i := range pc.Changes {
		ch := &pc.Changes[i]
		if ch.WorkspaceID == nil {
			continue
		}
		if res, ok := pendingTypeResource[ch.Type]; ok && !a.visibleIn(c, res, *ch.WorkspaceID) {
			ch.ID, ch.Name, ch.Hidden = 0, "", true
		}
		if !mayName(*ch.WorkspaceID) {
			ch.WorkspaceName = ""
		}
	}
	for i := range pc.ByWorkspace {
		if !mayName(pc.ByWorkspace[i].WorkspaceID) {
			pc.ByWorkspace[i].Name = ""
		}
	}
}

// mayNameWorkspace reports whether the caller may see a Workspace's name:
// with global workspaces:read, or a binding scoped to that Workspace. A
// global role without workspaces:read (the Operator's edges role) names no
// Workspace it is not bound in.
func (a *API) mayNameWorkspace(c *gin.Context, workspaceID uint) bool {
	switch a.workspaceCheckMode(c) {
	case workspaceOpen:
		return true
	case workspaceDeny:
		return false
	}
	x, ok := authz.FromContext(c)
	if !ok {
		return false
	}
	g, err := x.Grants()
	if err != nil {
		return false
	}
	if g.Can(authz.Read("workspaces")) {
		return true
	}
	ws, ok := g.Workspaces[workspaceID]
	return ok && !ws.IsEmpty()
}

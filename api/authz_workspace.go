package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
)

// Workspace object checks (Workspaces B3). The route gate only decides
// whether the caller may use a capability somewhere; on a Workspace-checked
// route (permRouter.InWorkspace) the handler then checks every object it
// touches with these helpers:
//
//   - requireVisible after loading an object (404 when the caller's read does
//     not cover its Workspace, for read-scoped resources);
//   - requireInWorkspace before changing it (403 permission_denied);
//   - workspaceListScope (and workspaceFilterParam) on lists;
//   - resolveCreateWorkspace on create (the landing rule, decision D5);
//   - requireMove when an update changes the object's Workspace;
//   - requirePublishIfChangedIn / requirePublishToCreateLiveIn for the live
//     switch, and holdsIn for any other in-handler permission check.
//
// Checks on an object take its stored workspace_id, read from the database
// (never from a per-replica cache) and passed as is: 0, written by a build
// from before Workspaces, is the Default workspace (.41), which the helpers
// apply themselves. For a ScopeWorkspaceParent resource (App credentials)
// pass the parent App's workspace_id.
//
// Like holds (authz_publish.go), every helper allows everything in TestMode
// and for a request without an authorization context that did not pass the
// route gate (legacy callers, handler tests). A request that passed the gate,
// or any /api/v1 request, without a context fails closed (.37). In
// Community Edition the grants are global only, so every check reduces to
// the admin-or-not rule.

// authzGatedKey marks a request that passed requireRoutePermission.
const authzGatedKey = "authz.gated"

type workspaceCheckMode int

const (
	workspaceCheck workspaceCheckMode = iota // evaluate the caller's grants
	workspaceOpen                            // TestMode or legacy: allow
	workspaceDeny                            // no context where one is required: deny
)

func (a *API) workspaceCheckMode(c *gin.Context) workspaceCheckMode {
	if a.config != nil && a.config.TestMode {
		return workspaceOpen
	}
	if _, ok := authz.FromContext(c); ok {
		return workspaceCheck
	}
	if _, gated := c.Get(authzGatedKey); gated {
		return workspaceDeny
	}
	if c.Request != nil && strings.HasPrefix(c.Request.URL.Path, "/api/v1/") {
		return workspaceDeny
	}
	return workspaceOpen
}

// effectiveWorkspaceID maps a stored workspace_id to the Workspace it means
// (0 is the Default). It answers 500 and false when the Default cannot be
// loaded.
func (a *API) effectiveWorkspaceID(c *gin.Context, workspaceID uint) (uint, bool) {
	if workspaceID != 0 {
		return workspaceID, true
	}
	def, err := a.service.DefaultWorkspaceID()
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		c.Abort()
		return 0, false
	}
	return models.EffectiveWorkspaceID(workspaceID, def), true
}

// holdsIn reports whether the caller holds p in the Workspace of an object
// whose stored workspace_id is given. It writes nothing, except a 500 when
// the Default workspace cannot be loaded (then it reports false).
func (a *API) holdsIn(c *gin.Context, workspaceID uint, p authz.Permission) bool {
	switch a.workspaceCheckMode(c) {
	case workspaceOpen:
		return true
	case workspaceDeny:
		return false
	}
	ws, ok := a.effectiveWorkspaceID(c, workspaceID)
	if !ok {
		return false
	}
	return authz.CanIn(c, ws, p)
}

// requireInWorkspace returns false, having written 403 permission_denied,
// when the caller does not hold perm in the object's Workspace.
func (a *API) requireInWorkspace(c *gin.Context, perm authz.Permission, workspaceID uint) bool {
	if a.holdsIn(c, workspaceID, perm) {
		return true
	}
	if !c.IsAborted() {
		c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(perm))
	}
	return false
}

// visibleIn reports whether the caller may see an object of resource in the
// Workspace: always for resources that are not read-scoped (D4), otherwise
// when the caller's read covers the Workspace.
func (a *API) visibleIn(c *gin.Context, resource string, workspaceID uint) bool {
	if !authz.ResourceScope(resource).ReadScoped() {
		return true
	}
	return a.holdsIn(c, workspaceID, authz.Read(resource))
}

// requireVisible returns false, having answered 404, when the object is
// hidden from the caller. notFound writes the handler's own not-found
// response, so a hidden object is indistinguishable from a missing one; nil
// writes a generic 404.
func (a *API) requireVisible(c *gin.Context, resource string, workspaceID uint, notFound func(*gin.Context)) bool {
	if a.visibleIn(c, resource, workspaceID) {
		return true
	}
	if c.IsAborted() {
		return false
	}
	if notFound != nil {
		notFound(c)
	} else {
		simpleError(c, http.StatusNotFound, "Not Found", "not found")
	}
	c.Abort()
	return false
}

// workspaceListScope restricts a list of a read-scoped resource to the
// Workspaces the caller's read covers; it is a no-op for other resources,
// for a global read, in TestMode and for legacy callers. A stored
// workspace_id of 0 counts as the Default workspace (.41). column is the
// workspace_id column to filter, qualified when the query joins ("apps.workspace_id");
// "" means "workspace_id". For App credentials pass the parent App's
// column on a query that joins apps.
func (a *API) workspaceListScope(c *gin.Context, resource, column string) func(*gorm.DB) *gorm.DB {
	if column == "" {
		column = "workspace_id"
	}
	noop := func(db *gorm.DB) *gorm.DB { return db }
	none := func(db *gorm.DB) *gorm.DB { return db.Where("1 = 0") }
	if !authz.ResourceScope(resource).ReadScoped() {
		return noop
	}
	switch a.workspaceCheckMode(c) {
	case workspaceOpen:
		return noop
	case workspaceDeny:
		return none
	}
	all, ids := authz.WorkspacesWith(c, authz.Read(resource))
	if all {
		return noop
	}
	if len(ids) == 0 {
		return none
	}
	def, err := a.service.DefaultWorkspaceID()
	if err != nil {
		return func(db *gorm.DB) *gorm.DB {
			_ = db.AddError(err)
			return db
		}
	}
	coversDefault := false
	for _, id := range ids {
		if id == def {
			coversDefault = true
			break
		}
	}
	return func(db *gorm.DB) *gorm.DB {
		if coversDefault {
			return db.Where("("+column+" IN ? OR "+column+" = 0)", ids)
		}
		return db.Where(column+" IN ?", ids)
	}
}

// workspaceFilterParam reads the optional ?workspace_id= list filter, usable
// on every stamped list (the Operator lists only its own objects). It
// returns a scope (a no-op without the parameter) and false, having answered
// 400, for a malformed value. Filtering on the Default workspace also
// matches stored 0. It narrows what workspaceListScope allows; it never
// widens it. column as for workspaceListScope.
func (a *API) workspaceFilterParam(c *gin.Context, column string) (func(*gorm.DB) *gorm.DB, bool) {
	if column == "" {
		column = "workspace_id"
	}
	raw, present := c.GetQuery("workspace_id")
	if !present || strings.TrimSpace(raw) == "" {
		return func(db *gorm.DB) *gorm.DB { return db }, true
	}
	id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 32)
	if err != nil || id == 0 {
		simpleError(c, http.StatusBadRequest, "Bad Request", "workspace_id must be a positive integer")
		c.Abort()
		return nil, false
	}
	def, err := a.service.DefaultWorkspaceID()
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		c.Abort()
		return nil, false
	}
	if uint(id) == def {
		return func(db *gorm.DB) *gorm.DB {
			return db.Where("("+column+" = ? OR "+column+" = 0)", id)
		}, true
	}
	return func(db *gorm.DB) *gorm.DB { return db.Where(column+" = ?", id) }, true
}

// invalidWorkspace answers 422 invalid_workspace. The body is the same for a
// Workspace that does not exist and one the caller's grant does not cover,
// so Workspace ids cannot be probed (.37).
func invalidWorkspace(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnprocessableEntity, authz.Body("Invalid Workspace",
		"the Workspace does not exist or you cannot create objects in it", authz.CodeInvalidWorkspace))
}

// resolveCreateWorkspace applies the landing rule (decision D5) for a new
// object of resource. requested is the workspace_id in the request, if any.
//
//   - Requested: it must exist and the caller's write must cover it, else
//     422 invalid_workspace (same body for both cases).
//   - Not requested: a global write lands in the Default workspace; a scoped
//     write in exactly one Workspace lands there; in several, 422
//     workspace_required.
//
// The handler sets the result on the object and, in the insert's
// transaction, calls services.LockWorkspaceForInsert so the object never
// lands in a Workspace a concurrent delete is removing (.39). In TestMode
// and for legacy callers the requested Workspace (else the Default) is
// returned unchecked. Community Edition always resolves to the Default.
func (a *API) resolveCreateWorkspace(c *gin.Context, resource string, requested *uint) (uint, bool) {
	perm := authz.Write(resource)
	mode := a.workspaceCheckMode(c)
	if mode == workspaceDeny {
		c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(perm))
		return 0, false
	}
	def, err := a.service.DefaultWorkspaceID()
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		c.Abort()
		return 0, false
	}
	if requested != nil {
		id := *requested
		if mode == workspaceOpen {
			if id == 0 {
				return def, true
			}
			return id, true
		}
		exists, err := a.service.WorkspaceExists(id)
		if err != nil {
			simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
			c.Abort()
			return 0, false
		}
		if !exists || !authz.CanIn(c, id, perm) {
			invalidWorkspace(c)
			return 0, false
		}
		return id, true
	}
	if mode == workspaceOpen {
		return def, true
	}
	all, ids := authz.WorkspacesWith(c, perm)
	switch {
	case all:
		return def, true
	case len(ids) == 1:
		return ids[0], true
	case len(ids) > 1:
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, authz.Body("Workspace required",
			"you can create this object in several Workspaces; name one in workspace_id", authz.CodeWorkspaceRequired))
		return 0, false
	default:
		c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(perm))
		return 0, false
	}
}

// requireMove checks an update that changes an object's Workspace from
// (stored workspace_id) to (requested): the caller's write must cover both,
// and the destination must exist (422 invalid_workspace otherwise, the same
// body whether it is missing or not covered). An unchanged Workspace passes.
// The move itself writes workspace_id and updated_at together, e.g.
// Select("workspace_id", "updated_at", ...) or UpdateColumns setting both:
// ordinary updates never write workspace_id (.42), and a move must bump
// updated_at so it shows as a pending change (moves stain).
func (a *API) requireMove(c *gin.Context, resource string, from, to uint) bool {
	perm := authz.Write(resource)
	mode := a.workspaceCheckMode(c)
	if mode == workspaceDeny {
		c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(perm))
		return false
	}
	src, ok := a.effectiveWorkspaceID(c, from)
	if !ok {
		return false
	}
	if to == 0 {
		invalidWorkspace(c)
		return false
	}
	if src == to {
		return true
	}
	if mode == workspaceOpen {
		return true
	}
	if !authz.CanIn(c, src, perm) {
		c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(perm))
		return false
	}
	exists, err := a.service.WorkspaceExists(to)
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		c.Abort()
		return false
	}
	if !exists || !authz.CanIn(c, to, perm) {
		invalidWorkspace(c)
		return false
	}
	return true
}

// requirePublishIfChangedIn is requirePublishIfChanged for an object in a
// Workspace: flipping the live switch needs publish in the object's
// Workspace, never merely somewhere (that would let publish in W1 release a
// W2 object).
func (a *API) requirePublishIfChangedIn(c *gin.Context, resource string, workspaceID uint, before, after bool) bool {
	if before == after {
		return true
	}
	return a.requireInWorkspace(c, authz.Publish(resource), workspaceID)
}

// requirePublishToCreateLiveIn is requirePublishToCreateLive for an object
// being created in a Workspace (the result of resolveCreateWorkspace).
func (a *API) requirePublishToCreateLiveIn(c *gin.Context, resource string, workspaceID uint, live bool) bool {
	if !live {
		return true
	}
	return a.requireInWorkspace(c, authz.Publish(resource), workspaceID)
}

// isWorkspaceNotFound reports whether err says the Workspace an insert
// targets is gone (services.LockWorkspaceForInsert); handlers answer it
// with invalidWorkspace.
func isWorkspaceNotFound(err error) bool {
	return errors.Is(err, services.ErrWorkspaceNotFound)
}

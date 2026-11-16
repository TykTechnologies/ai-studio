package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/models/wsstamp"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// Shared plumbing of the Workspace-checked object handlers (Workspaces B4).
// The optional workspace_id travels in data.attributes; it is read
// separately from each resource's input struct (as role_ids is), so the
// shared input structs stay untouched.

// workspaceIDProbe reads data.attributes.workspace_id.
type workspaceIDProbe struct {
	Data struct {
		Attributes struct {
			WorkspaceID *uint `json:"workspace_id"`
		} `json:"attributes"`
	} `json:"data"`
}

// requestedWorkspaceID reads data.attributes.workspace_id from a JSON body;
// nil when absent. It answers 400 and returns false for a value that is not
// a non-negative integer.
func requestedWorkspaceID(c *gin.Context, raw []byte) (*uint, bool) {
	var probe workspaceIDProbe
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &probe); err != nil {
			simpleError(c, http.StatusBadRequest, "Bad Request", "workspace_id must be a Workspace id")
			c.Abort()
			return nil, false
		}
	}
	return probe.Data.Attributes.WorkspaceID, true
}

// boundWorkspaceID is requestedWorkspaceID for a handler that bound its
// body with ShouldBindBodyWith (the body is cached on the context).
func boundWorkspaceID(c *gin.Context) (*uint, bool) {
	var probe workspaceIDProbe
	if err := c.ShouldBindBodyWith(&probe, binding.JSON); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", "workspace_id must be a Workspace id")
		c.Abort()
		return nil, false
	}
	return probe.Data.Attributes.WorkspaceID, true
}

// resolveCreateFromBody is resolveCreateWorkspace with the requested
// Workspace read from the bound body.
func (a *API) resolveCreateFromBody(c *gin.Context, resource string) (uint, bool) {
	requested, ok := boundWorkspaceID(c)
	if !ok {
		return 0, false
	}
	return a.resolveCreateWorkspace(c, resource, requested)
}

// checkObjectUpdate checks an update of an object stored in workspaceID: the
// caller's write must cover the Workspace, and a requested workspace_id that
// differs is a move (requireMove). It returns the destination of a move, or
// nil when the object stays.
func (a *API) checkObjectUpdate(c *gin.Context, resource string, stored uint, requested *uint) (*uint, bool) {
	if !a.requireInWorkspace(c, authz.Write(resource), stored) {
		return nil, false
	}
	if requested == nil {
		return nil, true
	}
	src, ok := a.effectiveWorkspaceID(c, stored)
	if !ok {
		return nil, false
	}
	if *requested == src {
		return nil, true
	}
	if !a.requireMove(c, resource, stored, *requested) {
		return nil, false
	}
	return requested, true
}

// errMoveRaced reports that the object was no longer in the Workspace the
// move was checked against: a concurrent request moved it first.
var errMoveRaced = errors.New("the object was moved by another request")

// moveObject moves the row of model with the id from the Workspace from (its
// raw stored workspace_id as loaded, 0 included, never the Default-mapped
// value: the guard compares the column itself; the caller's permissions
// were checked in that Workspace) to
// the Workspace to: workspace_id and updated_at together, so the move shows
// as a pending change (moves stain), with the destination locked for the
// write so it cannot land in a Workspace a concurrent delete is removing
// (422 invalid_workspace then). The UPDATE only matches while the row is
// still in from; when a concurrent move got there first nothing changes
// and the answer is 409 Conflict. It writes the error response and returns
// false on failure.
func (a *API) moveObject(c *gin.Context, model interface{}, id uint, from, to uint) bool {
	err := a.service.DB.Transaction(func(tx *gorm.DB) error {
		if err := models.LockWorkspaceForInsert(tx, to); err != nil {
			return err
		}
		res := tx.Model(model).Where("id = ? AND workspace_id = ?", id, from).Select("workspace_id", "updated_at").
			Updates(map[string]interface{}{"workspace_id": to, "updated_at": time.Now()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errMoveRaced
		}
		return nil
	})
	switch {
	case err == nil:
		return true
	case errors.Is(err, errMoveRaced):
		simpleError(c, http.StatusConflict, "Conflict", "The object was moved to another Workspace by a concurrent request; reload it and try again.")
		c.Abort()
		return false
	case isWorkspaceNotFound(err) || err == wsstamp.ErrWorkspaceNotFound:
		invalidWorkspace(c)
		return false
	}
	simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
	c.Abort()
	return false
}

// listWorkspaceScopes are the list scopes of a Workspace resource: the
// read scope (workspaceListScope: a no-op for read-global resources,
// the caller's Workspaces for read-scoped ones) and the optional
// ?workspace_id= filter. column is the workspace_id column, qualified when
// the query joins other stamped tables ("data_catalogues.workspace_id");
// "" means "workspace_id". It answers 400 and returns false for a
// malformed value.
func (a *API) listWorkspaceScopes(c *gin.Context, resource, column string) ([]func(*gorm.DB) *gorm.DB, bool) {
	filter, ok := a.workspaceFilterParam(c, column)
	if !ok {
		return nil, false
	}
	return []func(*gorm.DB) *gorm.DB{a.workspaceListScope(c, resource, column), filter}, true
}

// storedWorkspaceID reads the workspace_id of the live row of model with id;
// found is false when there is none.
func (a *API) storedWorkspaceID(model interface{}, id uint) (ws uint, found bool, err error) {
	var ids []uint
	if err := a.service.DB.Model(model).Where("id = ?", id).Limit(1).Pluck("workspace_id", &ids).Error; err != nil {
		return 0, false, err
	}
	if len(ids) == 0 {
		return 0, false, nil
	}
	return ids[0], true, nil
}

// requireOnObject checks perm in the Workspace of the object (model, id)
// before an action on it (403 permission_denied). For a read-scoped
// resource (workspace_full, workspace_parent: perm's resource) an object
// the caller may not see answers 404 first, as requireVisible does, so a
// 403 never confirms it exists. A missing object passes, so the handler
// answers its own 404. notFound, when given, is the handler's not-found
// writer: a hidden object and a missing one then both get it, byte for byte
// (no existence oracle).
func (a *API) requireOnObject(c *gin.Context, model interface{}, id uint, perm authz.Permission, notFound ...func(*gin.Context)) bool {
	if a.workspaceCheckMode(c) == workspaceOpen {
		return true
	}
	ws, found, err := a.storedWorkspaceID(model, id)
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		c.Abort()
		return false
	}
	if !found {
		if len(notFound) > 0 {
			// Missing answers exactly as hidden does.
			notFound[0](c)
			c.Abort()
			return false
		}
		return true
	}
	return a.requireOnWorkspace(c, ws, perm, notFound...)
}

// requireOnWorkspace is requireOnObject once the object's stored Workspace
// (ws) is known: 404 (notFound) first for a read-scoped resource the caller
// may not see there, then perm in ws.
func (a *API) requireOnWorkspace(c *gin.Context, ws uint, perm authz.Permission, notFound ...func(*gin.Context)) bool {
	var nf func(*gin.Context)
	if len(notFound) > 0 {
		nf = notFound[0]
	}
	if resource := perm.Resource(); authz.ResourceScope(resource).ReadScoped() && !a.requireVisible(c, resource, ws, nf) {
		return false
	}
	return a.requireInWorkspace(c, perm, ws)
}

// notFoundBody is a not-found writer with the handler's own body, for
// onObject and requireOnObject.
func notFoundBody(detail string) func(*gin.Context) {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: detail}},
		})
	}
}

// responseWorkspaceID is the workspace_id an object response carries: the
// Workspace it means, a stored 0 read as the Default (when the Default
// cannot be loaded, the stored value).
func (a *API) responseWorkspaceID(stored uint) uint {
	if stored != 0 || a.service == nil {
		return stored
	}
	if def, err := a.service.DefaultWorkspaceID(); err == nil {
		return def
	}
	return stored
}

// onObject is route middleware for an action on the object named by the
// :id path parameter: perm must cover the object's Workspace (403
// permission_denied; 404 first for a read-scoped object the caller may not
// see, requireOnObject). A malformed or unknown id passes, for the handler to
// answer as it always has.
func (a *API) onObject(model interface{}, perm authz.Permission, notFound ...func(*gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err == nil && !a.requireOnObject(c, model, uint(id), perm, notFound...) {
			return
		}
		c.Next()
	}
}

// responseWorkspaceIDFrom is responseWorkspaceID for serializers that have
// only a database handle.
func responseWorkspaceIDFrom(db *gorm.DB, stored uint) uint {
	if stored != 0 || db == nil {
		return stored
	}
	if def, err := models.GetDefaultWorkspace(db); err == nil {
		return def.ID
	}
	return stored
}

// beginMove moves the object (model, id) from its stored Workspace to *to
// before the handler applies the rest of the update, so the update's checks
// (secret references were checked against the destination) and the stored
// Workspace agree. finish(true) keeps the move; finish(false), when the
// update then fails, puts the object back where it was, but only while it
// is still in *to (a later concurrent move is left alone). With to nil it
// does nothing. It writes the error response and returns false when the
// move itself fails (409 when a concurrent move got there first).
func (a *API) beginMove(c *gin.Context, model interface{}, id uint, from uint, to *uint) (finish func(updated bool), ok bool) {
	if to == nil {
		return func(bool) {}, true
	}
	if !a.moveObject(c, model, id, from, *to) {
		return nil, false
	}
	return func(updated bool) {
		if updated {
			return
		}
		if err := a.service.DB.Model(model).Where("id = ? AND workspace_id = ?", id, *to).Select("workspace_id", "updated_at").
			Updates(map[string]interface{}{"workspace_id": from, "updated_at": time.Now()}).Error; err != nil {
			logger.Errorf("Workspace move of %T %d could not be undone after a failed update: %v", model, id, err)
		}
	}, true
}

// App credentials (credentials, mcp-credentials) have no Workspace of their
// own: they belong to their App's (ScopeWorkspaceParent, decision of .9).
// An action on one is checked in the App's Workspace, and a credential whose
// App the caller cannot see answers the handler's own 404. A credential
// with no live App (created standalone, or its App deleted) is reachable
// only with the permission held globally; to anyone else it is not found.

// errMalformedID: the path id is not one; the handler answers it.
var errMalformedID = errors.New("malformed id")

// appWorkspaceOfCredential reads the Workspace of the live App holding the
// API credential id (apps.credential_id). credFound is false when there is
// no such credential; appFound when no live App holds it.
func (a *API) appWorkspaceOfCredential(rawID string) (ws uint, credFound, appFound bool, err error) {
	id, perr := strconv.ParseUint(rawID, 10, 32)
	if perr != nil {
		return 0, false, false, errMalformedID
	}
	// One query: the credential, and the workspace_id of each live App
	// holding it (NULL when none).
	var rows []struct{ WorkspaceID *uint }
	if err := a.service.DB.Model(&models.Credential{}).
		Select("apps.workspace_id").
		Joins("LEFT JOIN apps ON apps.credential_id = credentials.id AND apps.deleted_at IS NULL").
		Where("credentials.id = ?", id).Limit(2).Scan(&rows).Error; err != nil {
		return 0, false, false, err
	}
	switch {
	case len(rows) == 0:
		return 0, false, false, nil
	case len(rows) == 1 && rows[0].WorkspaceID != nil:
		return *rows[0].WorkspaceID, true, true, nil
	}
	// No App, or (only through hand-written SQL) more than one: no single
	// owning Workspace, so it is treated as standalone (global holders
	// only), failing closed rather than picking one.
	return 0, true, false, nil
}

// appWorkspaceOfMCPCredential is appWorkspaceOfCredential for an MCP
// credential (mcp_credentials.app_id).
func (a *API) appWorkspaceOfMCPCredential(id string) (ws uint, credFound, appFound bool, err error) {
	// One query: the credential, and its live App's workspace_id (NULL when
	// the App is gone).
	var rows []struct{ WorkspaceID *uint }
	if err := a.service.DB.Model(&models.MCPCredential{}).
		Select("apps.workspace_id").
		Joins("LEFT JOIN apps ON apps.id = mcp_credentials.app_id AND apps.deleted_at IS NULL").
		Where("mcp_credentials.id = ?", id).Limit(1).Scan(&rows).Error; err != nil || len(rows) == 0 {
		return 0, false, false, err
	}
	if rows[0].WorkspaceID == nil {
		return 0, true, false, nil
	}
	return *rows[0].WorkspaceID, true, true, nil
}

// requireOnAppCredential decides an action (perm) on a credential from what
// lookup found (appWorkspaceOfCredential or appWorkspaceOfMCPCredential).
// A missing credential gets notFound, as a hidden one does.
func (a *API) requireOnAppCredential(c *gin.Context, id string, perm authz.Permission, lookup func(string) (uint, bool, bool, error), notFound func(*gin.Context)) bool {
	if a.workspaceCheckMode(c) == workspaceOpen {
		return true
	}
	ws, credFound, appFound, err := lookup(id)
	if errors.Is(err, errMalformedID) {
		return true // the handler answers its 400
	}
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		c.Abort()
		return false
	}
	if !credFound {
		// Missing answers exactly as hidden does.
		notFound(c)
		c.Abort()
		return false
	}
	if !appFound {
		if authz.Can(c, perm) {
			return true
		}
		notFound(c)
		c.Abort()
		return false
	}
	return a.requireOnWorkspace(c, ws, perm, notFound)
}

// onAppCredential is route middleware for an action on the credential named
// by the :id path parameter (requireOnAppCredential).
func (a *API) onAppCredential(perm authz.Permission, lookup func(string) (uint, bool, bool, error), notFound func(*gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.requireOnAppCredential(c, c.Param("id"), perm, lookup, notFound) {
			return
		}
		c.Next()
	}
}

// parentAppScopes are the conditions on the apps table that limit a list
// of an App-parented resource (credentials, mcp-credentials) to the Apps
// the caller may see there (workspaceListScope on apps.workspace_id), plus
// the optional ?workspace_id= filter on the App's Workspace. nil when
// nothing is limited: a global reader, TestMode and legacy callers see
// every row, standalone ones included, unless they filter by Workspace.
func (a *API) parentAppScopes(c *gin.Context, resource string) ([]func(*gorm.DB) *gorm.DB, bool) {
	filter, ok := a.workspaceFilterParam(c, "apps.workspace_id")
	if !ok {
		return nil, false
	}
	_, filtered := c.GetQuery("workspace_id")
	if !filtered && (a.workspaceCheckMode(c) != workspaceCheck || authz.Can(c, authz.Read(resource))) {
		return nil, true
	}
	return []func(*gorm.DB) *gorm.DB{a.workspaceListScope(c, resource, "apps.workspace_id"), filter}, true
}

// credentialListScopes narrow a list of API credentials to those of a live
// App the caller may see (parentAppScopes).
func (a *API) credentialListScopes(c *gin.Context) ([]func(*gorm.DB) *gorm.DB, bool) {
	appScopes, ok := a.parentAppScopes(c, "credentials")
	if !ok || appScopes == nil {
		return nil, ok
	}
	return []func(*gorm.DB) *gorm.DB{func(db *gorm.DB) *gorm.DB {
		// Two uncorrelated subqueries on the indexed apps.credential_id: the
		// credentials of visible Apps, and those exactly one live App holds
		// (see appWorkspaceOfCredential).
		fresh := db.Session(&gorm.Session{NewDB: true})
		visible := fresh.Model(&models.App{}).Select("apps.credential_id").Scopes(appScopes...)
		single := fresh.Model(&models.App{}).Select("credential_id").Group("credential_id").Having("COUNT(*) = 1")
		return db.Where("credentials.id IN (?)", visible).Where("credentials.id IN (?)", single)
	}}, true
}

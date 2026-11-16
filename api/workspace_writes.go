package api

import (
	"encoding/json"
	"net/http"
	"time"

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

// moveObject moves the row of model with the id to the Workspace: workspace_id
// and updated_at together, so the move shows as a pending change (moves
// stain), with the destination locked for the write so it cannot land in a
// Workspace a concurrent delete is removing (422 invalid_workspace then).
// It writes the error response and returns false on failure.
func (a *API) moveObject(c *gin.Context, model interface{}, id uint, to uint) bool {
	err := a.service.DB.Transaction(func(tx *gorm.DB) error {
		if err := models.LockWorkspaceForInsert(tx, to); err != nil {
			return err
		}
		return tx.Model(model).Where("id = ?", id).Select("workspace_id", "updated_at").
			Updates(map[string]interface{}{"workspace_id": to, "updated_at": time.Now()}).Error
	})
	if err == nil {
		return true
	}
	if isWorkspaceNotFound(err) || err == wsstamp.ErrWorkspaceNotFound {
		invalidWorkspace(c)
		return false
	}
	simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
	c.Abort()
	return false
}

// listWorkspaceScopes are the list scopes of a read-global (workspace_write)
// resource: only the optional ?workspace_id= filter. It answers 400 and
// returns false for a malformed value.
func (a *API) listWorkspaceScopes(c *gin.Context, resource string) ([]func(*gorm.DB) *gorm.DB, bool) {
	filter, ok := a.workspaceFilterParam(c, "")
	if !ok {
		return nil, false
	}
	return []func(*gorm.DB) *gorm.DB{a.workspaceListScope(c, resource, ""), filter}, true
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
// before an action on it (403 permission_denied). A missing object passes,
// so the handler answers its own 404.
func (a *API) requireOnObject(c *gin.Context, model interface{}, id uint, perm authz.Permission) bool {
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
		return true
	}
	return a.requireInWorkspace(c, perm, ws)
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

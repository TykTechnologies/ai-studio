package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/rbac"
	"github.com/TykTechnologies/midsommar/v2/services/workspaces"
	"github.com/gin-gonic/gin"
)

// Workspaces API (Workspaces B5). The handlers are core: in Community
// Edition, or Enterprise without role-based access control licensed, the
// service answers ErrEnterpriseFeature and every route answers 402 with
// code enterprise_required.

// WorkspaceInput is the body of POST and PATCH /workspaces. is_default is
// not accepted: the Default workspace is fixed.
// @Description Workspace create or update payload
type WorkspaceInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// WorkspaceAttributes is the Workspace payload.
// @Description Workspace attributes
type WorkspaceAttributes struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WorkspaceResponse wraps a Workspace.
// @Description Workspace response model
type WorkspaceResponse struct {
	Type       string              `json:"type"`
	ID         string              `json:"id"`
	Attributes WorkspaceAttributes `json:"attributes"`
}

func serializeWorkspace(w *models.Workspace) WorkspaceResponse {
	return WorkspaceResponse{
		Type: "workspace",
		ID:   strconv.FormatUint(uint64(w.ID), 10),
		Attributes: WorkspaceAttributes{
			Name:        w.Name,
			Description: w.Description,
			IsDefault:   w.IsDefault,
			CreatedAt:   w.CreatedAt,
			UpdatedAt:   w.UpdatedAt,
		},
	}
}

func workspaceErrorResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, workspaces.ErrEnterpriseFeature):
		c.JSON(http.StatusPaymentRequired, authz.EnterpriseRequired(err.Error()))
	case errors.Is(err, workspaces.ErrNotFound):
		c.JSON(http.StatusNotFound, authz.Body("Not Found", err.Error(), ""))
	case errors.Is(err, workspaces.ErrInvalidName):
		c.JSON(http.StatusBadRequest, authz.Body("Bad Request", err.Error(), ""))
	case errors.Is(err, workspaces.ErrDuplicateName):
		c.JSON(http.StatusConflict, authz.Body("Conflict", err.Error(), ""))
	case errors.Is(err, workspaces.ErrDefaultLocked):
		c.JSON(http.StatusConflict, authz.Body("Default Workspace", err.Error(), authz.CodeDefaultWorkspaceLocked))
	case errors.Is(err, workspaces.ErrInUse):
		c.JSON(http.StatusConflict, authz.Body("Workspace In Use", err.Error(), authz.CodeWorkspaceInUse))
	default:
		c.JSON(http.StatusInternalServerError, authz.Body("Internal Server Error", err.Error(), ""))
	}
}

func workspaceIDParam(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		c.JSON(http.StatusNotFound, authz.Body("Not Found", workspaces.ErrNotFound.Error(), ""))
		return 0, false
	}
	return uint(id), true
}

// coversEveryWorkspace reports whether the caller's grant reaches every
// Workspace: workspaces:read globally, or any permission on a Workspace
// resource held globally (a global binding covers every Workspace, D1).
func coversEveryWorkspace(g authz.Grants) bool {
	if g.Global.Has(authz.Read("workspaces")) || g.Global.IsFullAdmin() {
		return true
	}
	for _, key := range authz.WorkspaceResources() {
		if g.Global.Has(authz.Read(key)) {
			return true
		}
	}
	return false
}

// @Summary List Workspaces
// @Description Lists the Workspaces the caller can work in: every Workspace for a caller whose access covers them all (workspaces:read, or any global permission on Workspace objects), otherwise those the caller holds a Workspace-scoped role in. The Default workspace comes first. Enterprise Edition.
// @Tags workspaces
// @Produce json
// @Success 200 {object} map[string][]WorkspaceResponse
// @Failure 402 {object} authz.ErrorBody
// @Router /workspaces [get]
// @Security BearerAuth
func (a *API) listWorkspaces(c *gin.Context) {
	all, err := a.service.WorkspacesService().List(c.Request.Context())
	if err != nil {
		workspaceErrorResponse(c, err)
		return
	}
	grants, err := requestGrants(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, authz.Body("Internal Server Error", err.Error(), ""))
		return
	}
	everyWorkspace := a.config.TestMode || coversEveryWorkspace(grants)
	out := make([]WorkspaceResponse, 0, len(all))
	for i := range all {
		if everyWorkspace {
			out = append(out, serializeWorkspace(&all[i]))
			continue
		}
		if set, ok := grants.Workspaces[all[i].ID]; ok && !set.IsEmpty() {
			out = append(out, serializeWorkspace(&all[i]))
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// @Summary Create a Workspace
// @Description Creates a Workspace. Names are unique without regard to case. Enterprise Edition.
// @Tags workspaces
// @Accept json
// @Produce json
// @Param workspace body WorkspaceInput true "Workspace"
// @Success 201 {object} map[string]WorkspaceResponse
// @Failure 400 {object} authz.ErrorBody
// @Failure 402 {object} authz.ErrorBody
// @Failure 409 {object} authz.ErrorBody
// @Router /workspaces [post]
// @Security BearerAuth
func (a *API) createWorkspace(c *gin.Context) {
	var in WorkspaceInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, authz.Body("Bad Request", err.Error(), ""))
		return
	}
	actor, _ := auth.UserFromContext(c)
	w, err := a.service.WorkspacesService().Create(c.Request.Context(), actor, workspaces.Input{Name: in.Name, Description: in.Description})
	if err != nil {
		workspaceErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": serializeWorkspace(w)})
}

// @Summary Get a Workspace
// @Tags workspaces
// @Produce json
// @Param id path int true "Workspace ID"
// @Success 200 {object} map[string]WorkspaceResponse
// @Failure 402 {object} authz.ErrorBody
// @Failure 404 {object} authz.ErrorBody
// @Router /workspaces/{id} [get]
// @Security BearerAuth
func (a *API) getWorkspace(c *gin.Context) {
	id, ok := workspaceIDParam(c)
	if !ok {
		return
	}
	w, err := a.service.WorkspacesService().Get(c.Request.Context(), id)
	if err != nil {
		workspaceErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeWorkspace(w)})
}

// @Summary Update a Workspace
// @Description Renames or re-describes a Workspace. is_default cannot be changed. Enterprise Edition.
// @Tags workspaces
// @Accept json
// @Produce json
// @Param id path int true "Workspace ID"
// @Param workspace body WorkspaceInput true "Fields to change"
// @Success 200 {object} map[string]WorkspaceResponse
// @Failure 400 {object} authz.ErrorBody
// @Failure 402 {object} authz.ErrorBody
// @Failure 404 {object} authz.ErrorBody
// @Failure 409 {object} authz.ErrorBody
// @Router /workspaces/{id} [patch]
// @Security BearerAuth
func (a *API) updateWorkspace(c *gin.Context) {
	id, ok := workspaceIDParam(c)
	if !ok {
		return
	}
	var in WorkspaceInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, authz.Body("Bad Request", err.Error(), ""))
		return
	}
	actor, _ := auth.UserFromContext(c)
	w, err := a.service.WorkspacesService().Update(c.Request.Context(), actor, id, workspaces.Input{Name: in.Name, Description: in.Description})
	if err != nil {
		workspaceErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeWorkspace(w)})
}

// @Summary Delete a Workspace
// @Description Deletes a Workspace and moves every object it owns to the Default workspace. Refused (409 default_workspace_locked) for the Default, and (409 workspace_in_use) while a role is assigned within it. Enterprise Edition.
// @Tags workspaces
// @Param id path int true "Workspace ID"
// @Success 204
// @Failure 402 {object} authz.ErrorBody
// @Failure 404 {object} authz.ErrorBody
// @Failure 409 {object} authz.ErrorBody
// @Router /workspaces/{id} [delete]
// @Security BearerAuth
func (a *API) deleteWorkspace(c *gin.Context) {
	id, ok := workspaceIDParam(c)
	if !ok {
		return
	}
	actor, _ := auth.UserFromContext(c)
	if err := a.service.WorkspacesService().Delete(c.Request.Context(), actor, id); err != nil {
		workspaceErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// WorkspaceObjectsResponse counts the objects a Workspace owns per resource.
// @Description Object counts of a Workspace, keyed by permission resource
type WorkspaceObjectsResponse struct {
	WorkspaceID uint             `json:"workspace_id"`
	Counts      map[string]int64 `json:"counts"`
}

// @Summary Count a Workspace's objects
// @Description Counts the objects a Workspace owns, per resource (llms, apps, ...), for the Workspace page. Enterprise Edition.
// @Tags workspaces
// @Produce json
// @Param id path int true "Workspace ID"
// @Success 200 {object} map[string]WorkspaceObjectsResponse
// @Failure 402 {object} authz.ErrorBody
// @Failure 404 {object} authz.ErrorBody
// @Router /workspaces/{id}/objects [get]
// @Security BearerAuth
func (a *API) getWorkspaceObjects(c *gin.Context) {
	id, ok := workspaceIDParam(c)
	if !ok {
		return
	}
	counts, err := a.service.WorkspacesService().Counts(c.Request.Context(), id)
	if err != nil {
		workspaceErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": WorkspaceObjectsResponse{WorkspaceID: id, Counts: counts}})
}

// WorkspaceAccess is the caller's (or a user's) access inside one
// Workspace, from Workspace-scoped role bindings. Global access is reported
// separately and covers every Workspace.
// @Description Access inside one Workspace
type WorkspaceAccess struct {
	ID          uint                               `json:"id"`
	Name        string                             `json:"name"`
	Permissions []string                           `json:"permissions"`
	Roles       []rbac.RoleSummary                 `json:"roles,omitempty"`
	Sources     map[string][]rbac.PermissionSource `json:"sources,omitempty"`
}

// workspaceNames loads the names of the given Workspaces in one query.
func (a *API) workspaceNames(ctx context.Context, ids []uint) map[uint]string {
	names := map[uint]string{}
	if len(ids) == 0 {
		return names
	}
	var rows []models.Workspace
	if err := a.service.DB.WithContext(ctx).Select("id", "name").Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return names
	}
	for _, w := range rows {
		names[w.ID] = w.Name
	}
	return names
}

// workspaceAccessOf lists the caller's Workspace-scoped permissions, per
// Workspace with its name, ordered by id.
func (a *API) workspaceAccessOf(ctx context.Context, g authz.Grants) []WorkspaceAccess {
	ids := make([]uint, 0, len(g.Workspaces))
	for id, set := range g.Workspaces {
		if !set.IsEmpty() {
			ids = append(ids, id)
		}
	}
	names := a.workspaceNames(ctx, ids)
	out := make([]WorkspaceAccess, 0, len(ids))
	for _, id := range ids {
		out = append(out, WorkspaceAccess{ID: id, Name: names[id], Permissions: g.Workspaces[id].List()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

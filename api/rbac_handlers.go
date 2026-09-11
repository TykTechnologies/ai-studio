package api

import (
	"fmt"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/rbac"
	"github.com/gin-gonic/gin"
)

// RBAC handlers are edition-neutral: they delegate to services/rbac and map
// ErrEnterpriseFeature onto 402 so the UI can tell "not licensed" apart from
// "not permitted".

// rbacErrorResponse maps service errors onto the authz error envelope.
func rbacErrorResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, rbac.ErrEnterpriseFeature):
		c.JSON(http.StatusPaymentRequired, authz.EnterpriseRequired(err.Error()))
	case errors.Is(err, rbac.ErrNotFound):
		c.JSON(http.StatusNotFound, authz.Body("Not Found", err.Error(), ""))
	case errors.Is(err, rbac.ErrSystemRole):
		c.JSON(http.StatusConflict, authz.Body("System Role", err.Error(), authz.CodeSystemRole))
	case errors.Is(err, rbac.ErrLastOwner):
		c.JSON(http.StatusConflict, authz.Body("Last Owner", err.Error(), authz.CodeLastOwner))
	case errors.Is(err, rbac.ErrOwnerUserOnly):
		c.JSON(http.StatusConflict, authz.Body("Owner Is User-Only", err.Error(), authz.CodeOwnerUserOnly))
	case errors.Is(err, rbac.ErrOwnerRequired):
		c.JSON(http.StatusForbidden, authz.Body("Owner Required", err.Error(), authz.CodeOwnerRequired))
	case errors.Is(err, rbac.ErrDuplicateRole), errors.Is(err, rbac.ErrDuplicateBinding):
		c.JSON(http.StatusConflict, authz.Body("Conflict", err.Error(), ""))
	case errors.Is(err, rbac.ErrInvalidPermission), errors.Is(err, rbac.ErrInvalidSubject):
		c.JSON(http.StatusBadRequest, authz.Body("Bad Request", err.Error(), ""))
	default:
		c.JSON(http.StatusInternalServerError, authz.Body("Internal Server Error", err.Error(), ""))
	}
}

func rbacBadRequest(c *gin.Context, detail string) {
	c.JSON(http.StatusBadRequest, authz.Body("Bad Request", detail, ""))
}

// --- response shapes ---------------------------------------------------------

// RoleAttributes is the role payload.
// @Description Role attributes
type RoleAttributes struct {
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	IsSystem    bool      `json:"is_system"`
	Permissions []string  `json:"permissions"`
	// OrphanedPermissions are held permissions whose resource is not in the
	// catalogue right now: grants on a plugin that is uninstalled or not
	// loaded. They stay on the role and take effect again when the plugin
	// returns; the role editor lists them separately.
	OrphanedPermissions []string  `json:"orphaned_permissions"`
	UsersCount          int64     `json:"users_count"`
	GroupsCount         int64     `json:"groups_count"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// RoleResponse wraps a role in the JSON:API-style envelope used across the API.
// @Description Role response model
type RoleResponse struct {
	Type       string         `json:"type"`
	ID         string         `json:"id"`
	Attributes RoleAttributes `json:"attributes"`
}

// RoleBindingAttributes is the binding payload.
// @Description Role binding attributes
type RoleBindingAttributes struct {
	SubjectType string            `json:"subject_type"`
	SubjectID   uint              `json:"subject_id"`
	RoleID      uint              `json:"role_id"`
	Role        *rbac.RoleSummary `json:"role,omitempty"`
	ScopeType   string            `json:"scope_type"`
	ScopeID     string            `json:"scope_id"`
	CreatedAt   time.Time         `json:"created_at"`
}

// RoleBindingResponse wraps a binding.
// @Description Role binding response model
type RoleBindingResponse struct {
	Type       string                `json:"type"`
	ID         string                `json:"id"`
	Attributes RoleBindingAttributes `json:"attributes"`
}

// EffectiveAccessResponse is a user's resolved access.
// @Description Effective permissions for a user
type EffectiveAccessResponse struct {
	UserID         uint               `json:"user_id"`
	Permissions    []string           `json:"permissions"`
	Roles          []rbac.RoleSummary `json:"roles"`
	IsFullAdmin    bool               `json:"is_full_admin"`
	HasAdminAccess bool               `json:"has_admin_access"`
	Enabled        bool               `json:"enabled"`
}

func serializeRole(r *models.Role, counts rbac.RoleCounts) RoleResponse {
	perms := []string(r.Permissions)
	if perms == nil {
		perms = []string{}
	}
	orphaned := []string{}
	for _, p := range perms {
		if !authz.Permission(p).Valid() {
			orphaned = append(orphaned, p)
		}
	}
	return RoleResponse{
		Type: "role",
		ID:   strconv.FormatUint(uint64(r.ID), 10),
		Attributes: RoleAttributes{
			Name:                r.Name,
			Slug:                r.Slug,
			Description:         r.Description,
			IsSystem:            r.IsSystem,
			Permissions:         perms,
			OrphanedPermissions: orphaned,
			UsersCount:          counts.Users,
			GroupsCount:         counts.Groups,
			CreatedAt:           r.CreatedAt,
			UpdatedAt:           r.UpdatedAt,
		},
	}
}

func roleSummaryOf(r *models.Role) *rbac.RoleSummary {
	if r == nil {
		return nil
	}
	return &rbac.RoleSummary{ID: r.ID, Name: r.Name, Slug: r.Slug, IsSystem: r.IsSystem}
}

func serializeBinding(b *models.RoleBinding) RoleBindingResponse {
	return RoleBindingResponse{
		Type: "role_binding",
		ID:   strconv.FormatUint(uint64(b.ID), 10),
		Attributes: RoleBindingAttributes{
			SubjectType: b.SubjectType,
			SubjectID:   b.SubjectID,
			RoleID:      b.RoleID,
			Role:        roleSummaryOf(b.Role),
			ScopeType:   b.ScopeType,
			ScopeID:     b.ScopeID,
			CreatedAt:   b.CreatedAt,
		},
	}
}

func effectiveResponse(userID uint, eff *rbac.Effective, enabled bool) EffectiveAccessResponse {
	roles := eff.Roles
	if roles == nil {
		roles = []rbac.RoleSummary{}
	}
	return EffectiveAccessResponse{
		UserID:         userID,
		Permissions:    eff.Permissions.List(),
		Roles:          roles,
		IsFullAdmin:    eff.Permissions.IsFullAdmin(),
		HasAdminAccess: !eff.Permissions.IsEmpty(),
		Enabled:        enabled,
	}
}

func parseUintParam(c *gin.Context, name string) (uint, bool) {
	v, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || v == 0 {
		rbacBadRequest(c, "invalid "+name)
		return 0, false
	}
	return uint(v), true
}

// --- catalogue and self ------------------------------------------------------

// PermissionCatalogueResponse is the catalogue the role editor renders.
// @Description Permission catalogue
type PermissionCatalogueResponse struct {
	Enabled   bool             `json:"enabled"`
	Groups    []string         `json:"groups"`
	Resources []authz.Resource `json:"resources"`
	Actions   []authz.Action   `json:"actions"`
	// Version changes whenever a plugin registers or removes permission
	// resources, so clients can tell a cached catalogue is stale.
	Version uint64 `json:"version"`
}

// @Summary Get the permission catalogue
// @Description Lists every resource and action roles can grant. Available in both editions; "enabled" reports whether roles are active.
// @Tags rbac
// @Produce json
// @Success 200 {object} PermissionCatalogueResponse
// @Router /rbac/permissions [get]
// @Security BearerAuth
func (a *API) getPermissionCatalogue(c *gin.Context) {
	version := authz.Version()
	c.Header("ETag", fmt.Sprintf(`"catalogue-%d"`, version))
	c.JSON(http.StatusOK, PermissionCatalogueResponse{
		Enabled:   a.service.Authz().Enabled(),
		Groups:    authz.Groups,
		Resources: authz.Catalogue(),
		Actions:   authz.Actions,
		Version:   version,
	})
}

// @Summary Get my effective access
// @Description Returns the caller's effective permissions and the roles that grant them.
// @Tags rbac
// @Produce json
// @Success 200 {object} EffectiveAccessResponse
// @Router /rbac/me [get]
// @Security BearerAuth
func (a *API) getMyAccess(c *gin.Context) {
	user, ok := auth.UserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, authz.Unauthenticated())
		return
	}
	eff, err := a.service.Authz().EffectivePermissions(c.Request.Context(), user.ID)
	if err != nil {
		rbacErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, effectiveResponse(user.ID, eff, a.service.Authz().Enabled()))
}

// @Summary Get a user's effective access
// @Tags rbac
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} EffectiveAccessResponse
// @Router /rbac/users/{id}/effective [get]
// @Security BearerAuth
func (a *API) getUserEffectiveAccess(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	eff, err := a.service.Authz().EffectivePermissions(c.Request.Context(), id)
	if err != nil {
		rbacErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, effectiveResponse(id, eff, a.service.Authz().Enabled()))
}

// --- roles ---------------------------------------------------------------------

// RoleInput is the create/update payload for a role.
// @Description Role input model
type RoleInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// @Summary List roles
// @Tags rbac
// @Produce json
// @Success 200 {object} map[string][]RoleResponse
// @Router /rbac/roles [get]
// @Security BearerAuth
func (a *API) listRoles(c *gin.Context) {
	roles, counts, err := a.service.Authz().ListRoles(c.Request.Context())
	if err != nil {
		rbacErrorResponse(c, err)
		return
	}
	out := make([]RoleResponse, 0, len(roles))
	for i := range roles {
		out = append(out, serializeRole(&roles[i], counts[roles[i].ID]))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// @Summary Get a role
// @Tags rbac
// @Produce json
// @Param id path int true "Role ID"
// @Success 200 {object} map[string]RoleResponse
// @Router /rbac/roles/{id} [get]
// @Security BearerAuth
func (a *API) getRole(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	role, err := a.service.Authz().GetRole(c.Request.Context(), id)
	if err != nil {
		rbacErrorResponse(c, err)
		return
	}
	_, counts, err := a.service.Authz().ListRoles(c.Request.Context())
	if err != nil {
		rbacErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeRole(role, counts[role.ID])})
}

// @Summary Create a custom role
// @Tags rbac
// @Accept json
// @Produce json
// @Param role body RoleInput true "Role"
// @Success 201 {object} map[string]RoleResponse
// @Router /rbac/roles [post]
// @Security BearerAuth
func (a *API) createRole(c *gin.Context) {
	var in RoleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		rbacBadRequest(c, err.Error())
		return
	}
	actor, _ := auth.UserFromContext(c)
	role, err := a.service.Authz().CreateRole(c.Request.Context(), actor, rbac.RoleInput(in))
	if err != nil {
		rbacErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": serializeRole(role, rbac.RoleCounts{})})
}

// @Summary Update a custom role
// @Tags rbac
// @Accept json
// @Produce json
// @Param id path int true "Role ID"
// @Param role body RoleInput true "Role"
// @Success 200 {object} map[string]RoleResponse
// @Router /rbac/roles/{id} [patch]
// @Security BearerAuth
func (a *API) updateRole(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var in RoleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		rbacBadRequest(c, err.Error())
		return
	}
	actor, _ := auth.UserFromContext(c)
	role, err := a.service.Authz().UpdateRole(c.Request.Context(), actor, id, rbac.RoleInput(in))
	if err != nil {
		rbacErrorResponse(c, err)
		return
	}
	_, counts, _ := a.service.Authz().ListRoles(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"data": serializeRole(role, counts[role.ID])})
}

// @Summary Delete a custom role
// @Description Deletes the role and every binding that references it.
// @Tags rbac
// @Param id path int true "Role ID"
// @Success 204
// @Router /rbac/roles/{id} [delete]
// @Security BearerAuth
func (a *API) deleteRole(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	actor, _ := auth.UserFromContext(c)
	if err := a.service.Authz().DeleteRole(c.Request.Context(), actor, id); err != nil {
		rbacErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// CloneRoleInput names the copy.
// @Description Clone role input
type CloneRoleInput struct {
	Name string `json:"name"`
}

// @Summary Clone a role
// @Description Creates a custom role with the same permissions. This is how system roles are customised.
// @Tags rbac
// @Accept json
// @Produce json
// @Param id path int true "Role ID"
// @Param body body CloneRoleInput true "New name"
// @Success 201 {object} map[string]RoleResponse
// @Router /rbac/roles/{id}/clone [post]
// @Security BearerAuth
func (a *API) cloneRole(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var in CloneRoleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		rbacBadRequest(c, err.Error())
		return
	}
	actor, _ := auth.UserFromContext(c)
	role, err := a.service.Authz().CloneRole(c.Request.Context(), actor, id, in.Name)
	if err != nil {
		rbacErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": serializeRole(role, rbac.RoleCounts{})})
}

// --- bindings ------------------------------------------------------------------

// RoleBindingInput assigns a role to a user or team.
// @Description Role binding input model
type RoleBindingInput struct {
	SubjectType string `json:"subject_type"`
	SubjectID   uint   `json:"subject_id"`
	RoleID      uint   `json:"role_id"`
}

// @Summary List role bindings
// @Tags rbac
// @Produce json
// @Param subject_type query string false "user or group"
// @Param subject_id query int false "Subject ID"
// @Param role_id query int false "Role ID"
// @Success 200 {object} map[string][]RoleBindingResponse
// @Router /rbac/bindings [get]
// @Security BearerAuth
func (a *API) listRoleBindings(c *gin.Context) {
	f := rbac.BindingFilter{SubjectType: c.Query("subject_type")}
	if v := c.Query("subject_id"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			rbacBadRequest(c, "invalid subject_id")
			return
		}
		f.SubjectID = uint(n)
	}
	if v := c.Query("role_id"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			rbacBadRequest(c, "invalid role_id")
			return
		}
		f.RoleID = uint(n)
	}
	bindings, err := a.service.Authz().ListBindings(c.Request.Context(), f)
	if err != nil {
		rbacErrorResponse(c, err)
		return
	}
	out := make([]RoleBindingResponse, 0, len(bindings))
	for i := range bindings {
		out = append(out, serializeBinding(&bindings[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// @Summary Assign a role
// @Tags rbac
// @Accept json
// @Produce json
// @Param binding body RoleBindingInput true "Binding"
// @Success 201 {object} map[string]RoleBindingResponse
// @Router /rbac/bindings [post]
// @Security BearerAuth
func (a *API) createRoleBinding(c *gin.Context) {
	var in RoleBindingInput
	if err := c.ShouldBindJSON(&in); err != nil {
		rbacBadRequest(c, err.Error())
		return
	}
	actor, _ := auth.UserFromContext(c)
	b, err := a.service.Authz().CreateBinding(c.Request.Context(), actor, rbac.BindingInput{
		Subject: rbac.Subject{Type: in.SubjectType, ID: in.SubjectID},
		RoleID:  in.RoleID,
	})
	if err != nil {
		rbacErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": serializeBinding(b)})
}

// @Summary Remove a role assignment
// @Tags rbac
// @Param id path int true "Binding ID"
// @Success 204
// @Router /rbac/bindings/{id} [delete]
// @Security BearerAuth
func (a *API) deleteRoleBinding(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	actor, _ := auth.UserFromContext(c)
	if err := a.service.Authz().DeleteBinding(c.Request.Context(), actor, id); err != nil {
		rbacErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

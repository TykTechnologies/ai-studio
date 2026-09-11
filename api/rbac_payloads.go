package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/rbac"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// Helpers that finish user and group payloads for the caller: mask API keys
// the caller may not see, attach the roles a subject holds, and reconcile
// role assignments sent alongside a user.

// callerPermissions returns the caller's effective permission list for
// handing to plugins; ["*"] for full administrators, empty when unknown.
func callerPermissions(c *gin.Context) []string {
	set, err := authz.Permissions(c)
	if err != nil {
		return []string{}
	}
	return set.List()
}

// revealAPIKeys reports whether the caller may see other users' API keys.
// With no authorization context on the request (TestMode without a user)
// the legacy behaviour applies and keys are shown.
func (a *API) revealAPIKeys(c *gin.Context) bool {
	if _, ok := authz.FromContext(c); !ok {
		return true
	}
	return authz.Can(c, authz.Write("users"))
}

func maskAPIKey(attrs *UserAttributes) {
	if attrs.APIKey == "" {
		return
	}
	if n := len(attrs.APIKey); n > 4 {
		attrs.APIKeyHint = attrs.APIKey[n-4:]
	} else {
		attrs.APIKeyHint = attrs.APIKey
	}
	attrs.APIKey = ""
}

// finishUsers masks API keys and attaches roles to serialised users.
func (a *API) finishUsers(c *gin.Context, users []UserResponse) []UserResponse {
	reveal := a.revealAPIKeys(c)
	var selfID uint
	if u, ok := auth.UserFromContext(c); ok {
		selfID = u.ID
	}
	ids := make([]uint, 0, len(users))
	for i := range users {
		id, _ := strconv.ParseUint(users[i].ID, 10, 64)
		ids = append(ids, uint(id))
		if !reveal && uint(id) != selfID {
			maskAPIKey(&users[i].Attributes)
		}
		for j := range users[i].Attributes.Groups {
			a.finishGroupRoles(c, &users[i].Attributes.Groups[j])
		}
	}
	if svc := a.service.Authz(); svc.Enabled() {
		if byUser, err := svc.RolesForSubjects(c.Request.Context(), models.RoleBindingSubjectUser, ids); err == nil {
			for i := range users {
				users[i].Attributes.Roles = roleSummaries(byUser[ids[i]])
			}
		}
	}
	return users
}

func (a *API) finishUser(c *gin.Context, user UserResponse) UserResponse {
	return a.finishUsers(c, []UserResponse{user})[0]
}

// finishGroup finishes a group payload: its embedded users and its roles.
func (a *API) finishGroup(c *gin.Context, group GroupResponse) GroupResponse {
	if len(group.Attributes.Users) > 0 {
		group.Attributes.Users = a.finishUsers(c, group.Attributes.Users)
	}
	a.finishGroupRoles(c, &group)
	return group
}

func (a *API) finishGroupRoles(c *gin.Context, group *GroupResponse) {
	svc := a.service.Authz()
	if !svc.Enabled() {
		return
	}
	id, _ := strconv.ParseUint(group.ID, 10, 64)
	if byGroup, err := svc.RolesForSubjects(c.Request.Context(), models.RoleBindingSubjectGroup, []uint{uint(id)}); err == nil {
		group.Attributes.Roles = roleSummaries(byGroup[uint(id)])
	}
}

// finishGroupList attaches roles to the group list rows.
func (a *API) finishGroupList(c *gin.Context, groups []GroupListResponse) []GroupListResponse {
	svc := a.service.Authz()
	if !svc.Enabled() || len(groups) == 0 {
		return groups
	}
	ids := make([]uint, 0, len(groups))
	for i := range groups {
		id, _ := strconv.ParseUint(groups[i].ID, 10, 64)
		ids = append(ids, uint(id))
	}
	if byGroup, err := svc.RolesForSubjects(c.Request.Context(), models.RoleBindingSubjectGroup, ids); err == nil {
		for i := range groups {
			groups[i].Attributes.Roles = roleSummaries(byGroup[ids[i]])
		}
	}
	return groups
}

func roleSummaries(roles []models.Role) []rbac.RoleSummary {
	if len(roles) == 0 {
		return nil
	}
	out := make([]rbac.RoleSummary, 0, len(roles))
	for i := range roles {
		out = append(out, *roleSummaryOf(&roles[i]))
	}
	return out
}

// roleIDsInput is the optional role assignment carried on a user payload.
// It is bound separately from UserInput so the shared input struct (and
// every test that spells it out) stays untouched. nil means "unchanged".
type roleIDsInput struct {
	Data struct {
		Attributes struct {
			RoleIDs *[]uint `json:"role_ids"`
		} `json:"attributes"`
	} `json:"data"`
}

func bindRoleIDs(c *gin.Context) *[]uint {
	var in roleIDsInput
	if err := c.ShouldBindBodyWith(&in, binding.JSON); err != nil {
		return nil
	}
	return in.Data.Attributes.RoleIDs
}

// reconcileUserRoles makes the user's direct role bindings equal to desired.
// It requires roles:write and applies the Owner rules through the service.
// Returns false after writing an error response.
func (a *API) reconcileUserRoles(c *gin.Context, userID uint, desired []uint) bool {
	svc := a.service.Authz()
	if !svc.Enabled() {
		c.JSON(http.StatusPaymentRequired, authz.EnterpriseRequired(rbac.ErrEnterpriseFeature.Error()))
		return false
	}
	ctx := c.Request.Context()
	current, err := svc.ListBindings(ctx, rbac.BindingFilter{SubjectType: models.RoleBindingSubjectUser, SubjectID: userID})
	if err != nil {
		rbacErrorResponse(c, err)
		return false
	}
	want := map[uint]bool{}
	for _, id := range desired {
		want[id] = true
	}
	have := map[uint]uint{} // roleID -> bindingID
	for _, b := range current {
		if b.ScopeType == "" {
			have[b.RoleID] = b.ID
		}
	}
	changed := false
	for roleID := range want {
		if _, ok := have[roleID]; !ok {
			changed = true
		}
	}
	for roleID := range have {
		if !want[roleID] {
			changed = true
		}
	}
	if !changed {
		return true
	}
	if !authz.Can(c, authz.Write("roles")) {
		c.JSON(http.StatusForbidden, authz.Denied(authz.Write("roles")))
		return false
	}
	actor, _ := auth.UserFromContext(c)
	for roleID, bindingID := range have {
		if want[roleID] {
			continue
		}
		if err := svc.DeleteBinding(ctx, actor, bindingID); err != nil && !errors.Is(err, rbac.ErrNotFound) {
			rbacErrorResponse(c, err)
			return false
		}
	}
	for roleID := range want {
		if _, ok := have[roleID]; ok {
			continue
		}
		_, err := svc.CreateBinding(ctx, actor, rbac.BindingInput{
			Subject: rbac.Subject{Type: models.RoleBindingSubjectUser, ID: userID},
			RoleID:  roleID,
		})
		if err != nil && !errors.Is(err, rbac.ErrDuplicateBinding) {
			rbacErrorResponse(c, err)
			return false
		}
	}
	return true
}

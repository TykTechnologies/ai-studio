//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Roles the host application assigns are shown as such and survive an
// administrator saving the user's roles in Studio; deleting one directly is
// refused.
func TestRBAC_HostManagedRolesAreReadOnly(t *testing.T) {
	f := setupRBACFixture(t)
	db := f.api.service.DB
	hostBinding := models.RoleBinding{SubjectType: models.RoleBindingSubjectUser, SubjectID: f.viewer.ID,
		RoleID: f.roles[models.SystemRoleAuditor], Source: models.RoleBindingSourceHost}
	require.NoError(t, db.Create(&hostBinding).Error)

	w := f.do("GET", fmt.Sprintf("/api/v1/users/%d", f.viewer.ID), nil, f.owner)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got struct{ Data UserResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	via := map[string]string{}
	for _, r := range got.Data.Attributes.Roles {
		via[r.Slug] = r.Via
	}
	assert.Equal(t, map[string]string{models.SystemRoleViewer: "direct", models.SystemRoleAuditor: "host"}, via)

	// The form sends only the roles the administrator picked.
	w = f.do("PATCH", fmt.Sprintf("/api/v1/users/%d", f.viewer.ID), map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{
		"email": f.viewer.Email, "name": f.viewer.Name, "show_chat": true, "show_portal": true, "email_verified": true,
		"role_ids": []uint{f.roles[models.SystemRoleEditor]},
	}}}, f.owner)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var bindings []models.RoleBinding
	require.NoError(t, db.Preload("Role").Where("subject_type = ? AND subject_id = ?", models.RoleBindingSubjectUser, f.viewer.ID).Find(&bindings).Error)
	held := map[string]string{}
	for _, b := range bindings {
		held[b.Role.Slug] = b.Source
	}
	assert.Equal(t, map[string]string{models.SystemRoleEditor: "", models.SystemRoleAuditor: models.RoleBindingSourceHost}, held,
		"the administrator's change applied; the host's role kept")

	w = f.do("DELETE", fmt.Sprintf("/api/v1/rbac/bindings/%d", hostBinding.ID), nil, f.owner)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "embedded in")
}

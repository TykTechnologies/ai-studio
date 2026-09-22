//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The effective-access payload carries, per permission, which role granted
// it and whether that was a direct binding or one on a team the user is in.
func TestRBAC_EffectiveAccessCarriesSources(t *testing.T) {
	f := setupRBACFixture(t)
	db := f.api.service.DB

	team := &models.Group{Name: "Platform"}
	require.NoError(t, db.Create(team).Error)
	require.NoError(t, db.Model(team).Association("Users").Append(f.viewer))
	f.bind(t, "group", team.ID, f.roles[models.SystemRoleEditor])

	w := f.do("GET", fmt.Sprintf("/api/v1/rbac/users/%d/effective", f.viewer.ID), nil, f.owner)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var eff EffectiveAccessResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &eff))

	require.Len(t, eff.Sources["llms:write"], 1)
	write := eff.Sources["llms:write"][0]
	assert.Equal(t, f.roles[models.SystemRoleEditor], write.RoleID)
	assert.Equal(t, "Editor", write.RoleName)
	assert.Equal(t, "group", write.Via)
	assert.Equal(t, team.ID, write.GroupID)
	assert.Equal(t, "Platform", write.GroupName)

	require.Len(t, eff.Sources["llms:read"], 2)
	assert.Equal(t, "group", eff.Sources["llms:read"][0].Via)
	assert.Equal(t, "direct", eff.Sources["llms:read"][1].Via)
	assert.Equal(t, "Viewer", eff.Sources["llms:read"][1].RoleName)
	assert.Empty(t, eff.Sources["llms:read"][1].GroupName)

	// The raw JSON uses the documented snake_case keys and omits group fields
	// on a direct binding.
	var raw struct {
		Sources map[string][]map[string]interface{} `json:"sources"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	direct := raw.Sources["llms:read"][1]
	assert.Equal(t, "Viewer", direct["role_name"])
	assert.NotContains(t, direct, "group_name")
	assert.Contains(t, raw.Sources["llms:write"][0], "group_name")

	// Every listed permission is attributed.
	for _, p := range eff.Permissions {
		assert.NotEmpty(t, eff.Sources[p], p)
	}
}

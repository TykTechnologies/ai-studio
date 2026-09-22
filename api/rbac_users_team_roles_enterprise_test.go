//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The users list shows effective roles: roles bound to a user's teams are
// listed next to the direct ones, marked via=group with the team named, and a
// role held both ways appears once (direct wins).
func TestRBAC_UsersListIncludesTeamRoles(t *testing.T) {
	f := setupRBACFixture(t)
	db := f.api.service.DB

	teamOnly := models.NewUser()
	teamOnly.Email = "teamonly@tyk.io"
	teamOnly.Name = "Team Only"
	teamOnly.Password = "hash"
	teamOnly.EmailVerified = true
	require.NoError(t, teamOnly.Create(db))

	team := &models.Group{Name: "Platform"}
	require.NoError(t, db.Create(team).Error)
	require.NoError(t, db.Model(team).Association("Users").Append(teamOnly, f.viewer))

	// The team carries Editor and Viewer; the viewer already holds Viewer directly.
	f.bind(t, "group", team.ID, f.roles[models.SystemRoleEditor])
	f.bind(t, "group", team.ID, f.roles[models.SystemRoleViewer])

	w := f.do("GET", "/api/v1/users?all=true", nil, f.owner)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list struct{ Data []UserResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))

	byID := map[uint]UserResponse{}
	for _, u := range list.Data {
		byID[uintFromString(t, u.ID)] = u
	}

	// No direct roles: the team role is the only one, attributed to the team.
	u, ok := byID[teamOnly.ID]
	require.True(t, ok, "team-only user is listed")
	require.Len(t, u.Attributes.Roles, 2, "%+v", u.Attributes.Roles)
	slugs := map[string]bool{}
	for _, r := range u.Attributes.Roles {
		slugs[r.Slug] = true
		assert.Equal(t, "group", r.Via)
		assert.Equal(t, team.ID, r.GroupID)
		assert.Equal(t, "Platform", r.GroupName)
	}
	assert.True(t, slugs[models.SystemRoleEditor])
	assert.True(t, slugs[models.SystemRoleViewer])

	// Direct wins over the team copy of the same role; the extra team role is added.
	v := byID[f.viewer.ID]
	require.Len(t, v.Attributes.Roles, 2, "%+v", v.Attributes.Roles)
	var viewerRole, editorRole *struct {
		Via, GroupName string
		GroupID        uint
	}
	for _, r := range v.Attributes.Roles {
		s := &struct {
			Via, GroupName string
			GroupID        uint
		}{r.Via, r.GroupName, r.GroupID}
		switch r.Slug {
		case models.SystemRoleViewer:
			viewerRole = s
		case models.SystemRoleEditor:
			editorRole = s
		}
	}
	require.NotNil(t, viewerRole)
	require.NotNil(t, editorRole)
	assert.Equal(t, "direct", viewerRole.Via)
	assert.Empty(t, viewerRole.GroupName)
	assert.Equal(t, "group", editorRole.Via)
	assert.Equal(t, team.ID, editorRole.GroupID)
	assert.Equal(t, "Platform", editorRole.GroupName)

	// A user in no team keeps direct roles only.
	e := byID[f.editor.ID]
	require.Len(t, e.Attributes.Roles, 1)
	assert.Equal(t, "direct", e.Attributes.Roles[0].Via)

	// The single-user endpoint agrees.
	w = f.do("GET", "/api/v1/users/"+u.ID, nil, f.owner)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var one struct{ Data UserResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &one))
	require.Len(t, one.Data.Attributes.Roles, 2)
	assert.Equal(t, "Platform", one.Data.Attributes.Roles[0].GroupName)
}

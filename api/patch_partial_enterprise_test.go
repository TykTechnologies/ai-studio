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

// A PATCH of a custom role's name or description keeps its permissions; an
// explicit empty list still clears them (midsommar-clean-96e).
func TestRBAC_RolePatchKeepsOmittedAttributes(t *testing.T) {
	f := setupRBACFixture(t)
	w := f.do("POST", fmt.Sprintf("/api/v1/rbac/roles/%d/clone", f.roles[models.SystemRoleViewer]), map[string]interface{}{"name": "Patch me"}, f.owner)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct{ Data RoleResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	id := uintFromString(t, created.Data.ID)
	perms := created.Data.Attributes.Permissions
	require.NotEmpty(t, perms)

	w = f.do("PATCH", fmt.Sprintf("/api/v1/rbac/roles/%d", id), map[string]interface{}{"description": "only this"}, f.owner)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got struct{ Data RoleResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "Patch me", got.Data.Attributes.Name)
	assert.Equal(t, "only this", got.Data.Attributes.Description)
	assert.ElementsMatch(t, perms, got.Data.Attributes.Permissions)

	w = f.do("PATCH", fmt.Sprintf("/api/v1/rbac/roles/%d", id), map[string]interface{}{"permissions": []string{}}, f.owner)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Empty(t, got.Data.Attributes.Permissions)
	assert.Equal(t, "Patch me", got.Data.Attributes.Name)
}

// Groups are Enterprise; a rename keeps the members and catalogues.
func TestUpdateGroup_PartialPatchKeepsAccess(t *testing.T) {
	api, db := setupTestAPI(t)
	member := &models.User{Email: "member@example.com", Name: "Member", EmailVerified: true}
	require.NoError(t, member.Create(db))
	cat := &models.Catalogue{Name: "LLM cat"}
	require.NoError(t, db.Create(cat).Error)
	dcat := &models.DataCatalogue{Name: "Data cat"}
	require.NoError(t, db.Create(dcat).Error)
	tcat := &models.ToolCatalogue{Name: "Tool cat"}
	require.NoError(t, db.Create(tcat).Error)
	g := &models.Group{Name: "Team", Users: []models.User{*member}, Catalogues: []models.Catalogue{*cat},
		DataCatalogues: []models.DataCatalogue{*dcat}, ToolCatalogues: []models.ToolCatalogue{*tcat}}
	require.NoError(t, db.Create(g).Error)
	reload := func() *models.Group {
		got := models.NewGroup()
		require.NoError(t, got.Get(db, g.ID, "Users", "Catalogues", "DataCatalogues", "ToolCatalogues"))
		return got
	}

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/groups/%d", g.ID), patchAttrs(map[string]interface{}{"name": "Renamed team"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got := reload()
	assert.Equal(t, "Renamed team", got.Name)
	assert.Len(t, got.Users, 1, "a rename keeps the members")
	assert.Len(t, got.Catalogues, 1)
	assert.Len(t, got.DataCatalogues, 1)
	assert.Len(t, got.ToolCatalogues, 1)

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/groups/%d", g.ID), patchAttrs(map[string]interface{}{"members": []uint{}}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got = reload()
	assert.Empty(t, got.Users, "an explicit empty list clears")
	assert.Len(t, got.Catalogues, 1)
	assert.Equal(t, "Renamed team", got.Name)
}

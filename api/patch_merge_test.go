package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A partial PATCH changes only the attributes the body holds: on
// datasources, tools and embedders it used to blank the name, owner,
// privacy score and credentials (midsommar-clean-96e).

func patchAttrs(attrs map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"data": map[string]interface{}{"attributes": attrs}}
}

func TestUpdateDatasource_PartialPatchKeepsTheRest(t *testing.T) {
	api, db := setupTestAPI(t)
	owner := &models.User{Email: "ds-owner@example.com", Name: "Owner", EmailVerified: true}
	require.NoError(t, owner.Create(db))
	ds := &models.Datasource{Name: "Keep DS", ShortDescription: "old", PrivacyScore: 3, UserID: owner.ID,
		DBSourceType: "pgvector", DBConnString: "postgres://u:p@pg.example:5432/vec", DBConnAPIKey: "ds-key",
		DBName: "vec", Active: true, Tags: []models.Tag{{Name: "keep"}}}
	require.NoError(t, db.Create(ds).Error)

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/datasources/%d", ds.ID),
		patchAttrs(map[string]interface{}{"short_description": "new"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	got := models.NewDatasource()
	require.NoError(t, got.Get(db, ds.ID))
	assert.Equal(t, "new", got.ShortDescription)
	assert.Equal(t, "Keep DS", got.Name)
	assert.Equal(t, owner.ID, got.UserID)
	assert.Equal(t, 3, got.PrivacyScore)
	assert.Equal(t, "ds-key", got.DBConnAPIKey)
	assert.Equal(t, "postgres://u:p@pg.example:5432/vec", got.DBConnString)
	assert.Equal(t, "pgvector", got.DBSourceType)
	assert.Equal(t, "vec", got.DBName)
	assert.True(t, got.Active)
	require.Len(t, got.Tags, 1)
	assert.Equal(t, "keep", got.Tags[0].Name)

	// A present attribute still changes, empty included.
	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/datasources/%d", ds.ID),
		patchAttrs(map[string]interface{}{"name": "Renamed", "tags": []string{}}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got = models.NewDatasource()
	require.NoError(t, got.Get(db, ds.ID))
	assert.Equal(t, "Renamed", got.Name)
	assert.Empty(t, got.Tags)
	assert.Equal(t, "ds-key", got.DBConnAPIKey)
}

func TestUpdateTool_PartialPatchKeepsTheRest(t *testing.T) {
	api, db := setupTestAPI(t)
	tool := &models.Tool{Name: "Keep Tool", Description: "old", PrivacyScore: 4,
		AuthKey: "tool-key", AuthSchemaName: "bearerAuth", Active: true}
	tool.AddOperation("getThing")
	require.NoError(t, db.Create(tool).Error)

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tools/%d", tool.ID),
		patchAttrs(map[string]interface{}{"description": "new"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	got := models.NewTool()
	require.NoError(t, got.Get(db, tool.ID))
	assert.Equal(t, "new", got.Description)
	assert.Equal(t, "Keep Tool", got.Name)
	assert.Equal(t, 4, got.PrivacyScore)
	assert.Equal(t, "tool-key", got.AuthKey)
	assert.Equal(t, "bearerAuth", got.AuthSchemaName)
	assert.Equal(t, []string{"getThing"}, got.GetOperations())
	assert.True(t, got.Active)

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tools/%d", tool.ID),
		patchAttrs(map[string]interface{}{"name": "Renamed", "operations": []string{}}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got = models.NewTool()
	require.NoError(t, got.Get(db, tool.ID))
	assert.Equal(t, "Renamed", got.Name)
	assert.Empty(t, got.GetOperations())
	assert.Equal(t, "tool-key", got.AuthKey)
}

func TestUpdateEmbedder_PartialPatchKeepsTheRest(t *testing.T) {
	api, db := setupTestAPI(t)
	e := &models.Embedder{Name: "Keep Embedder", Description: "old", Vendor: models.OPENAI,
		Endpoint: "https://api.openai.com/v1", APIKey: "emb-key", ModelName: "text-embedding-3-small", PrivacyScore: 4}
	require.NoError(t, db.Create(e).Error)

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/embedders/%d", e.ID),
		patchAttrs(map[string]interface{}{"description": "new"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got models.Embedder
	require.NoError(t, got.Get(db, e.ID))
	assert.Equal(t, "new", got.Description)
	assert.Equal(t, "Keep Embedder", got.Name)
	assert.Equal(t, models.OPENAI, got.Vendor)
	assert.Equal(t, "https://api.openai.com/v1", got.Endpoint)
	assert.Equal(t, "emb-key", got.APIKey)
	assert.Equal(t, "text-embedding-3-small", got.ModelName)
	assert.Equal(t, 4, got.PrivacyScore)

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/embedders/%d", e.ID),
		patchAttrs(map[string]interface{}{"name": "Renamed"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, got.Get(db, e.ID))
	assert.Equal(t, "Renamed", got.Name)
	assert.Equal(t, "emb-key", got.APIKey)
}

func TestUpdateUser_PartialPatchKeepsTheRest(t *testing.T) {
	api, _, _ := setupTestAPIWithAdminUser(t)
	db := api.service.DB
	u := &models.User{Email: "keep@example.com", Name: "Keep Me", EmailVerified: true, ShowChat: true, ShowPortal: true, AccessToSSOConfig: false}
	require.NoError(t, u.Create(db))
	g := &models.Group{Name: "Keep group"}
	require.NoError(t, db.Create(g).Error)
	require.NoError(t, db.Model(u).Association("Groups").Append(g))
	reload := func() *models.User {
		got := models.NewUser()
		require.NoError(t, got.Get(db, u.ID, "Groups"))
		return got
	}

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/users/%d", u.ID), patchAttrs(map[string]interface{}{"name": "Renamed"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got := reload()
	assert.Equal(t, "Renamed", got.Name)
	assert.Equal(t, "keep@example.com", got.Email)
	assert.True(t, got.EmailVerified)
	assert.True(t, got.ShowChat)
	assert.True(t, got.ShowPortal)
	require.Len(t, got.Groups, 1, "a rename keeps the groups")

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/users/%d", u.ID), patchAttrs(map[string]interface{}{"groups": []uint{}, "show_chat": false}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got = reload()
	assert.Empty(t, got.Groups, "an explicit empty list clears")
	assert.False(t, got.ShowChat, "an explicit false applies")
	assert.True(t, got.EmailVerified)
}

func TestUpdateChat_PartialPatchKeepsAccessAndFilters(t *testing.T) {
	api, db := setupTestAPI(t)
	llm := &models.LLM{Name: "Chat LLM", Vendor: "openai", DefaultModel: "m", PrivacyScore: 5, Active: true}
	require.NoError(t, db.Create(llm).Error)
	g := &models.Group{Name: "Chat group"}
	require.NoError(t, db.Create(g).Error)
	f := &models.Filter{Name: "Guard"}
	require.NoError(t, db.Create(f).Error)
	chat := &models.Chat{Name: "Keep chat", Description: "d", LLMID: llm.ID, SystemPrompt: "be nice", SupportsTools: true,
		RagResultsPerSource: 3, Groups: []models.Group{*g}, Filters: []*models.Filter{f}}
	require.NoError(t, db.Create(chat).Error)
	reload := func() *models.Chat {
		got := &models.Chat{}
		require.NoError(t, got.Get(db, chat.ID))
		return got
	}

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/chats/%d", chat.ID), patchAttrs(map[string]interface{}{"name": "Renamed chat"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got := reload()
	assert.Equal(t, "Renamed chat", got.Name)
	assert.Equal(t, "d", got.Description)
	assert.Equal(t, llm.ID, got.LLMID)
	assert.Equal(t, "be nice", got.SystemPrompt)
	assert.True(t, got.SupportsTools)
	assert.Equal(t, 3, got.RagResultsPerSource)
	assert.Len(t, got.Groups, 1, "a rename keeps the chat's groups")
	assert.Len(t, got.Filters, 1, "a rename keeps the chat's filters")

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/chats/%d", chat.ID), patchAttrs(map[string]interface{}{"filter_ids": []uint{}, "tool_support": false}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got = reload()
	assert.Empty(t, got.Filters, "an explicit empty list clears")
	assert.False(t, got.SupportsTools)
	assert.Len(t, got.Groups, 1)
}

func TestUpdateCredential_OmittedActiveIsKept(t *testing.T) {
	api, db := setupTestAPI(t)
	cred := &models.Credential{KeyID: "kid", Secret: "sec", Active: true}
	require.NoError(t, db.Create(cred).Error)

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/credentials/%d", cred.ID), patchAttrs(map[string]interface{}{}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got models.Credential
	require.NoError(t, db.First(&got, cred.ID).Error)
	assert.True(t, got.Active, "an omitted active keeps the credential live")

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/credentials/%d", cred.ID), patchAttrs(map[string]interface{}{"active": false}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, db.First(&got, cred.ID).Error)
	assert.False(t, got.Active)
}

// The model-router toggle needs active: an empty body used to deactivate
// the router (midsommar-clean-96e).
func TestToggleModelRouter_RequiresActive(t *testing.T) {
	api, db := setupTestAPI(t)
	r := &models.ModelRouter{Name: "Toggle me", Slug: "toggle-me", Active: true}
	require.NoError(t, db.Create(r).Error)

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/model-routers/%d/toggle", r.ID), map[string]interface{}{})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var got models.ModelRouter
	require.NoError(t, db.First(&got, r.ID).Error)
	assert.True(t, got.Active, "an empty body leaves the router live")

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/model-routers/%d/toggle", r.ID), map[string]interface{}{"active": false})
	assert.NotEqual(t, http.StatusBadRequest, w.Code, "an explicit active is accepted: %s", w.Body.String())
}

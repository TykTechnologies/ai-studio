package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
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

// A partial PATCH keeps the datasource's embedder: an omitted
// embed_api_key used to bind as "" (which clears a key), so the datasource
// was moved to a new keyless copy of its embedder.
func TestUpdateDatasource_PartialPatchKeepsKeyedEmbedder(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")
	api, db := setupTestAPI(t)
	emb, err := api.service.CreateEmbedder(&models.Embedder{Name: "Keyed", Vendor: models.OPENAI,
		Endpoint: "https://api.openai.com/v1", APIKey: "sk-keep", ModelName: "text-embedding-3-small", PrivacyScore: 5}, 1)
	require.NoError(t, err)
	ds := &models.Datasource{Name: "Embedded DS", PrivacyScore: 3, DBSourceType: "pgvector",
		DBConnString: "postgres://u:p@pg.example:5432/vec", EmbedderID: &emb.ID}
	require.NoError(t, db.Create(ds).Error)
	var before int64
	require.NoError(t, db.Model(&models.Embedder{}).Count(&before).Error)

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/datasources/%d", ds.ID),
		patchAttrs(map[string]interface{}{"short_description": "partial"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	got := models.NewDatasource()
	require.NoError(t, got.Get(db, ds.ID))
	require.NotNil(t, got.EmbedderID)
	assert.Equal(t, emb.ID, *got.EmbedderID, "the datasource keeps its embedder")
	var after int64
	require.NoError(t, db.Model(&models.Embedder{}).Count(&after).Error)
	assert.Equal(t, before, after, "no new embedder is created")
	var stored models.Embedder
	require.NoError(t, stored.Get(db, emb.ID))
	assert.Equal(t, "sk-keep", stored.APIKey)
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

func TestUpdateLLMSettings_PartialPatchKeepsTheRest(t *testing.T) {
	api, db := setupTestAPI(t)
	s := &models.LLMSettings{ModelName: "gpt-x", MaxTokens: 512, Temperature: 0.7, TopP: 0.9, SystemPrompt: "keep", StopWords: []string{"END"}}
	require.NoError(t, db.Create(s).Error)

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/llm-settings/%d", s.ID), patchAttrs(map[string]interface{}{"max_tokens": 1024}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got models.LLMSettings
	require.NoError(t, db.First(&got, s.ID).Error)
	assert.Equal(t, 1024, got.MaxTokens)
	assert.Equal(t, "gpt-x", got.ModelName)
	assert.Equal(t, 0.7, got.Temperature)
	assert.Equal(t, 0.9, got.TopP)
	assert.Equal(t, "keep", got.SystemPrompt)
	assert.Equal(t, []string{"END"}, got.StopWords)

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/llm-settings/%d", s.ID), patchAttrs(map[string]interface{}{"temperature": 0, "stop_words": []string{}}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got = models.LLMSettings{}
	require.NoError(t, db.First(&got, s.ID).Error)
	assert.Zero(t, got.Temperature, "an explicit 0 applies")
	assert.Empty(t, got.StopWords)
	assert.Equal(t, "gpt-x", got.ModelName)
}

func TestUpdateModelPrice_PartialPatchKeepsThePrices(t *testing.T) {
	api, db := setupTestAPI(t)
	p := &models.ModelPrice{ModelName: "m-price", Vendor: "openai", CPT: 0.00002, CPIT: 0.00001, CacheReadPT: 0.000001, Currency: "USD"}
	require.NoError(t, db.Create(p).Error)
	reload := func() models.ModelPrice {
		var got models.ModelPrice
		require.NoError(t, db.First(&got, p.ID).Error)
		return got
	}

	for _, path := range []string{"/api/v1/model-prices/%d", "/api/v1/model-prices/%d/recalculate"} {
		w := performRequest(api.router, "PATCH", fmt.Sprintf(path, p.ID), patchAttrs(map[string]interface{}{"cpit": 0.000015}))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		got := reload()
		assert.Equal(t, 0.000015, got.CPIT, path)
		assert.Equal(t, 0.00002, got.CPT, "%s: an omitted price is kept, not zeroed", path)
		assert.Equal(t, 0.000001, got.CacheReadPT, path)
		assert.Equal(t, "m-price", got.ModelName, path)
		assert.Equal(t, "openai", got.Vendor, path)
		assert.Equal(t, "USD", got.Currency, path)
	}

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/model-prices/%d", p.ID), patchAttrs(map[string]interface{}{"cache_read_pt": 0}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Zero(t, reload().CacheReadPT, "an explicit 0 applies")
}

func TestUpdateFileStore_PartialPatchKeepsContent(t *testing.T) {
	api, db := setupTestAPI(t)
	f := &models.FileStore{FileName: "keep.txt", Description: "old", Content: "the content", Length: 11}
	require.NoError(t, db.Create(f).Error)

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/filestore/%d", f.ID), patchAttrs(map[string]interface{}{"description": "new"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got models.FileStore
	require.NoError(t, db.First(&got, f.ID).Error)
	assert.Equal(t, "new", got.Description)
	assert.Equal(t, "keep.txt", got.FileName)
	assert.Equal(t, "the content", got.Content)
	assert.Equal(t, 11, got.Length)

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/filestore/%d", f.ID), patchAttrs(map[string]interface{}{"description": ""}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got = models.FileStore{}
	require.NoError(t, db.First(&got, f.ID).Error)
	assert.Empty(t, got.Description, "an explicit empty string applies")
	assert.Equal(t, "the content", got.Content)
}

func TestUpdateAttestationTemplate_PartialPatchKeepsTheRest(t *testing.T) {
	api, db := setupTestAPI(t)
	tpl := &models.AttestationTemplate{Name: "Keep", Text: "I attest", Required: true, AppliesToType: "tool", Active: true, SortOrder: 3}
	require.NoError(t, db.Create(tpl).Error)

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/attestation-templates/%d", tpl.ID), patchAttrs(map[string]interface{}{"name": "Renamed"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got models.AttestationTemplate
	require.NoError(t, db.First(&got, tpl.ID).Error)
	assert.Equal(t, "Renamed", got.Name)
	assert.Equal(t, "I attest", got.Text)
	assert.True(t, got.Required)
	assert.True(t, got.Active)
	assert.Equal(t, "tool", got.AppliesToType)
	assert.Equal(t, 3, got.SortOrder)

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/attestation-templates/%d", tpl.ID), patchAttrs(map[string]interface{}{"required": false}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got = models.AttestationTemplate{}
	require.NoError(t, db.First(&got, tpl.ID).Error)
	assert.False(t, got.Required, "an explicit false applies")
	assert.True(t, got.Active)
}

func TestUpdateTag_OmittedNameIsKept(t *testing.T) {
	api, db := setupTestAPI(t)
	tag := &models.Tag{Name: "keep-tag"}
	require.NoError(t, db.Create(tag).Error)

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tags/%d", tag.ID), patchAttrs(map[string]interface{}{}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got models.Tag
	require.NoError(t, db.First(&got, tag.ID).Error)
	assert.Equal(t, "keep-tag", got.Name)
}

func TestUpdateMySubmission_PartialPatchKeepsTheRest(t *testing.T) {
	api, db := setupTestAPI(t)
	user := &models.User{Email: "submitter@example.com", Name: "Submitter", EmailVerified: true}
	require.NoError(t, user.Create(db))
	sub, err := api.service.CreateSubmission(user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{"name": "My DS", "db_source_type": "pgvector"}, nil, 3, "Product data only",
		"owner@example.com", "backup@example.com", "", nil, "https://docs.example.com", "keep notes")
	require.NoError(t, err)

	router := gin.New()
	group := router.Group("/common")
	group.Use(func(c *gin.Context) { c.Set("user", user); c.Next() })
	group.PATCH("/submissions/:id", api.updateMySubmission)

	w := performRequest(router, "PATCH", fmt.Sprintf("/common/submissions/%d", sub.ID), patchAttrs(map[string]interface{}{"notes": "new notes"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got, err := api.service.GetSubmissionByID(sub.ID)
	require.NoError(t, err)
	assert.Equal(t, "new notes", got.Notes)
	assert.Equal(t, "My DS", got.ResourcePayload["name"], "an omitted payload is kept")
	assert.Equal(t, 3, got.SuggestedPrivacy)
	assert.Equal(t, "Product data only", got.PrivacyJustification)
	assert.Equal(t, "owner@example.com", got.PrimaryContact)
	assert.Equal(t, "backup@example.com", got.SecondaryContact)
	assert.Equal(t, "https://docs.example.com", got.DocumentationURL)

	w = performRequest(router, "PATCH", fmt.Sprintf("/common/submissions/%d", sub.ID), patchAttrs(map[string]interface{}{"secondary_contact": ""}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got, err = api.service.GetSubmissionByID(sub.ID)
	require.NoError(t, err)
	assert.Empty(t, got.SecondaryContact, "an explicit empty string applies")
	assert.Equal(t, "owner@example.com", got.PrimaryContact)
}

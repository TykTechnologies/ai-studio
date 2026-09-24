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

// Through the real middleware and the Enterprise evaluator: embedders:write
// manages standalone embedders, but linking one to an LLM (which spends the
// LLM's credentials) also needs llms:read. Built-in roles follow the
// catalogue defaults.
func TestRBAC_Embedders(t *testing.T) {
	f := setupRBACFixture(t)
	db := f.api.service.DB

	newUserWithRole := func(email string, perms ...string) *models.User {
		u := models.NewUser()
		u.Email, u.Name, u.Password, u.EmailVerified = email, email, "hash", true
		require.NoError(t, u.Create(db))
		w := f.do("POST", "/api/v1/rbac/roles", map[string]interface{}{"name": "role-" + email, "permissions": perms}, f.owner)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var resp struct{ Data RoleResponse }
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		f.bind(t, "user", u.ID, uintFromString(t, resp.Data.ID))
		return u
	}
	llm := &models.LLM{Name: "Prod OpenAI", Vendor: models.OPENAI, PrivacyScore: 80, Active: true}
	require.NoError(t, db.Create(llm).Error)

	body := func(attrs map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"data": map[string]interface{}{"type": "embedders", "attributes": attrs}}
	}
	standaloneBody := func(name string) map[string]interface{} {
		return body(map[string]interface{}{"name": name, "vendor": "openai", "model": "m", "privacy_score": 50})
	}
	linkedBody := func(name string) map[string]interface{} {
		return body(map[string]interface{}{"name": name, "llm_id": llm.ID, "model": "text-embedding-3-small"})
	}

	writerOnly := newUserWithRole("emb-writer@tyk.io", "embedders:read", "embedders:write")
	writerWithLLMs := newUserWithRole("emb-llm@tyk.io", "embedders:read", "embedders:write", "llms:read")

	w := f.do("POST", "/api/v1/embedders", standaloneBody("mine"), writerOnly)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct{ Data struct{ ID string } }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	w = f.do("POST", "/api/v1/embedders", linkedBody("linked"), writerOnly)
	assert.Equal(t, http.StatusForbidden, w.Code, "linking needs llms:read: %s", w.Body.String())
	assert.Contains(t, w.Body.String(), "llms:read")

	w = f.do("PATCH", "/api/v1/embedders/"+created.Data.ID, linkedBody("mine"), writerOnly)
	assert.Equal(t, http.StatusForbidden, w.Code, "relinking needs llms:read too")

	w = f.do("POST", "/api/v1/embedders", linkedBody("linked"), writerWithLLMs)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var linked struct{ Data struct{ ID string } }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &linked))

	// Editing a linked embedder without changing its LLM does not need
	// llms:read (the link already exists).
	w = f.do("PATCH", "/api/v1/embedders/"+linked.Data.ID,
		body(map[string]interface{}{"name": "linked renamed", "llm_id": llm.ID, "model": "text-embedding-3-small"}), writerOnly)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Catalogue defaults: Viewer reads, Editor manages.
	w = f.do("GET", "/api/v1/embedders", nil, f.viewer)
	assert.Equal(t, http.StatusOK, w.Code)
	w = f.do("POST", "/api/v1/embedders", standaloneBody("viewer"), f.viewer)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = f.do("POST", "/api/v1/embedders", linkedBody("editor"), f.editor)
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	w = f.do("DELETE", fmt.Sprintf("/api/v1/embedders/%s", created.Data.ID), nil, f.viewer)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

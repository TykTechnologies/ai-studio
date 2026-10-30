package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppPromptTemplateHandlers(t *testing.T) {
	api, db := setupTestAPI(t)
	app := &models.App{Name: "pr-review bot", UserID: 1}
	require.NoError(t, app.Create(db))
	base := fmt.Sprintf("/api/v1/apps/%d/prompt-templates", app.ID)

	decode := func(body []byte) AppPromptTemplateResponse {
		var resp struct {
			Data AppPromptTemplateResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &resp), string(body))
		return resp.Data
	}

	in := map[string]any{
		"name":      "pr-review",
		"placement": map[string]string{"role": "user", "anchor": "both"},
		"parts": []map[string]string{
			{"const": "You are reviewing a pull request. Follow the rules.\n<diff>\n"},
			{"slot": "diff"},
			{"const": "\n</diff>\nReport only real defects, one per line.\n"},
		},
	}
	w := performRequest(api.router, http.MethodPost, base, in)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decode(w.Body.Bytes())
	assert.Equal(t, "pr-review@1", created.Ref)
	assert.Equal(t, "proposed", created.Status)
	assert.Equal(t, "any", created.Placement.Message)

	w = performRequest(api.router, http.MethodPost, base, map[string]any{"name": "x", "placement": map[string]string{"role": "user"}, "parts": []map[string]string{{"const": "short"}}})
	assert.Equal(t, http.StatusBadRequest, w.Code, "a constant under 32 bytes")

	item := fmt.Sprintf("%s/%d", base, created.ID)
	w = performRequest(api.router, http.MethodPost, item+"/approve", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "approved", decode(w.Body.Bytes()).Status)

	w = performRequest(api.router, http.MethodPost, item+"/approve", nil)
	assert.Equal(t, http.StatusConflict, w.Code)

	w = performRequest(api.router, http.MethodPatch, item, map[string]any{"parts": []map[string]string{
		{"const": "You are reviewing a pull request. Follow the rules.\n<diff>\n"},
		{"slot": "diff"},
		{"const": "\n</diff>\nReport every defect, one per line, with its line.\n"},
	}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	v2 := decode(w.Body.Bytes())
	assert.Equal(t, 2, v2.Version)
	assert.Equal(t, "proposed", v2.Status)

	w = performRequest(api.router, http.MethodGet, base+"?status=approved", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var list struct {
		Data []AppPromptTemplateResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 1)
	assert.Equal(t, 1, list.Data[0].Version)

	w = performRequest(api.router, http.MethodPost, item+"/retire", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "retired", decode(w.Body.Bytes()).Status)

	w = performRequest(api.router, http.MethodDelete, fmt.Sprintf("%s/%d", base, v2.ID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
	w = performRequest(api.router, http.MethodGet, fmt.Sprintf("%s/%d", base, v2.ID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = performRequest(api.router, http.MethodGet, fmt.Sprintf("/api/v1/apps/%d/prompt-templates/%d", app.ID+1, created.ID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "another app's template")
}

// Approving exempts bytes from guardrail scoring, so it is for full
// administrators only; proposing and retiring are app writes.
func TestAppPromptTemplateRoutes_Permissions(t *testing.T) {
	api, _ := setupTestAPI(t)
	get := func(method, path string) authz.Permission {
		e, ok := api.routePerms[routeKey(method, path)]
		require.True(t, ok, "%s %s", method, path)
		return e.perm
	}
	assert.Equal(t, authz.FullAdmin, get("POST", "/api/v1/apps/:id/prompt-templates/:template_id/approve"))
	assert.Equal(t, authz.Write("apps"), get("POST", "/api/v1/apps/:id/prompt-templates"))
	assert.Equal(t, authz.Write("apps"), get("PATCH", "/api/v1/apps/:id/prompt-templates/:template_id"))
	assert.Equal(t, authz.Write("apps"), get("POST", "/api/v1/apps/:id/prompt-templates/:template_id/retire"))
	assert.Equal(t, authz.Read("apps"), get("GET", "/api/v1/apps/:id/prompt-templates"))
	assert.Equal(t, authz.Delete("apps"), get("DELETE", "/api/v1/apps/:id/prompt-templates/:template_id"))
}

func TestAppPromptTemplateRoutes_NonAdminDenied(t *testing.T) {
	api, _, member := setupAuthzAPI(t)
	w := apitest.PerformAuthRequest(api.router, http.MethodPost, "/api/v1/apps/1/prompt-templates/1/approve", nil, member.APIKey)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

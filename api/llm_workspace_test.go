package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

func llmWorkspaceOf(t *testing.T, body []byte) string {
	t.Helper()
	var resp struct {
		Data LLMResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp.Data.Attributes.VendorWorkspaceID
}

// vendor_workspace_id is set on create, kept by a PATCH that omits it,
// cleared by "", and validated.
func TestLLMVendorWorkspaceID(t *testing.T) {
	api, db := setupTestAPI(t)
	create := func(ws string) (int, []byte) {
		body := map[string]any{"data": map[string]any{"type": "llms", "attributes": map[string]any{
			"name": "Claude " + ws, "vendor": "anthropic", "api_endpoint": "https://api.anthropic.com",
			"privacy_score": 5, "vendor_workspace_id": ws,
		}}}
		w := performRequest(api.router, http.MethodPost, "/api/v1/llms", body)
		return w.Code, w.Body.Bytes()
	}
	code, body := create("wrkspc_01ABC")
	require.Equal(t, http.StatusCreated, code, string(body))
	assert.Equal(t, "wrkspc_01ABC", llmWorkspaceOf(t, body))
	var stored models.LLM
	require.NoError(t, db.Where("name = ?", "Claude wrkspc_01ABC").First(&stored).Error)
	assert.Equal(t, "wrkspc_01ABC", stored.VendorWorkspaceID)
	path := "/api/v1/llms/" + idStr(stored.ID)

	patch := func(attrs map[string]any) (int, []byte) {
		w := performRequest(api.router, http.MethodPatch, path, map[string]any{"data": map[string]any{"type": "llms", "attributes": attrs}})
		return w.Code, w.Body.Bytes()
	}
	code, body = patch(map[string]any{"short_description": "edited"})
	require.Equal(t, http.StatusOK, code, string(body))
	assert.Equal(t, "wrkspc_01ABC", llmWorkspaceOf(t, body), "a PATCH that omits it keeps the pin")

	code, body = patch(map[string]any{"vendor_workspace_id": "wrkspc_other"})
	require.Equal(t, http.StatusOK, code, string(body))
	assert.Equal(t, "wrkspc_other", llmWorkspaceOf(t, body))

	for _, bad := range []string{"wrkspc bad", "wrkspc\r\nX-Injected: 1", string(make([]byte, 129))} {
		code, body = patch(map[string]any{"vendor_workspace_id": bad})
		assert.Equal(t, http.StatusBadRequest, code, string(body))
	}
	code, body = create("bad value")
	assert.Equal(t, http.StatusBadRequest, code, string(body))

	code, body = patch(map[string]any{"vendor_workspace_id": ""})
	require.Equal(t, http.StatusOK, code, string(body))
	assert.Empty(t, llmWorkspaceOf(t, body), `"" removes the pin`)

	w := performRequest(api.router, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"vendor_workspace_id":""`)
}

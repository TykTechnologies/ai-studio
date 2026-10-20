//go:build !enterprise
// +build !enterprise

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTykMCPCommunity_NotAvailable(t *testing.T) {
	r, key := setupWebhooksCommunityAPI(t)
	assert.False(t, tykmcp.IsEnterpriseAvailable())

	w := apitest.PerformAuthRequest(r, "GET", "/api/v1/tyk-mcp/status", nil, key)
	require.Equal(t, http.StatusOK, w.Code)
	var status tykmcp.Status
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	assert.False(t, status.Available)
	assert.False(t, status.Enabled)

	w = apitest.PerformAuthRequest(r, "GET", "/common/system", nil, key)
	require.Equal(t, http.StatusOK, w.Code)
	var sys struct {
		Features map[string]interface{} `json:"features"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sys))
	assert.Equal(t, false, sys.Features["feature_tyk_mcp"])

	body := map[string]interface{}{"name": "d", "dashboard_url": "https://dash.example.com", "dashboard_access_token": "t"}
	for _, rq := range []struct{ method, path string }{
		{"GET", "/api/v1/tyk-connections"},
		{"POST", "/api/v1/tyk-connections"},
		{"POST", "/api/v1/tyk-connections/probe"},
		{"GET", "/api/v1/tyk-connections/1"},
		{"PATCH", "/api/v1/tyk-connections/1"},
		{"DELETE", "/api/v1/tyk-connections/1"},
		{"POST", "/api/v1/tyk-connections/1/activate"},
		{"POST", "/api/v1/tyk-connections/1/disable"},
		{"POST", "/api/v1/tyk-connections/1/probe"},
		{"POST", "/api/v1/tyk-connections/1/sync"},
		{"GET", "/api/v1/tyk-connections/1/policies"},
		{"GET", "/api/v1/tyk-connections/1/sync-runs"},
		{"GET", "/api/v1/tyk-connections/1/nodes"},
		{"GET", "/api/v1/tyk-connections/1/ingest-status"},
		{"POST", "/api/v1/tyk-connections/1/ingest-token"},
		{"DELETE", "/api/v1/tyk-connections/1/ingest-token"},
		{"GET", "/api/v1/mcp-servers"},
		{"GET", "/api/v1/mcp-servers/1"},
		{"PATCH", "/api/v1/mcp-servers/1"},
		{"DELETE", "/api/v1/mcp-servers/1"},
		{"POST", "/api/v1/mcp-servers/1/activate"},
		{"POST", "/api/v1/mcp-servers/1/deactivate"},
		{"PUT", "/api/v1/mcp-servers/1/catalogues"},
		{"PUT", "/api/v1/mcp-servers/1/bundle"},
		{"PUT", "/api/v1/mcp-servers/1/key-access"},
		{"POST", "/api/v1/mcp-servers/register"},
		{"POST", "/api/v1/mcp-servers/discover-tools"},
		{"POST", "/api/v1/mcp-servers/1/push"},
		{"GET", "/api/v1/mcp-servers/1/handoff"},
		{"POST", "/api/v1/mcp-servers/1/link"},
		{"GET", "/api/v1/tyk-connections/1/apis"},
		{"GET", "/api/v1/tyk-connections/1/apis/x/operations"},
		{"GET", "/api/v1/tyk-connections/1/gateway-tags"},
		{"POST", "/api/v1/tyk-connections/1/policies"},
		{"PATCH", "/api/v1/tyk-connections/1/policies/x"},
		{"GET", "/api/v1/mcp-credentials"},
		{"POST", "/api/v1/mcp-credentials"},
		{"GET", "/api/v1/mcp-credentials/x"},
		{"POST", "/api/v1/mcp-credentials/x/rotate"},
		{"POST", "/api/v1/mcp-credentials/x/suspend"},
		{"POST", "/api/v1/mcp-credentials/x/resume"},
		{"POST", "/api/v1/mcp-credentials/x/revoke"},
		{"POST", "/api/v1/mcp-credentials/x/apply-drift"},
		{"GET", "/api/v1/mcp-access-report"},
		// The Tools import reads Dashboard APIs through the same service.
		{"GET", "/api/v1/tools/import/tyk/connections"},
		{"GET", "/api/v1/tools/import/tyk/connections/1/apis"},
		{"GET", "/api/v1/tools/import/tyk/connections/1/apis/x"},
	} {
		w := apitest.PerformAuthRequest(r, rq.method, rq.path, body, key)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s %s", rq.method, rq.path)
	}

	// The analytics ingest endpoint does not exist in CE, with or without a
	// token (and never answers 401, which would suggest it does).
	for _, auth := range []string{"", "Bearer tai_ing_anything"} {
		req := httptest.NewRequest(http.MethodPost, tykmcp.IngestPath, strings.NewReader(`[]`))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code, "ingest with auth %q", auth)
	}

	// A malformed connection id is refused before the service is asked.
	for _, path := range []string{"/api/v1/tools/import/tyk/connections/abc/apis", "/api/v1/tools/import/tyk/connections/0/apis/x"} {
		w := apitest.PerformAuthRequest(r, "GET", path, nil, key)
		assert.Equal(t, http.StatusBadRequest, w.Code, path)
	}
}

//go:build enterprise
// +build enterprise

package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keyedFakeDashboard extends the rich fake with a key store so the broker
// can mint, read, update and delete keys.
type keyedFakeDashboard struct {
	srv  *httptest.Server
	mu   sync.Mutex
	keys map[string]map[string]interface{} // by hash
	seq  int
}

func newKeyedFakeDashboard(t *testing.T, token string, mcps, policies []string) *keyedFakeDashboard {
	t.Helper()
	f := &keyedFakeDashboard{keys: map[string]map[string]interface{}{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != token {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"Status":"Error","Message":"Not authorised"}`))
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		enc := json.NewEncoder(w)
		path := r.URL.Path
		switch {
		case r.Method == "GET" && path == "/api/schemas/apidefs/mcp":
			_, _ = w.Write([]byte(`{"definitions":{"x-tyk-mcp-server":{}}}`))
		case r.Method == "GET" && path == "/api/mcps":
			_, _ = w.Write([]byte(`{"mcps":[` + strings.Join(mcps, ",") + `],"pages":1}`))
		case r.Method == "GET" && path == "/api/portal/policies":
			_, _ = w.Write([]byte(`{"Data":[` + strings.Join(policies, ",") + `],"Pages":1}`))
		case r.Method == "GET" && path == "/api/apis":
			_, _ = w.Write([]byte(`{"apis":[],"pages":1}`))
		case r.Method == "POST" && path == "/api/keys/preview":
			_, _ = w.Write([]byte(`{"key_id":"","data":{"org_id":"org-1"}}`))
		case r.Method == "POST" && path == "/api/mcps":
			_, _ = w.Write([]byte(`{"openapi":"3.0.3"}`))
		case r.Method == "POST" && path == "/api/keys":
			var sess map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&sess)
			f.seq++
			id := fmt.Sprintf("org1plaintextkey%d", f.seq)
			sum := sha256.Sum256([]byte(id))
			hash := hex.EncodeToString(sum[:])[:32]
			f.keys[hash] = sess
			_ = enc.Encode(map[string]interface{}{"key_id": id, "key_hash": hash, "data": sess})
		case strings.HasPrefix(path, "/api/keys/"):
			ref := strings.TrimPrefix(path, "/api/keys/")
			sess, ok := f.keys[ref]
			if !ok {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"Status":"Error","Message":"Could not retrieve key detail"}`))
				return
			}
			switch r.Method {
			case "GET":
				_ = enc.Encode(map[string]interface{}{"key_id": "", "key_hash": ref, "data": sess})
			case "PUT":
				var next map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&next)
				f.keys[ref] = next
				_, _ = w.Write([]byte(`{"Status":"OK","Message":"Key updated"}`))
			case "DELETE":
				delete(f.keys, ref)
				_, _ = w.Write([]byte(`{"Status":"OK","Message":"Key deleted"}`))
			}
		default:
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"Status":"Error","Message":"Not found"}`))
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func TestTykMCPEnterprise_CredentialFlow(t *testing.T) {
	h := setupTykMCPEnterpriseAPI(t)
	dash := newKeyedFakeDashboard(t, "user-key-abcd", []string{weatherMCP}, []string{weatherACL, goldPlan})
	r := h.api.router
	adminKey := h.admin.APIKey
	db := h.api.service.DB

	// Connection (broker), sync, bundle, publish, grant.
	input := map[string]interface{}{"name": "Prod", "dashboard_url": dash.srv.URL, "dashboard_access_token": "user-key-abcd", "declared_mode": "broker", "allow_internal_host": true, "gateway_base_url": "https://gw.example.com"}
	w := apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections", input, adminKey)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var conn models.TykConnectionResponse
	decodeWebhookJSON(t, w, &conn)
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(conn.ID)+"/activate", nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(conn.ID)+"/sync?wait=true", nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-servers", nil, adminKey)
	var list tykmcp.ServerList
	decodeWebhookJSON(t, w, &list)
	require.Len(t, list.Servers, 1)
	srv := list.Servers[0]
	w = apitest.PerformAuthRequest(r, "PUT", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/bundle", map[string]interface{}{"pins": []map[string]string{{"tyk_policy_id": "pol-acl", "role": "access"}, {"tyk_policy_id": "pol-gold", "role": "consumption"}}}, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "PATCH", "/api/v1/mcp-servers/"+tykIDStr(srv.ID), map[string]interface{}{"privacy_score": 20, "lock_version": srv.LockVersion}, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/activate", nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	team := &models.Group{Name: "AI team"}
	require.NoError(t, db.Create(team).Error)
	w = apitest.PerformAuthRequest(r, "PUT", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/groups", map[string]interface{}{"group_ids": []uint{team.ID}}, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Portal user builds an App with the server.
	member := models.NewUser()
	member.Email, member.Name, member.Password, member.EmailVerified, member.ShowPortal = "member@tyk.io", "Member", "hash", true, true
	require.NoError(t, member.Create(db))
	require.NoError(t, db.Model(team).Association("Users").Append(member))
	other := models.NewUser()
	other.Email, other.Name, other.Password, other.EmailVerified, other.ShowPortal = "other@tyk.io", "Other", "hash", true, true
	require.NoError(t, other.Create(db))
	appReq := map[string]interface{}{"name": "Weather app", "description": "d", "data_source_ids": []uint{}, "llm_ids": []uint{}, "tool_ids": []uint{}, "mcp_server_ids": []uint{srv.ID}}
	w = apitest.PerformAuthRequest(r, "POST", "/common/apps", appReq, member.APIKey)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var app AppResponse
	decodeWebhookJSON(t, w, &app)

	// Before approval: cannot mint.
	w = apitest.PerformAuthRequest(r, "GET", "/common/apps/"+app.ID+"/mcp", nil, member.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var summary tykmcp.AppMCPSummary
	decodeWebhookJSON(t, w, &summary)
	require.Len(t, summary.Connections, 1)
	assert.False(t, summary.Connections[0].CanMint)
	assert.Contains(t, summary.Connections[0].Reason, "approved")
	w = apitest.PerformAuthRequest(r, "POST", "/common/apps/"+app.ID+"/mcp/credentials", map[string]interface{}{"connection_id": conn.ID}, member.APIKey)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	// Approve, then mint as the owner: key shown once, never stored.
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/apps/"+app.ID+"/activate-credential", nil, adminKey)
	require.Contains(t, []int{http.StatusOK, http.StatusNoContent}, w.Code)
	w = apitest.PerformAuthRequest(r, "POST", "/common/apps/"+app.ID+"/mcp/credentials", map[string]interface{}{"connection_id": conn.ID}, member.APIKey)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var minted tykmcp.MintedCredential
	decodeWebhookJSON(t, w, &minted)
	assert.Contains(t, minted.Key, "plaintextkey")
	assert.Equal(t, "active", minted.Credential.Status)
	assert.Equal(t, []string{"pol-acl", "pol-gold"}, minted.Credential.AppliedPolicyIDs)
	require.Len(t, minted.Servers, 1)
	assert.Equal(t, "https://gw.example.com/weather/mcp", minted.Servers[0].EndpointURL)

	// No later response carries the key.
	for _, path := range []string{"/common/apps/" + app.ID + "/mcp", "/api/v1/mcp-credentials", "/api/v1/mcp-credentials/" + minted.Credential.ID, "/api/v1/mcp-access-report"} {
		key := member.APIKey
		if strings.HasPrefix(path, "/api/v1") {
			key = adminKey
		}
		w = apitest.PerformAuthRequest(r, "GET", path, nil, key)
		require.Equal(t, http.StatusOK, w.Code, path+": "+w.Body.String())
		assert.NotContains(t, w.Body.String(), minted.Key, path)
	}
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-access-report", nil, adminKey)
	var report []tykmcp.AccessReportRow
	decodeWebhookJSON(t, w, &report)
	require.Len(t, report, 1)
	assert.Equal(t, "member@tyk.io", report[0].UserEmail)
	assert.Equal(t, minted.Credential.ID, report[0].CredentialID)

	// A second mint is refused; the summary explains why.
	w = apitest.PerformAuthRequest(r, "POST", "/common/apps/"+app.ID+"/mcp/credentials", map[string]interface{}{"connection_id": conn.ID}, member.APIKey)
	assert.Equal(t, http.StatusConflict, w.Code)
	w = apitest.PerformAuthRequest(r, "GET", "/common/apps/"+app.ID+"/mcp", nil, member.APIKey)
	decodeWebhookJSON(t, w, &summary)
	assert.False(t, summary.Connections[0].CanMint)
	assert.Equal(t, minted.Credential.ID, summary.Connections[0].CredentialID)
	require.Len(t, summary.Credentials, 1)

	// Another user can neither see nor rotate it.
	w = apitest.PerformAuthRequest(r, "POST", "/common/apps/"+app.ID+"/mcp/credentials/"+minted.Credential.ID+"/rotate", nil, other.APIKey)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Owner rotates: a new key, the old one revoked.
	w = apitest.PerformAuthRequest(r, "POST", "/common/apps/"+app.ID+"/mcp/credentials/"+minted.Credential.ID+"/rotate", nil, member.APIKey)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var rotated tykmcp.MintedCredential
	decodeWebhookJSON(t, w, &rotated)
	assert.NotEqual(t, minted.Key, rotated.Key)
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-credentials/"+minted.Credential.ID, nil, adminKey)
	var old models.MCPCredentialResponse
	decodeWebhookJSON(t, w, &old)
	assert.Equal(t, "revoked", old.Status)

	// Admin suspends and resumes; owner revokes.
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/mcp-credentials/"+rotated.Credential.ID+"/suspend", map[string]string{"reason": "audit"}, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/mcp-credentials/"+rotated.Credential.ID+"/resume", nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "POST", "/common/apps/"+app.ID+"/mcp/credentials/"+rotated.Credential.ID+"/revoke", nil, member.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var revoked models.MCPCredentialResponse
	decodeWebhookJSON(t, w, &revoked)
	assert.Equal(t, "revoked", revoked.Status)
	dash.mu.Lock()
	assert.Empty(t, dash.keys, "every key removed from the Dashboard")
	dash.mu.Unlock()

	// The audit trail never carries a key.
	require.Eventually(t, func() bool {
		w = apitest.PerformAuthRequest(r, "GET", "/api/v1/audit/records?resource_type=mcp_credential", nil, adminKey)
		return w.Code == http.StatusOK && strings.Contains(w.Body.String(), "mcp_credential")
	}, 5e9, 1e8)
	assert.NotContains(t, w.Body.String(), minted.Key)
	assert.NotContains(t, w.Body.String(), rotated.Key)
}

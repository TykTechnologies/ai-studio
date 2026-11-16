//go:build enterprise
// +build enterprise

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Workspaces B4b: filestores are read-global, write-scoped objects.

// uploadFileStore POSTs a multipart filestore create, with workspace_id when
// ws is not nil.
func (f *wsB4Fixture) uploadFileStore(t *testing.T, name string, ws *uint, as *models.User) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", name)
	require.NoError(t, err)
	_, err = part.Write([]byte("hello workspaces"))
	require.NoError(t, err)
	if ws != nil {
		require.NoError(t, mw.WriteField("workspace_id", fmt.Sprint(*ws)))
	}
	require.NoError(t, mw.Close())
	req := httptest.NewRequest("POST", "/api/v1/filestore", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+as.APIKey)
	w := httptest.NewRecorder()
	f.api.router.ServeHTTP(w, req)
	return w
}

func TestWorkspaceB4b_FileStores(t *testing.T) {
	f := setupWSB4(t)

	// Create lands by the landing rule; a Workspace without write there
	// answers as an unknown one does.
	mine, ws := f.created(t, f.uploadFileStore(t, "mine.txt", nil, f.inW1))
	assert.Equal(t, f.w1, ws, "a W1-only writer lands in W1")
	assertCode(t, f.uploadFileStore(t, "theirs.txt", &f.w2, f.inW1), http.StatusUnprocessableEntity, authz.CodeInvalidWorkspace)
	theirs, ws := f.created(t, f.uploadFileStore(t, "theirs.txt", &f.w2, f.owner))
	assert.Equal(t, f.w2, ws)
	bad := uint(9999)
	assertCode(t, f.uploadFileStore(t, "nowhere.txt", &bad, f.owner), http.StatusUnprocessableEntity, authz.CodeInvalidWorkspace)

	// Read-global: a W1 writer reads and lists W2's entry.
	assert.Equal(t, http.StatusOK, f.do("GET", fmt.Sprintf("/api/v1/filestore/%d", theirs), nil, f.inW1).Code)

	// Update and delete need the permission in the entry's Workspace.
	patch := func(attrs map[string]any) map[string]any { return body(attrs) }
	assertCode(t, f.do("PATCH", fmt.Sprintf("/api/v1/filestore/%d", theirs), patch(map[string]any{"description": "x"}), f.inW1),
		http.StatusForbidden, authz.CodePermissionDenied)
	assertCode(t, f.do("DELETE", fmt.Sprintf("/api/v1/filestore/%d", theirs), nil, f.inW1), http.StatusForbidden, authz.CodePermissionDenied)
	assert.Equal(t, http.StatusOK, f.do("PATCH", fmt.Sprintf("/api/v1/filestore/%d", mine), patch(map[string]any{"description": "mine"}), f.inW1).Code)

	// A move needs write in both Workspaces and stains (updated_at moves).
	assertCode(t, f.do("PATCH", fmt.Sprintf("/api/v1/filestore/%d", mine), patch(map[string]any{"workspace_id": f.w2}), f.inW1),
		http.StatusUnprocessableEntity, authz.CodeInvalidWorkspace)
	_, before := f.stored(t, &models.FileStore{}, mine)
	w := f.do("PATCH", fmt.Sprintf("/api/v1/filestore/%d", mine), patch(map[string]any{"workspace_id": f.w2}), f.inW1W2)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	ws, after := f.stored(t, &models.FileStore{}, mine)
	assert.Equal(t, f.w2, ws)
	assert.True(t, after.After(before), "the move advanced updated_at")

	// ?workspace_id= filters the list and the search.
	list := func(path string) []string {
		w := f.do("GET", path, nil, f.inW1)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Data []struct {
				Attributes struct {
					FileName string `json:"file_name"`
				}
			}
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		var names []string
		for _, d := range resp.Data {
			names = append(names, d.Attributes.FileName)
		}
		return names
	}
	assert.ElementsMatch(t, []string{"mine.txt", "theirs.txt"}, list("/api/v1/filestore?all=true"))
	assert.ElementsMatch(t, []string{"mine.txt", "theirs.txt"}, list(fmt.Sprintf("/api/v1/filestore?all=true&workspace_id=%d", f.w2)))
	assert.Empty(t, list(fmt.Sprintf("/api/v1/filestore?all=true&workspace_id=%d", f.w1)))
	assert.ElementsMatch(t, []string{"theirs.txt"}, list(fmt.Sprintf("/api/v1/filestore/search?query=theirs&workspace_id=%d", f.w2)))
	assert.Empty(t, list(fmt.Sprintf("/api/v1/filestore/search?query=theirs&workspace_id=%d", f.w1)))

	assert.Equal(t, http.StatusNoContent, f.do("DELETE", fmt.Sprintf("/api/v1/filestore/%d", theirs), nil, f.owner).Code)
}

// Plugins and webhooks are platform-wide powers (ScopeGlobal, .36): a role
// bound in a Workspace grants none of them, whatever its permissions.
func TestWorkspaceB4b_PluginsAndWebhooksAreGlobalOnly(t *testing.T) {
	f := setupWSB4(t)
	db := f.api.service.DB
	plugin := &models.Plugin{Name: "Guard", Command: "file:///bin/true", HookType: "post_auth", IsActive: true}
	require.NoError(t, db.Create(plugin).Error)

	for _, r := range []struct{ method, path string }{
		{"POST", "/api/v1/plugins"},
		{"POST", fmt.Sprintf("/api/v1/plugins/%d/approve-scopes", plugin.ID)},
		{"POST", fmt.Sprintf("/api/v1/plugins/%d/enable", plugin.ID)},
		{"POST", fmt.Sprintf("/api/v1/plugins/%d/disable", plugin.ID)},
		{"POST", fmt.Sprintf("/api/v1/plugins/%d/rpc/anything", plugin.ID)}, // plugin-contributed admin route
		{"POST", "/api/v1/webhooks/targets"},
		{"POST", "/api/v1/webhooks/targets/1/approve"},
		{"GET", "/api/v1/webhooks/deliveries"},
	} {
		w := f.do(r.method, r.path, map[string]any{}, f.inW1)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s %s: %s", r.method, r.path, w.Body.String())
	}
	// A global holder is unaffected.
	assert.NotEqual(t, http.StatusForbidden, f.do("POST", fmt.Sprintf("/api/v1/plugins/%d/disable", plugin.ID), nil, f.owner).Code)
}

// MCP servers are read-global, write-scoped objects; ones the Dashboard sync
// catalogues (no acting user) land in the Default, never at 0.
func TestWorkspaceB4b_MCPServers(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "workspace-b4b-test-key")
	f := setupWSB4(t)
	db := f.api.service.DB
	f.api.service.InitTykMCP(config.TykMCPConfig{Enabled: true, SyncMinInterval: 10 * time.Second, RequestTimeout: 2 * time.Second, RateLimitPerSecond: 100}, "test")
	t.Cleanup(f.api.service.TykMCP.Stop)

	synced := &models.MCPServer{Name: "Synced", Slug: "synced", Origin: models.MCPOriginDashboard}
	require.NoError(t, db.Create(synced).Error)
	assert.Equal(t, f.def, synced.WorkspaceID, "stamped with the Default at create")
	mine := &models.MCPServer{Name: "Mine", Slug: "mine", Origin: models.MCPOriginDashboard, WorkspaceID: f.w1}
	theirs := &models.MCPServer{Name: "Theirs", Slug: "theirs", Origin: models.MCPOriginDashboard, WorkspaceID: f.w2}
	require.NoError(t, db.Create(mine).Error)
	require.NoError(t, db.Create(theirs).Error)

	// Read-global, with the ?workspace_id= filter.
	names := func(path string) []string {
		w := f.do("GET", path, nil, f.inW1)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var list tykmcp.ServerList
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
		var out []string
		for _, s := range list.Servers {
			out = append(out, s.Name)
		}
		return out
	}
	assert.ElementsMatch(t, []string{"Synced", "Mine", "Theirs"}, names("/api/v1/mcp-servers"))
	assert.ElementsMatch(t, []string{"Theirs"}, names(fmt.Sprintf("/api/v1/mcp-servers?workspace_id=%d", f.w2)))
	assert.ElementsMatch(t, []string{"Synced"}, names(fmt.Sprintf("/api/v1/mcp-servers?workspace_id=%d", f.def)))
	assert.Equal(t, http.StatusOK, f.do("GET", fmt.Sprintf("/api/v1/mcp-servers/%d", theirs.ID), nil, f.inW1).Code)

	// Write, delete, publish and execute need the permission in the server's Workspace.
	for _, r := range []struct {
		method, path string
		body         any
	}{
		{"PATCH", fmt.Sprintf("/api/v1/mcp-servers/%d", theirs.ID), map[string]any{"description": "x"}},
		{"DELETE", fmt.Sprintf("/api/v1/mcp-servers/%d", theirs.ID), nil},
		{"POST", fmt.Sprintf("/api/v1/mcp-servers/%d/activate", theirs.ID), nil},
		{"POST", fmt.Sprintf("/api/v1/mcp-servers/%d/deactivate", theirs.ID), nil},
		{"PUT", fmt.Sprintf("/api/v1/mcp-servers/%d/catalogues", theirs.ID), map[string]any{"tool_catalogue_ids": []uint{}}},
		{"PUT", fmt.Sprintf("/api/v1/mcp-servers/%d/bundle", theirs.ID), map[string]any{}},
		{"PUT", fmt.Sprintf("/api/v1/mcp-servers/%d/key-access", theirs.ID), map[string]any{}},
		{"POST", fmt.Sprintf("/api/v1/mcp-servers/%d/push", theirs.ID), map[string]any{"definition": map[string]any{}}},
		{"POST", fmt.Sprintf("/api/v1/mcp-servers/%d/link", theirs.ID), map[string]any{"tyk_api_id": "x"}},
	} {
		assertCode(t, f.do(r.method, r.path, r.body, f.inW1), http.StatusForbidden, authz.CodePermissionDenied)
	}

	// A move needs write in both Workspaces and advances updated_at.
	_, before := f.stored(t, &models.MCPServer{}, mine.ID)
	w := f.do("PATCH", fmt.Sprintf("/api/v1/mcp-servers/%d", mine.ID), map[string]any{"workspace_id": f.w2, "lock_version": 0}, f.inW1W2)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var view models.MCPServerResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &view))
	assert.Equal(t, f.w2, view.WorkspaceID)
	ws, after := f.stored(t, &models.MCPServer{}, mine.ID)
	assert.Equal(t, f.w2, ws)
	assert.True(t, after.After(before), "the move advanced updated_at")
	assertCode(t, f.do("PATCH", fmt.Sprintf("/api/v1/mcp-servers/%d", mine.ID), map[string]any{"workspace_id": f.w1, "lock_version": view.LockVersion}, f.inW1),
		http.StatusForbidden, authz.CodePermissionDenied)
}

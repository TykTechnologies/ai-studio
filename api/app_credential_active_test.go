package api

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The apps list and the single-app response carry credential_active so the
// UI can show the key state without fetching every credential: true/false
// from the app's credential, null when the app has none.
func TestApps_CredentialActive(t *testing.T) {
	api, db := setupTestAPI(t)

	live := &models.Credential{KeyID: "key-live", Secret: "s1", Active: true}
	require.NoError(t, db.Create(live).Error)
	revoked := &models.Credential{KeyID: "key-revoked", Secret: "s2", Active: false}
	require.NoError(t, db.Create(revoked).Error)

	withLive := &models.App{Name: "With live key", UserID: 1, CredentialID: live.ID}
	require.NoError(t, db.Create(withLive).Error)
	withRevoked := &models.App{Name: "With revoked key", UserID: 1, CredentialID: revoked.ID}
	require.NoError(t, db.Create(withRevoked).Error)
	without := &models.App{Name: "No key", UserID: 1}
	require.NoError(t, db.Create(without).Error)

	type attrs struct {
		Name             string `json:"name"`
		CredentialActive *bool  `json:"credential_active"`
	}
	byName := func(t *testing.T, body []byte) map[string]json.RawMessage {
		t.Helper()
		var resp struct {
			Data []struct {
				Attributes json.RawMessage `json:"attributes"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &resp))
		out := map[string]json.RawMessage{}
		for _, d := range resp.Data {
			var a attrs
			require.NoError(t, json.Unmarshal(d.Attributes, &a))
			out[a.Name] = d.Attributes
		}
		return out
	}
	credentialActive := func(t *testing.T, raw json.RawMessage) *bool {
		t.Helper()
		var a attrs
		require.NoError(t, json.Unmarshal(raw, &a))
		return a.CredentialActive
	}

	w := performRequest(api.router, "GET", "/api/v1/apps?all=true", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	rows := byName(t, w.Body.Bytes())
	require.Len(t, rows, 3)
	require.NotNil(t, credentialActive(t, rows["With live key"]))
	assert.True(t, *credentialActive(t, rows["With live key"]))
	require.NotNil(t, credentialActive(t, rows["With revoked key"]))
	assert.False(t, *credentialActive(t, rows["With revoked key"]))
	assert.Nil(t, credentialActive(t, rows["No key"]))
	// The key is present as an explicit null, not omitted.
	assert.Contains(t, string(rows["No key"]), `"credential_active":null`)

	// Search goes through a different query; same field.
	w = performRequest(api.router, "GET", "/api/v1/apps?search=revoked", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	rows = byName(t, w.Body.Bytes())
	require.Len(t, rows, 1)
	assert.False(t, *credentialActive(t, rows["With revoked key"]))

	// Single app.
	single := func(t *testing.T, id uint) *bool {
		t.Helper()
		w := performRequest(api.router, "GET", "/api/v1/apps/"+idStr(id), nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Data struct {
				Attributes json.RawMessage `json:"attributes"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return credentialActive(t, resp.Data.Attributes)
	}
	require.NotNil(t, single(t, withLive.ID))
	assert.True(t, *single(t, withLive.ID))
	assert.False(t, *single(t, withRevoked.ID))
	assert.Nil(t, single(t, without.ID))

	// Flipping the credential shows on the next read.
	w = performRequest(api.router, "POST", "/api/v1/apps/"+idStr(withLive.ID)+"/deactivate-credential", nil)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.False(t, *single(t, withLive.ID))
}

// Listing apps runs a fixed number of queries however many rows the page
// has: the credential comes from one preload, not a lookup per app.
func TestApps_ListCredentialIsNotNPlusOne(t *testing.T) {
	api, db := setupTestAPI(t)

	var queries int64
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:count_queries", func(*gorm.DB) {
		atomic.AddInt64(&queries, 1)
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove("test:count_queries") })

	addApps := func(n int) {
		for i := 0; i < n; i++ {
			cred := &models.Credential{KeyID: "k-" + idStr(uint(i)) + "-" + idStr(uint(n)), Secret: "s", Active: i%2 == 0}
			require.NoError(t, db.Create(cred).Error)
			require.NoError(t, db.Create(&models.App{Name: "App", UserID: 1, CredentialID: cred.ID}).Error)
		}
	}
	count := func() int64 {
		atomic.StoreInt64(&queries, 0)
		w := performRequest(api.router, "GET", "/api/v1/apps?all=true", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return atomic.LoadInt64(&queries)
	}

	addApps(2)
	small := count()
	addApps(6)
	large := count()
	assert.Equal(t, small, large, "query count must not grow with the number of apps (%d rows: %d queries, %d rows: %d queries)", 2, small, 8, large)
}

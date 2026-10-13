package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
)

func decodeSigningStatus(t *testing.T, body []byte) ResourceSigningStatus {
	t.Helper()
	var resp ResourceSigningStatusResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp.Data
}

func TestLLMResourceSigningKeyEndpoints(t *testing.T) {
	api, db := setupTestAPI(t)
	llm := &models.LLM{Name: "Claude", Vendor: models.ANTHROPIC, APIKey: "vendor-key", Active: true}
	require.NoError(t, db.Create(llm).Error)
	path := "/api/v1/llms/" + idStr(llm.ID) + "/resource-signing-key"

	w := performRequest(api.router, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), llm.ResourceSigningKeys().Secret, "the secret is never returned")
	st := decodeSigningStatus(t, w.Body.Bytes())
	assert.Equal(t, llm.ID, st.LLMID)
	assert.Equal(t, "secret", st.Source)
	want, err := nativeapi.SecretKey(llm.ResourceSigningKeys().Secret)
	require.NoError(t, err)
	assert.Equal(t, want.KID(), st.KID)
	assert.NotNil(t, st.RotatedAt)
	assert.Nil(t, st.Previous)
	firstKID := st.KID

	// Default grace: 30 days.
	w = performRequest(api.router, http.MethodPost, path+"/rotate", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	st = decodeSigningStatus(t, w.Body.Bytes())
	assert.NotEqual(t, firstKID, st.KID)
	require.NotNil(t, st.Previous)
	assert.Equal(t, firstKID, st.Previous.KID)
	assert.Equal(t, "secret", st.Previous.Source)
	assert.True(t, st.Previous.Active)
	assert.WithinDuration(t, time.Now().Add(models.DefaultResourceSigningGrace), st.Previous.ExpiresAt, time.Minute)
	secondKID := st.KID

	w = performRequest(api.router, http.MethodPost, path+"/rotate", map[string]int{"grace_period_hours": 2})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	st = decodeSigningStatus(t, w.Body.Bytes())
	require.NotNil(t, st.Previous)
	assert.Equal(t, secondKID, st.Previous.KID, "only the key just replaced is kept")
	assert.WithinDuration(t, time.Now().Add(2*time.Hour), st.Previous.ExpiresAt, time.Minute)

	w = performRequest(api.router, http.MethodPost, path+"/rotate", map[string]int{"grace_period_hours": 0})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Nil(t, decodeSigningStatus(t, w.Body.Bytes()).Previous, "no grace revokes the old key")

	for _, bad := range []any{
		map[string]int{"grace_period_hours": -1},
		map[string]int{"grace_period_hours": 8761},
		// Would overflow time.Duration if multiplied before the bound check.
		map[string]int64{"grace_period_hours": 2562048},
		map[string]int64{"grace_period_hours": 1<<51 + 1},
		map[string]float64{"grace_period_hours": 1e30},
		"not json",
	} {
		w = performRequest(api.router, http.MethodPost, path+"/rotate", bad)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	}
	w = performRequest(api.router, http.MethodPost, "/api/v1/llms/9999/resource-signing-key/rotate", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = performRequest(api.router, http.MethodGet, "/api/v1/llms/9999/resource-signing-key", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = performRequest(api.router, http.MethodGet, "/api/v1/llms/abc/resource-signing-key", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// The LLM's own JSON never carries the keys.
	w = performRequest(api.router, http.MethodGet, "/api/v1/llms/"+idStr(llm.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "resource_signing")
}

// An LLM not yet backfilled reports its vendor-credential key; after the
// backfill the vendor-credential key is the previous one, in its grace.
func TestLLMResourceSigningKeyStatusLegacy(t *testing.T) {
	api, db := setupTestAPI(t)
	llm := &models.LLM{Name: "Claude", Vendor: models.ANTHROPIC, APIKey: "vendor-key", Active: true}
	require.NoError(t, db.Create(llm).Error)
	require.NoError(t, db.Model(&models.LLM{}).Where("id = ?", llm.ID).
		UpdateColumns(map[string]any{"resource_signing_secret": "", "resource_signing_rotated_at": nil}).Error)
	path := "/api/v1/llms/" + idStr(llm.ID) + "/resource-signing-key"
	credKID := func() string {
		k, err := nativeapi.CredentialKey("vendor-key")
		require.NoError(t, err)
		return k.KID()
	}()

	w := performRequest(api.router, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	st := decodeSigningStatus(t, w.Body.Bytes())
	assert.Equal(t, "vendor_credential", st.Source)
	assert.Equal(t, credKID, st.KID)
	assert.Nil(t, st.RotatedAt)

	_, err := models.BackfillLLMResourceSigningSecrets(db, models.DefaultResourceSigningGrace, time.Now())
	require.NoError(t, err)
	w = performRequest(api.router, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	st = decodeSigningStatus(t, w.Body.Bytes())
	assert.Equal(t, "secret", st.Source)
	require.NotNil(t, st.Previous)
	assert.Equal(t, "vendor_credential", st.Previous.Source)
	assert.Equal(t, credKID, st.Previous.KID)
	assert.True(t, st.Previous.Active)

	// The stored upgrade-time key is reported, whatever the credential is now.
	require.NoError(t, db.Model(&models.LLM{}).Where("id = ?", llm.ID).Update("api_key", "rotated-key").Error)
	w = performRequest(api.router, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	st = decodeSigningStatus(t, w.Body.Bytes())
	require.NotNil(t, st.Previous)
	assert.Equal(t, credKID, st.Previous.KID, "the kid outstanding pre-upgrade IDs carry")

	// A legacy LLM with no credential at all has no key.
	require.NoError(t, db.Model(&models.LLM{}).Where("id = ?", llm.ID).
		UpdateColumns(map[string]any{"resource_signing_secret": "", "api_key": ""}).Error)
	w = performRequest(api.router, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	st = decodeSigningStatus(t, w.Body.Bytes())
	assert.Equal(t, "none", st.Source)
	assert.Empty(t, st.KID)
}

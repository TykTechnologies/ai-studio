package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// fakeIngestService implements the ingest half of tykmcp.Service; every
// other method panics through the nil embedded interface.
type fakeIngestService struct {
	tykmcp.Service
	mu      sync.Mutex
	tokens  map[string]uint
	batches [][]tykmcp.TykAnalyticsRecord
	conns   []uint
	rotated []uint
}

func (f *fakeIngestService) AuthenticateIngest(_ context.Context, token string) (uint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id, ok := f.tokens[token]; ok && token != "" {
		return id, nil
	}
	return 0, tykmcp.ErrIngestUnauthorized
}

func (f *fakeIngestService) IngestAnalytics(_ context.Context, connectionID uint, recs []tykmcp.TykAnalyticsRecord) (*tykmcp.IngestResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches = append(f.batches, recs)
	f.conns = append(f.conns, connectionID)
	res := &tykmcp.IngestResult{Accepted: len(recs)}
	for _, r := range recs {
		if r.MCPStats.IsMCP {
			res.Stored++
		} else {
			res.SkippedNonMCP++
		}
	}
	return res, nil
}

func (f *fakeIngestService) RotateIngestToken(_ context.Context, _ tykmcp.Actor, id uint) (*tykmcp.RotatedIngestToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rotated = append(f.rotated, id)
	return &tykmcp.RotatedIngestToken{Token: "tai_ing_new", Status: tykmcp.IngestStatus{ConnectionID: id, HasToken: true}}, nil
}

func (f *fakeIngestService) RevokeIngestToken(_ context.Context, _ tykmcp.Actor, id uint) (*tykmcp.IngestStatus, error) {
	return &tykmcp.IngestStatus{ConnectionID: id}, nil
}

func (f *fakeIngestService) GetIngestStatus(_ context.Context, id uint) (*tykmcp.IngestStatus, error) {
	return &tykmcp.IngestStatus{ConnectionID: id, HasToken: true, IngestPath: tykmcp.IngestPath}, nil
}

type ingestHarness struct {
	r        *gin.Engine
	fake     *fakeIngestService
	adminKey string
	userKey  string
}

func setupIngestAPI(t *testing.T) *ingestHarness {
	t.Helper()
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	cfg := apitest.SetupTestAuthConfig(db, service)
	// TestMode off: the real permission guards and CSRF apply (the ingest
	// endpoint must work with CSRF on).
	cfg.TestMode = false
	authService := apitest.SetupTestAuthService(db, service)
	authService.Config.TestMode = false // enforce permissions
	a := NewAPI(service, true, authService, cfg, nil, emptyFile, nil)
	fake := &fakeIngestService{tokens: map[string]uint{"tai_ing_good": 7}}
	service.TykMCP = fake

	admin := models.NewUser()
	admin.Email = "admin@tyk.io"
	admin.Name = "Admin"
	admin.Password = "hash"
	admin.IsAdmin = true
	admin.EmailVerified = true
	require.NoError(t, admin.Create(db))

	user := models.NewUser()
	user.Email = "dev@tyk.io"
	user.Name = "Dev"
	user.Password = "hash"
	user.EmailVerified = true
	require.NoError(t, user.Create(db))

	return &ingestHarness{r: a.router, fake: fake, adminKey: admin.APIKey, userKey: user.APIKey}
}

func (h *ingestHarness) ingest(t *testing.T, auth string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, tykmcp.IngestPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	return w
}

func gzipped(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(b)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func pumpFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../services/tykmcp/testdata/pump_records.json")
	require.NoError(t, err)
	return b
}

func TestIngestTykAnalytics_Auth(t *testing.T) {
	h := setupIngestAPI(t)
	body := []byte(`[]`)
	for name, auth := range map[string]string{
		"none":        "",
		"wrong token": "Bearer tai_ing_bad",
		"not bearer":  "Basic dGFpX2luZ19nb29kOg==",
		"empty":       "Bearer ",
	} {
		w := h.ingest(t, auth, body, nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code, name)
		assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Bearer", name)
	}
	assert.Empty(t, h.fake.batches, "no batch reaches the service without a valid token")

	// Revoking (the service no longer knows the token) gets 401 too.
	h.fake.mu.Lock()
	delete(h.fake.tokens, "tai_ing_good")
	h.fake.mu.Unlock()
	w := h.ingest(t, "Bearer tai_ing_good", body, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestIngestTykAnalytics_Accepts(t *testing.T) {
	h := setupIngestAPI(t)
	fixture := pumpFixture(t)

	w := h.ingest(t, "Bearer tai_ing_good", fixture, nil)
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	var res tykmcp.IngestResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, tykmcp.IngestResult{Accepted: 2, Stored: 1, SkippedNonMCP: 1}, res)

	w = h.ingest(t, "bearer tai_ing_good", gzipped(t, fixture), map[string]string{"Content-Encoding": "gzip"})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())

	require.Len(t, h.fake.batches, 2)
	assert.Equal(t, []uint{7, 7}, h.fake.conns, "the connection comes from the token")
	got := h.fake.batches[1][0]
	assert.Equal(t, "get_forecast", got.MCPStats.PrimitiveName)
	assert.Equal(t, int64(37), got.Latency.Upstream)

	// An empty batch is fine.
	w = h.ingest(t, "Bearer tai_ing_good", []byte(` [ ] `), nil)
	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestIngestTykAnalytics_BadBodies(t *testing.T) {
	h := setupIngestAPI(t)
	for name, tc := range map[string]struct {
		body    []byte
		headers map[string]string
		status  int
	}{
		"not json":       {[]byte(`nope`), nil, http.StatusBadRequest},
		"object":         {[]byte(`{"api_id":"x"}`), nil, http.StatusBadRequest},
		"truncated":      {[]byte(`[{"api_id":"x"},`), nil, http.StatusBadRequest},
		"wrong type":     {[]byte(`[{"response_code":"x"}]`), nil, http.StatusBadRequest},
		"bad gzip":       {[]byte(`[]`), map[string]string{"Content-Encoding": "gzip"}, http.StatusBadRequest},
		"other encoding": {[]byte(`[]`), map[string]string{"Content-Encoding": "br"}, http.StatusUnsupportedMediaType},
	} {
		w := h.ingest(t, "Bearer tai_ing_good", tc.body, tc.headers)
		assert.Equal(t, tc.status, w.Code, name)
	}
	assert.Empty(t, h.fake.batches)
}

func TestIngestTykAnalytics_Limits(t *testing.T) {
	oldBytes, oldRecords := ingestMaxBodyBytes, ingestMaxRecords
	ingestMaxBodyBytes, ingestMaxRecords = 4096, 3
	t.Cleanup(func() { ingestMaxBodyBytes, ingestMaxRecords = oldBytes, oldRecords })
	h := setupIngestAPI(t)

	big := []byte(`[{"api_id":"` + strings.Repeat("a", 8000) + `"}]`)
	w := h.ingest(t, "Bearer tai_ing_good", big, nil)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code, "raw body over the cap")

	// Small on the wire, over the cap once expanded.
	bomb := gzipped(t, []byte(`[{"api_id":"`+strings.Repeat("a", 1<<20)+`"}]`))
	require.Less(t, len(bomb), 4096)
	w = h.ingest(t, "Bearer tai_ing_good", bomb, map[string]string{"Content-Encoding": "gzip"})
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code, "gzip expansion over the cap")

	w = h.ingest(t, "Bearer tai_ing_good", []byte(`[{},{},{},{}]`), nil)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code, "too many records")

	w = h.ingest(t, "Bearer tai_ing_good", []byte(`[{},{},{}]`), nil)
	assert.Equal(t, http.StatusAccepted, w.Code, "at the record cap")
	require.Len(t, h.fake.batches, 1)
}

func TestIngestTykAnalytics_RateLimit(t *testing.T) {
	oldRate, oldBurst := ingestRate, ingestBurst
	ingestRate, ingestBurst = rate.Limit(0.001), 2
	t.Cleanup(func() { ingestRate, ingestBurst = oldRate, oldBurst })
	h := setupIngestAPI(t)
	h.fake.tokens["tai_ing_other"] = 8

	for i := 0; i < 2; i++ {
		assert.Equal(t, http.StatusAccepted, h.ingest(t, "Bearer tai_ing_good", []byte(`[]`), nil).Code)
	}
	w := h.ingest(t, "Bearer tai_ing_good", []byte(`[]`), nil)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.NotEmpty(t, w.Header().Get("Retry-After"))

	// The limit is per connection: another token is unaffected.
	assert.Equal(t, http.StatusAccepted, h.ingest(t, "Bearer tai_ing_other", []byte(`[]`), nil).Code)
}

func TestIngestToken_AdminRoutes(t *testing.T) {
	h := setupIngestAPI(t)

	w := apitest.PerformAuthRequest(h.r, "POST", "/api/v1/tyk-connections/3/ingest-token", map[string]interface{}{}, h.adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var rot tykmcp.RotatedIngestToken
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rot))
	assert.Equal(t, "tai_ing_new", rot.Token)
	assert.Equal(t, []uint{3}, h.fake.rotated)

	w = apitest.PerformAuthRequest(h.r, "GET", "/api/v1/tyk-connections/3/ingest-status", nil, h.adminKey)
	require.Equal(t, http.StatusOK, w.Code)
	var st tykmcp.IngestStatus
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &st))
	assert.Equal(t, uint(3), st.ConnectionID)
	assert.Equal(t, tykmcp.IngestPath, st.IngestPath)

	w = apitest.PerformAuthRequest(h.r, "DELETE", "/api/v1/tyk-connections/3/ingest-token", nil, h.adminKey)
	require.Equal(t, http.StatusOK, w.Code)

	w = apitest.PerformAuthRequest(h.r, "POST", "/api/v1/tyk-connections/abc/ingest-token", nil, h.adminKey)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// A non-admin reaches none of them.
	for _, rq := range []struct{ method, path string }{
		{"GET", "/api/v1/tyk-connections/3/ingest-status"},
		{"POST", "/api/v1/tyk-connections/3/ingest-token"},
		{"DELETE", "/api/v1/tyk-connections/3/ingest-token"},
	} {
		w := apitest.PerformAuthRequest(h.r, rq.method, rq.path, nil, h.userKey)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s %s", rq.method, rq.path)
	}
	assert.Equal(t, []uint{3}, h.fake.rotated)
}

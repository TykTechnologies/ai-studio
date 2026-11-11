package api

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signalRelay stands in for the cluster event log: a signal sent by one
// replica is delivered to the handlers the others registered.
type signalRelay struct{}

func (signalRelay) IsLeader() bool { return true }
func (signalRelay) Signal(_ context.Context, name string) error {
	replicas.Deliver(name)
	return nil
}

// uploadBrandingFile posts a file with the content type a browser declares.
func uploadBrandingFile(r http.Handler, path, contentType string, data []byte) *httptest.ResponseRecorder {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="upload"`)
	h.Set("Content-Type", contentType)
	part, err := writer.CreatePart(h)
	if err != nil {
		panic(err)
	}
	part.Write(data)
	writer.Close()
	req, _ := http.NewRequest("POST", path, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// A logo uploaded through replica A is served by replica B, which shares
// only the database with it: no volume. B answers with the stored content
// type, the hash as ETag and "no-cache", and 304 to a browser that has it.
// A reset through A returns B to the default. Nothing is written to disk.
func TestBrandingAssetUploadedOnOneReplicaServedByAnother(t *testing.T) {
	useTempBrandingStorage(t)
	dir := services.GetBrandingStoragePath()
	db := apitest.SetupTestDB(t)

	replica := func(admin bool) (*gin.Engine, *services.Service) {
		svc := services.NewService(db)
		config := apitest.SetupTestAuthConfig(db, svc)
		api := NewAPI(svc, true, apitest.SetupTestAuthService(db, svc), config, nil, emptyFile, nil)
		gin.SetMode(gin.TestMode)
		r := gin.New()
		if admin {
			user := createTestUserWithSettings(t, svc, "admin@test.com", "Admin", true, true, true, true, false)
			r.Use(func(c *gin.Context) { c.Set("user", user); c.Next() })
		}
		api.setupBrandingRoutes(r.Group("/api/v1"))
		return r, svc
	}
	a, _ := replica(true)
	b, svcB := replica(false)
	replicas.SetBackend(signalRelay{})
	remove := replicas.OnSignal(replicas.SignalBranding, svcB.InvalidateBrandingAssets)
	t.Cleanup(func() { remove(); replicas.SetBackend(nil) })

	get := func(r http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest("GET", path, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// B has served the default before the upload, so it holds "no logo".
	before := get(b, "/api/v1/branding/logo", nil)
	assert.Empty(t, before.Header().Get("ETag"), "the default carries no ETag")

	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 42}
	w := uploadBrandingFile(a, "/api/v1/branding/logo", "image/png", png)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"has_custom_logo":true`)

	got := get(b, "/api/v1/branding/logo", nil)
	require.Equal(t, http.StatusOK, got.Code)
	assert.Equal(t, png, got.Body.Bytes())
	assert.Equal(t, "image/png", got.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", got.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", got.Header().Get("X-Content-Type-Options"))
	assert.Contains(t, got.Header().Get("Content-Security-Policy"), "sandbox", "an SVG opened directly runs no script")
	assert.Contains(t, got.Header().Get("Content-Security-Policy"), "default-src 'none'")
	etag := got.Header().Get("ETag")
	require.NotEmpty(t, etag)

	for _, match := range []string{etag, "W/" + etag, `"other", ` + etag} {
		cached := get(b, "/api/v1/branding/logo", map[string]string{"If-None-Match": match})
		assert.Equal(t, http.StatusNotModified, cached.Code, match)
		assert.Empty(t, cached.Body.Bytes())
		assert.Equal(t, etag, cached.Header().Get("ETag"))
	}
	changed := get(b, "/api/v1/branding/logo", map[string]string{"If-None-Match": `"something-else"`})
	assert.Equal(t, http.StatusOK, changed.Code)

	ico := []byte{0, 0, 1, 0, 5}
	w = uploadBrandingFile(a, "/api/v1/branding/favicon", "image/x-icon", ico)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	fav := get(b, "/api/v1/branding/favicon", nil)
	require.Equal(t, http.StatusOK, fav.Code)
	assert.Equal(t, ico, fav.Body.Bytes())
	assert.Equal(t, "image/x-icon", fav.Header().Get("Content-Type"))

	// The limits answer as before.
	w = uploadBrandingFile(a, "/api/v1/branding/logo", "image/gif", png)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid file type")
	w = uploadBrandingFile(a, "/api/v1/branding/favicon", "image/png", bytes.Repeat([]byte{1}, services.MaxFaviconSize+1))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "File size exceeds")
	assert.Equal(t, png, get(b, "/api/v1/branding/logo", nil).Body.Bytes(), "a refused upload changes nothing")

	// A database error is not shown to the (public) caller.
	closed := apitest.SetupTestDB(t)
	c, _ := func() (*gin.Engine, *services.Service) {
		svc := services.NewService(closed)
		api := NewAPI(svc, true, apitest.SetupTestAuthService(closed, svc), apitest.SetupTestAuthConfig(closed, svc), nil, emptyFile, nil)
		r := gin.New()
		api.setupBrandingRoutes(r.Group("/api/v1"))
		return r, svc
	}()
	sqlDB, err := closed.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	failed := get(c, "/api/v1/branding/logo", nil)
	assert.Equal(t, http.StatusInternalServerError, failed.Code)
	assert.NotContains(t, failed.Body.String(), "sql")
	assert.NotContains(t, failed.Body.String(), "closed")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "uploads are not written to BRANDING_STORAGE_PATH")

	w = apitest.PerformRequest(a, "POST", "/api/v1/branding/reset", nil)
	require.Equal(t, http.StatusOK, w.Code)
	for _, path := range []string{"/api/v1/branding/logo", "/api/v1/branding/favicon"} {
		after := get(b, path, nil)
		assert.Equal(t, before.Code, after.Code, fmt.Sprintf("%s is the default again", path))
		assert.Empty(t, after.Header().Get("ETag"))
		assert.NotEqual(t, png, after.Body.Bytes())
	}
}

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// Root-level build files (manifest.json, robots.txt, logo192.png) are served
// as themselves. The SPA fallback used to answer them with index.html, so
// browsers read the web manifest as HTML.
func TestServeBuildRootFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	build := fstest.MapFS{
		"ui/admin-frontend/build/index.html":    {Data: []byte("<html></html>")},
		"ui/admin-frontend/build/manifest.json": {Data: []byte(`{"short_name":"Studio"}`)},
		"ui/admin-frontend/build/robots.txt":    {Data: []byte("User-agent: *")},
		"ui/admin-frontend/build/logo192.png":   {Data: []byte("png")},
	}

	serve := func(path string) (*httptest.ResponseRecorder, bool) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, path, nil)
		ok := serveBuildRootFile(c, build)
		return w, ok
	}

	w, ok := serve("/manifest.json")
	assert.True(t, ok)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	assert.JSONEq(t, `{"short_name":"Studio"}`, w.Body.String())

	w, ok = serve("/robots.txt")
	assert.True(t, ok)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/plain")

	w, ok = serve("/logo192.png")
	assert.True(t, ok)
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))

	// Not files of the build root: left to the SPA fallback.
	for _, p := range []string{"/", "/index.html", "/admin/apps", "/missing.json", "/static/../index.html", "/a/manifest.json"} {
		_, ok = serve(p)
		assert.False(t, ok, p)
	}
}

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithBasePath(t *testing.T) {
	var got struct{ path, rawPath string }
	h := withBasePath("/ai-studio", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path, got.rawPath = r.URL.Path, r.URL.RawPath
	}))
	serve := func(target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		return w
	}

	serve("/ai-studio/api/v1/llms")
	assert.Equal(t, "/api/v1/llms", got.path, "the base path is stripped")

	serve("/ai-studio/api/v1/tools/a%2Fb")
	assert.Equal(t, "/api/v1/tools/a/b", got.path)
	assert.Equal(t, "/api/v1/tools/a%2Fb", got.rawPath, "the escaped path is stripped too")

	serve("/api/v1/llms")
	assert.Equal(t, "/api/v1/llms", got.path, "a proxy may already have stripped the base path")

	w := serve("/ai-studio?x=1")
	assert.Equal(t, http.StatusMovedPermanently, w.Code)
	assert.Equal(t, "/ai-studio/?x=1", w.Header().Get("Location"))

	got.path = ""
	serve("/ai-studiox/admin")
	assert.Equal(t, "/ai-studiox/admin", got.path, "only a whole path segment matches")
}

func TestWithBasePathAtRootPassesRequestsThrough(t *testing.T) {
	var got string
	h := withBasePath("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.URL.Path }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ai-studio/admin", nil))
	assert.Equal(t, "/ai-studio/admin", got)
}

func TestInjectBasePath(t *testing.T) {
	index := []byte(`<!doctype html><html><head><link href="/__TYK_AI_BASE__/static/main.css"></head></html>`)

	out := string(injectBasePath(index, "/ai-studio"))
	assert.Contains(t, out, `<head><base href="/ai-studio/"><script>window.__TYK_AI_BASE__="/ai-studio";</script>`)
	assert.Contains(t, out, `href="/ai-studio/static/main.css"`)
	assert.NotContains(t, out, "__TYK_AI_BASE__/")

	root := string(injectBasePath(index, ""))
	assert.Contains(t, root, `href="/static/main.css"`)
	assert.NotContains(t, root, "<base")

	legacy := []byte(`<html><head><script src="/static/js/main.js"></script></head></html>`)
	assert.Equal(t, string(legacy), string(injectBasePath(legacy, "")), "a root build without the placeholder is served unchanged")

	hostile := string(injectBasePath([]byte("<head></head>"), `/x</script><script>alert(1)`))
	assert.False(t, strings.Contains(hostile, "</script><script>alert(1)"), "the base path cannot close the script element")
}

// A configured CSRF_KEY gives every replica and restart the same token key;
// without one each process makes its own.
func TestCSRFKey(t *testing.T) {
	a, err := csrfKey("shared-secret")
	assert.NoError(t, err)
	b, _ := csrfKey("shared-secret")
	assert.Equal(t, a, b)
	assert.Len(t, a, 32)

	r1, _ := csrfKey("")
	r2, _ := csrfKey("")
	assert.Len(t, r1, 32)
	assert.NotEqual(t, r1, r2)
}

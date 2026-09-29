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

func TestInjectBootstrap(t *testing.T) {
	index := []byte(`<!doctype html><html><head><link href="./static/css/main.css"></head></html>`)

	out := string(injectBootstrap(index, frontendBootstrap{BasePath: "/ai-studio", AuthMode: "host", LoginURL: "/login", CSRFTokenHeader: "X-CSRF-Token", CSRFTokenURL: "/ai-studio/csrf-token"}))
	assert.Contains(t, out, `<head><base href="/ai-studio/"><script>window.__TYK_AI_STUDIO__={"basePath":"/ai-studio","authMode":"host","loginURL":"/login","csrfTokenHeader":"X-CSRF-Token","csrfTokenURL":"/ai-studio/csrf-token"};</script>`)
	assert.Contains(t, out, `href="./static/css/main.css"`, "relative asset URLs resolve against the <base> element")

	root := string(injectBootstrap(index, frontendBootstrap{AuthMode: "local"}))
	assert.Contains(t, root, `<base href="/">`, "at the root the base anchors relative assets on deep routes")

	hostile := string(injectBootstrap([]byte("<head></head>"), frontendBootstrap{BasePath: `/x</script><script>alert(1)`}))
	assert.False(t, strings.Contains(hostile, "</script><script>alert(1)"), "a setting cannot close the script element")
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

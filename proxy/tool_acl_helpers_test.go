package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// collapsePathForTest runs a path through the fixDoubleSlash middleware and
// reports what the auth layer downstream would actually see.
func collapsePathForTest(path string) string {
	var seen string
	handler := fixDoubleSlash(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil)
	req.URL.Path = path // preserve the raw form; parsing may normalise it
	handler.ServeHTTP(httptest.NewRecorder(), req)
	return seen
}

// toolSlugFromPath decides whether a request is subject to the tool ACL at all,
// so a path it fails to recognise is a path that skips authorization. These
// pin the boundary cases.
func TestToolSlugFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/tools/my-tool", "my-tool"},
		{"/tools/my-tool/mcp", "my-tool"},
		{"/tools/my-tool/mcp/sse", "my-tool"},
		{"/tools/my-tool/mcp/message", "my-tool"},
		{"/tools/my-tool/anything/deeper", "my-tool"},

		// Not tool paths.
		{"/llm/call/my-llm/v1/chat/completions", ""},
		{"/datasource/my-ds", ""},
		{"/ai/route/v1/chat/completions", ""},
		{"/anthropic/route/v1/messages", ""},
		{"/v1/models", ""},
		{"/.well-known/oauth-protected-resource", ""},
		{"/", ""},
		{"", ""},

		// A bare /tools has no slug to authorise, so it must not be treated as a
		// tool path. The router has no route for it either.
		{"/tools", ""},

		// A trailing slash yields an empty slug. Callers test for "" and fall
		// through, and GetToolBySlug("") would not resolve in any case.
		{"/tools/", ""},

		// Prefix collisions must not be mistaken for tool paths.
		{"/toolsets/x", ""},
		{"/tool/x", ""},

		// Leading double slash: fixDoubleSlash collapses these before auth runs,
		// but if it ever stopped, the raw form must not silently become a
		// non-tool path that skips the ACL. It yields "" here, which makes the
		// OAuth branch refuse rather than pass through.
		{"//tools/my-tool/mcp", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			require.Equal(t, tt.want, toolSlugFromPath(tt.path))
		})
	}
}

// fixDoubleSlash is what makes the "//tools/..." case above unreachable in
// production. If that ever changes, this fails and the note above stops being
// true.
func TestFixDoubleSlashNormalisesToolPathsBeforeAuth(t *testing.T) {
	require.Equal(t, "/tools/my-tool/mcp", collapsePathForTest("//tools/my-tool/mcp"))
	require.Equal(t, "/tools/my-tool/mcp", collapsePathForTest("/tools//my-tool///mcp"))
	require.Equal(t, "my-tool", toolSlugFromPath(collapsePathForTest("//tools/my-tool/mcp")))
}

func TestAppHasTool(t *testing.T) {
	app := &models.App{Tools: []models.Tool{{ID: 1}, {ID: 7}}}

	require.True(t, appHasTool(app, 1))
	require.True(t, appHasTool(app, 7))
	require.False(t, appHasTool(app, 2))

	// Zero is not a real tool id, and must not match a zero-valued entry.
	require.False(t, appHasTool(app, 0))

	// An app with no grants grants nothing.
	require.False(t, appHasTool(&models.App{}, 1))

	// A nil app must not panic and must not authorise. The branches all load the
	// app before calling this, but a nil here has to fail closed rather than
	// crash the gateway.
	require.False(t, appHasTool(nil, 1))
}

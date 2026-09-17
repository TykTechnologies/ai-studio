package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A tool is a chat capability first. REST and MCP on the gateway are optional
// access methods, switched per tool. These tests pin what the gateway does
// when a method is off, and that the two methods enforce the same rules.

// mcpRoutes are the three routes that make up a tool's MCP endpoint.
var mcpRoutes = []struct {
	name   string
	method string
	path   string
}{
	{"streamable HTTP", http.MethodPost, "/tools/" + testToolSlug + "/mcp"},
	{"SSE stream", http.MethodGet, "/tools/" + testToolSlug + "/mcp/sse"},
	{"SSE message", http.MethodPost, "/tools/" + testToolSlug + "/mcp/message"},
}

const mcpInitializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`

func (f *toolFilterFixture) request(t *testing.T, method, path, apiKey, body string) *httptest.ResponseRecorder {
	t.Helper()

	req, err := http.NewRequest(method, path, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)
	return rr
}

func TestToolAccess_RESTOffRefusesRESTOnly(t *testing.T) {
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	setTestToolAccess(t, f.service, f.tool.ID, false, true)

	rr := f.call(t, "getTestData", nil)
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), toolRESTAccessDisabledMessage)
	assert.Empty(t, rr.Header().Get("WWW-Authenticate"),
		"a switched-off method is not an authentication problem")
	assert.Empty(t, f.upstream.ReceivedRequests, "a refused call must not reach the tool")

	// MCP is a separate switch and still serves.
	session := newMCPSession(t, f)
	_, isErr := session.callTool(t, "getTestData", map[string]interface{}{})
	assert.False(t, isErr)
}

func TestToolAccess_MCPOffRefusesEveryMCPRoute(t *testing.T) {
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	setTestToolAccess(t, f.service, f.tool.ID, true, false)

	for _, route := range mcpRoutes {
		t.Run(route.name, func(t *testing.T) {
			rr := f.request(t, route.method, route.path, f.apiKey, mcpInitializeBody)
			assert.Equal(t, http.StatusForbidden, rr.Code, "body: %s", rr.Body.String())
			assert.Contains(t, rr.Body.String(), toolMCPAccessDisabledMessage)
			assert.Empty(t, rr.Header().Get("WWW-Authenticate"),
				"a 401 here would send an MCP client into an OAuth loop")
		})
	}

	// REST is a separate switch and still serves.
	rr := f.call(t, "getTestData", nil)
	assert.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
}

// The MCP server for a tool is cached. The switch must be read per request,
// or turning MCP off would only take effect once the cache entry expired.
func TestToolAccess_MCPSwitchAppliesWithWarmCache(t *testing.T) {
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	session := newMCPSession(t, f)
	_, isErr := session.callTool(t, "getTestData", map[string]interface{}{})
	require.False(t, isErr, "the cache is now warm")

	setTestToolAccess(t, f.service, f.tool.ID, true, false)
	rr := session.post(t, `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`)
	assert.Equal(t, http.StatusForbidden, rr.Code, "body: %s", rr.Body.String())

	setTestToolAccess(t, f.service, f.tool.ID, true, true)
	rr = session.post(t, `{"jsonrpc":"2.0","id":4,"method":"tools/list"}`)
	assert.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
}

// REST used to pass any operation_id in the spec straight to the tool, while
// MCP only ever exposed the whitelist.
func TestToolAccess_RESTEnforcesOperationWhitelist(t *testing.T) {
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	// The spec declares getTestData and submitTestData; allow only the first.
	exposeTestTool(t, f.service, f.tool.ID, "getTestData")

	rr := f.call(t, "submitTestData", map[string]interface{}{"key": "value"})
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), toolOperationNotPermittedMessage)
	assert.Empty(t, f.upstream.ReceivedRequests, "a refused operation must not reach the tool")

	rr = f.call(t, "getTestData", nil)
	assert.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
}

func TestToolAccess_RESTEmptyWhitelistAllowsNothing(t *testing.T) {
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	exposeTestTool(t, f.service, f.tool.ID, "")

	rr := f.call(t, "getTestData", nil)
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Empty(t, f.upstream.ReceivedRequests)
}

// An inactive tool is never shipped to an edge gateway. The embedded gateway
// has to refuse it too, and in the same way as a tool that does not exist.
func TestToolAccess_InactiveToolIsRefused(t *testing.T) {
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	require.NoError(t, f.service.DB.Model(&models.Tool{}).
		Where("id = ?", f.tool.ID).Update("active", false).Error)

	rr := f.call(t, "getTestData", nil)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Empty(t, f.upstream.ReceivedRequests)

	rr = f.request(t, http.MethodPost, "/tools/"+testToolSlug+"/mcp", f.apiKey, mcpInitializeBody)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// The 403 for a switched-off method is only for callers that passed the App
// ACL. Anyone else keeps getting the uniform 401, so the refusal does not
// reveal which tools exist or how they are configured.
func TestToolAccess_DisabledMethodStaysUniformForUnentitledApp(t *testing.T) {
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	setTestToolAccess(t, f.service, f.tool.ID, false, false)

	var owner models.User
	require.NoError(t, f.db.First(&owner).Error)
	other, err := f.service.CreateApp("No Tools App", "holds no tool", owner.ID,
		[]uint{}, []uint{}, []uint{}, nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, f.service.ActivateAppCredential(other.ID))
	other, err = f.service.GetAppByID(other.ID)
	require.NoError(t, err)
	require.NotNil(t, other.Credential)

	rr := f.request(t, http.MethodPost, "/tools/"+testToolSlug, other.Credential.Secret,
		`{"operation_id":"getTestData"}`)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	for _, route := range mcpRoutes {
		rr := f.request(t, route.method, route.path, other.Credential.Secret, mcpInitializeBody)
		assert.Equal(t, http.StatusUnauthorized, rr.Code, route.name)
	}
}

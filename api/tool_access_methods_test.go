package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A tool's REST and MCP access methods are switched per tool: off for a new
// tool, unchanged by an update that does not mention them, and never
// available on a client tool.

func toolAccessPayload(name, toolType string, rest, mcp *bool) map[string]interface{} {
	attributes := map[string]interface{}{
		"name":          name,
		"description":   "A tool",
		"tool_type":     toolType,
		"oas_spec":      testOASSpec(`{"openapi": "3.0.0"}`),
		"privacy_score": 1,
		"operations":    []string{"getThing"},
	}
	if rest != nil {
		attributes["rest_access_enabled"] = *rest
	}
	if mcp != nil {
		attributes["mcp_access_enabled"] = *mcp
	}
	return map[string]interface{}{"data": map[string]interface{}{"type": "tools", "attributes": attributes}}
}

func decodeTool(t *testing.T, w *httptest.ResponseRecorder) ToolResponse {
	t.Helper()
	var response map[string]ToolResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response), w.Body.String())
	return response["data"]
}

func TestToolAccessMethods_CreateAndUpdate(t *testing.T) {
	api, _ := setupTestAPI(t)
	on, off := true, false

	// Created without the switches: chat only.
	w := performRequest(api.router, "POST", "/api/v1/tools", toolAccessPayload("Quiet Tool", models.ToolTypeREST, nil, nil))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	quiet := decodeTool(t, w)
	assert.False(t, quiet.Attributes.RESTAccessEnabled)
	assert.False(t, quiet.Attributes.MCPAccessEnabled)
	assert.False(t, quiet.Attributes.AppGrantable)
	assert.Equal(t, "quiet-tool", quiet.Attributes.Slug)

	// Created with one switch on.
	w = performRequest(api.router, "POST", "/api/v1/tools", toolAccessPayload("MCP Tool", models.ToolTypeREST, nil, &on))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	mcpTool := decodeTool(t, w)
	assert.False(t, mcpTool.Attributes.RESTAccessEnabled)
	assert.True(t, mcpTool.Attributes.MCPAccessEnabled)
	assert.True(t, mcpTool.Attributes.AppGrantable)

	// An update that does not mention the switches leaves them alone.
	w = performRequest(api.router, "PATCH", "/api/v1/tools/"+mcpTool.ID, toolAccessPayload("MCP Tool", models.ToolTypeREST, nil, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	updated := decodeTool(t, w)
	assert.False(t, updated.Attributes.RESTAccessEnabled)
	assert.True(t, updated.Attributes.MCPAccessEnabled)

	// An update can flip each one independently.
	w = performRequest(api.router, "PATCH", "/api/v1/tools/"+mcpTool.ID, toolAccessPayload("MCP Tool", models.ToolTypeREST, &on, &off))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	updated = decodeTool(t, w)
	assert.True(t, updated.Attributes.RESTAccessEnabled)
	assert.False(t, updated.Attributes.MCPAccessEnabled)

	// The stored value survives a fresh read.
	w = performRequest(api.router, "GET", "/api/v1/tools/"+mcpTool.ID, nil)
	require.Equal(t, http.StatusOK, w.Code)
	fetched := decodeTool(t, w)
	assert.True(t, fetched.Attributes.RESTAccessEnabled)
	assert.False(t, fetched.Attributes.MCPAccessEnabled)
}

func TestToolAccessMethods_ClientToolRefusesGatewayAccess(t *testing.T) {
	api, db := setupTestAPI(t)
	on := true

	w := performRequest(api.router, "POST", "/api/v1/tools", toolAccessPayload("Approval", models.ToolTypeClient, &on, nil))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.Tool{}).Where("name = ?", "Approval").Count(&count).Error)
	assert.Zero(t, count, "a refused create must leave nothing behind")

	w = performRequest(api.router, "POST", "/api/v1/tools", toolAccessPayload("Approval", models.ToolTypeClient, nil, nil))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeTool(t, w)
	assert.False(t, created.Attributes.AppGrantable)
	assert.Empty(t, created.Attributes.MCPEndpointURL)

	w = performRequest(api.router, "PATCH", "/api/v1/tools/"+created.ID, toolAccessPayload("Approval", models.ToolTypeClient, nil, &on))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// The endpoint URLs come from the server because the slug does: the browser
// cannot reproduce the transliteration ("Café" is served under "cafe").
func TestToolGatewayAccess_EndpointURLsUseTheStoredSlug(t *testing.T) {
	t.Setenv("TOOL_DISPLAY_URL", "https://gateway.example.com/")
	t.Setenv("PROXY_URL", "https://proxy.example.com")
	config.ResetGlobalConfig()
	t.Cleanup(config.ResetGlobalConfig)

	access := toolGatewayAccess(&models.Tool{Name: "Café & Bar", Slug: "cafe-and-bar", ToolType: models.ToolTypeREST})
	assert.Equal(t, "https://gateway.example.com/tools/cafe-and-bar", access.RESTEndpointURL)
	assert.Equal(t, "https://gateway.example.com/tools/cafe-and-bar/mcp", access.MCPEndpointURL)

	client := toolGatewayAccess(&models.Tool{Name: "Approve", Slug: "approve", ToolType: models.ToolTypeClient})
	assert.Empty(t, client.RESTEndpointURL)
	assert.Empty(t, client.MCPEndpointURL)
}

// A chat-only tool is offered in chat and nowhere in the portal.
func TestPortal_ChatOnlyToolIsNotShown(t *testing.T) {
	api, db, service := setupTestAPIForCommonTests(t)

	user := createTestUser(t, service)
	toolCat := createTestToolCatalogue(t, service)
	giveUserTeam(t, service, "Platform", user.ID, nil, nil, []uint{toolCat.ID})

	exposed := createTestTool(t, service, "Exposed Tool")
	require.NoError(t, service.AddToolToToolCatalogue(exposed.ID, toolCat.ID))
	require.NoError(t, db.Model(&models.Tool{}).Where("id = ?", exposed.ID).Update("rest_access_disabled", true).Error)

	chatOnly, err := service.CreateTool("Chat Only Tool", "Description", models.ToolTypeREST, testOASSpec(`{"openapi": "3.0.0"}`), 1, "", "")
	require.NoError(t, err)
	require.NoError(t, service.AddToolToToolCatalogue(chatOnly.ID, toolCat.ID))

	// Browse lists only the exposed tool, with its one enabled method.
	w := portalGet(t, api.getPortalCatalog, user)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list CatalogListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 1)
	item := list.Data[0]
	assert.Equal(t, "Exposed Tool", item.Attributes.Name)
	assert.True(t, item.Attributes.AccessGrantedViaApp)
	require.NotNil(t, item.Attributes.MCPAccessEnabled)
	assert.True(t, *item.Attributes.MCPAccessEnabled)
	require.NotNil(t, item.Attributes.RESTAccessEnabled)
	assert.False(t, *item.Attributes.RESTAccessEnabled)
	assert.Empty(t, item.Attributes.RESTEndpointURL, "a switched-off method has no URL in the portal")
	assert.Equal(t, 1, list.Meta.Counts["tool"])

	// Its detail page does not exist (nor does its documentation page; see
	// the enterprise test, the page being an Enterprise feature).
	w = portalGet(t, api.getPortalCatalogTool, user, gin.Param{Key: "id", Value: idOf(chatOnly.ID)})
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = portalGet(t, api.getPortalCatalogTool, user, gin.Param{Key: "id", Value: idOf(exposed.ID)})
	assert.Equal(t, http.StatusOK, w.Code)

	// The chat picker still offers it; the App builder's view does not.
	names := func(query string) []string {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/common/accessible-tools"+query, nil)
		c.Set("user", user)
		api.getUserAccessibleTools(c)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var tools []ToolResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tools))
		out := []string{}
		for _, tool := range tools {
			out = append(out, tool.Attributes.Name)
		}
		return out
	}
	assert.ElementsMatch(t, []string{"Exposed Tool", "Chat Only Tool"}, names(""))
	assert.ElementsMatch(t, []string{"Exposed Tool"}, names("?app_grantable=true"))

	// And the portal refuses to bind it to an App.
	body := fmt.Sprintf(`{"name":"App","description":"d","llm_ids":[],"datasource_ids":[],"tool_ids":[%d]}`, chatOnly.ID)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/common/apps", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", user)
	api.createUserApp(c)
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// The built-in Generative UI tool is a client tool: the chat draws what the
// model asks for, and nothing about it can be called by an App. It is seeded
// into the Default tool catalogue, so it used to appear in the portal. It
// stays in the chat picker and is shown nowhere else.
func TestPortal_BuiltInGenerativeUIToolIsChatOnly(t *testing.T) {
	api, db, service := setupTestAPIForCommonTests(t)

	require.NoError(t, models.GetOrCreateDefaultClientTools(db))
	var present models.Tool
	require.NoError(t, db.Where("tool_type = ? AND available_operations = ?",
		models.ToolTypeClient, models.PresentToolOperation).First(&present).Error)

	user := createTestUser(t, service)
	toolCat := createTestToolCatalogue(t, service)
	require.NoError(t, service.AddToolToToolCatalogue(present.ID, toolCat.ID))
	giveUserTeam(t, service, "Platform", user.ID, nil, nil, []uint{toolCat.ID})

	w := portalGet(t, api.getPortalCatalog, user)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list CatalogListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Empty(t, list.Data, "the Generative UI tool must not be listed in the portal")
	assert.Equal(t, 0, list.Meta.Counts["tool"])

	w = portalGet(t, api.getPortalCatalogTool, user, gin.Param{Key: "id", Value: idOf(present.ID)})
	assert.Equal(t, http.StatusNotFound, w.Code)

	accessible := func(query string) []string {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/common/accessible-tools"+query, nil)
		c.Set("user", user)
		api.getUserAccessibleTools(c)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var tools []ToolResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tools))
		out := []string{}
		for _, tool := range tools {
			out = append(out, tool.Attributes.Name)
		}
		return out
	}
	assert.Equal(t, []string{models.DefaultPresentToolName}, accessible(""), "chat still offers it")
	assert.Empty(t, accessible("?app_grantable=true"), "the App builder does not")

	_, err := service.CreateApp("App", "d", user.ID, nil, nil, []uint{present.ID}, nil, nil, nil)
	assert.Error(t, err, "and it cannot be bound to an App")
}

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression: GET /common/apps/:id did not serialise the App's MCP servers,
// so the portal app page never showed the "MCP access" section although the
// list endpoint and /common/apps/:id/mcp both had the binding.
func TestCommon_GetUserAppDetailsIncludesMCPServers(t *testing.T) {
	api, db, service := setupTestAPIForCommonTests(t)

	user := createTestUserWithSettings(t, service, "mcpdetail@example.com", "MCP Detail", false, true, true, true, false)

	payload, _ := json.Marshal(CreateAppRequest{Name: "AppWithMCP", Description: "mcp", DataSourceIDs: []uint{}, LLMIDs: []uint{}, ToolIDs: []uint{}})
	wCreate := httptest.NewRecorder()
	reqCreate, _ := http.NewRequest("POST", "/common/apps", bytes.NewBuffer(payload))
	reqCreate.Header.Set("Content-Type", "application/json")
	cCreate, _ := gin.CreateTestContext(wCreate)
	cCreate.Request = reqCreate
	cCreate.Set("user", user)
	api.createUserApp(cCreate)
	require.Equal(t, http.StatusCreated, wCreate.Code, wCreate.Body.String())
	var created AppResponse
	require.NoError(t, json.Unmarshal(wCreate.Body.Bytes(), &created))

	server := &models.MCPServer{Name: "Support MCP", Slug: "support-mcp", AuthMode: "api_key", EndpointURL: "https://mcp.example.com/mcp", Brokerable: true}
	require.NoError(t, db.Create(server).Error)
	var app models.App
	require.NoError(t, db.First(&app, created.ID).Error)
	require.NoError(t, db.Model(&app).Association("MCPServers").Append(server))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/common/apps/%s", created.ID), nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("user", user)
	c.Params = gin.Params{gin.Param{Key: "id", Value: created.ID}}
	api.getUserAppDetails(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body struct {
		Attributes struct {
			MCPServerIDs []uint               `json:"mcp_server_ids"`
			MCPServers   []AppMCPServerOutput `json:"mcp_servers"`
		} `json:"attributes"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, []uint{server.ID}, body.Attributes.MCPServerIDs, "detail must carry the same mcp_server_ids as the list endpoint: %s", w.Body.String())
	require.Len(t, body.Attributes.MCPServers, 1)
	assert.Equal(t, "Support MCP", body.Attributes.MCPServers[0].Name)
	assert.Equal(t, "support-mcp", body.Attributes.MCPServers[0].Slug)
	assert.True(t, body.Attributes.MCPServers[0].Brokerable)
}

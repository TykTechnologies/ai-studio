package services

import (
	"encoding/base64"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const capabilityTestSpec = `{
  "openapi": "3.0.0",
  "info": {"title": "Repo", "version": "1.0.0"},
  "servers": [{"url": "https://api.example.com"}],
  "paths": {
    "/pulls/{n}": {
      "get": {
        "operationId": "getPull",
        "parameters": [{"name": "n", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/search": {
      "post": {
        "operationId": "searchCode",
        "x-mcp-read-only": true,
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/comments": {
      "post": {
        "operationId": "postComment",
        "responses": {"201": {"description": "Created"}}
      }
    }
  }
}`

func capabilityTool(name, ops string) models.Tool {
	return models.Tool{
		Name:                name,
		ToolType:            models.ToolTypeREST,
		OASSpec:             base64.StdEncoding.EncodeToString([]byte(capabilityTestSpec)),
		AvailableOperations: ops,
	}
}

func capabilityMCPServer(name string, prims []models.MCPPrimitive) models.MCPServer {
	m := models.MCPServer{Name: name}
	m.SetPrimitives(prims)
	return m
}

func TestSuggestAppCapability(t *testing.T) {
	t.Run("nothing attached is read_only", func(t *testing.T) {
		s := SuggestAppCapability(&models.App{})
		assert.Equal(t, guardrails.CapabilityReadOnly, s.Suggestion)
		assert.Equal(t, []string{"No tools, datasources or MCP servers attached"}, s.Reasons)
	})

	t.Run("GET-only tool is read_tools", func(t *testing.T) {
		s := SuggestAppCapability(&models.App{Tools: []models.Tool{capabilityTool("GitHub", "getPull")}})
		assert.Equal(t, guardrails.CapabilityReadTools, s.Suggestion)
		require.Len(t, s.Reasons, 1)
		assert.Contains(t, s.Reasons[0], `"GitHub"`)
	})

	t.Run("x-mcp-read-only POST counts as read", func(t *testing.T) {
		s := SuggestAppCapability(&models.App{Tools: []models.Tool{capabilityTool("GitHub", "getPull,searchCode")}})
		assert.Equal(t, guardrails.CapabilityReadTools, s.Suggestion)
	})

	t.Run("a POST operation is acting and named", func(t *testing.T) {
		s := SuggestAppCapability(&models.App{Tools: []models.Tool{
			capabilityTool("Reader", "getPull"),
			capabilityTool("GitHub", "getPull,postComment"),
		}})
		assert.Equal(t, guardrails.CapabilityActing, s.Suggestion)
		require.Len(t, s.Reasons, 1, "only the deciding operations are listed")
		assert.Contains(t, s.Reasons[0], `"GitHub"`)
		assert.Contains(t, s.Reasons[0], "postComment")
		assert.Contains(t, s.Reasons[0], "POST")
	})

	t.Run("an operation only the spec lacks is acting", func(t *testing.T) {
		s := SuggestAppCapability(&models.App{Tools: []models.Tool{capabilityTool("GitHub", "deleteRepo")}})
		assert.Equal(t, guardrails.CapabilityActing, s.Suggestion)
	})

	t.Run("an unreadable spec is acting", func(t *testing.T) {
		tool := capabilityTool("Broken", "getPull")
		tool.OASSpec = "not base64!"
		s := SuggestAppCapability(&models.App{Tools: []models.Tool{tool}})
		assert.Equal(t, guardrails.CapabilityActing, s.Suggestion)
		assert.Contains(t, s.Reasons[0], `"Broken"`)
	})

	t.Run("a chat-only tool adds nothing", func(t *testing.T) {
		tool := capabilityTool("ChatOnly", "postComment")
		tool.RESTAccessDisabled, tool.MCPAccessDisabled = true, true
		s := SuggestAppCapability(&models.App{Tools: []models.Tool{tool}})
		assert.Equal(t, guardrails.CapabilityReadOnly, s.Suggestion)
	})

	t.Run("datasources are read_tools", func(t *testing.T) {
		s := SuggestAppCapability(&models.App{Datasources: []models.Datasource{{Name: "Docs"}}})
		assert.Equal(t, guardrails.CapabilityReadTools, s.Suggestion)
		assert.Contains(t, s.Reasons[0], `"Docs"`)
	})

	t.Run("read-only MCP primitives are read_tools", func(t *testing.T) {
		s := SuggestAppCapability(&models.App{MCPServers: []models.MCPServer{capabilityMCPServer("Search", []models.MCPPrimitive{
			{Type: "tool", Name: "search", Annotations: map[string]interface{}{"readOnlyHint": true}},
			{Type: "resource", Name: "docs"},
			{Type: "prompt", Name: "summarise"},
		})}})
		assert.Equal(t, guardrails.CapabilityReadTools, s.Suggestion)
		assert.Contains(t, s.Reasons[0], `"Search"`)
	})

	t.Run("a destructive MCP primitive is acting and named", func(t *testing.T) {
		s := SuggestAppCapability(&models.App{MCPServers: []models.MCPServer{capabilityMCPServer("Jira", []models.MCPPrimitive{
			{Type: "tool", Name: "search", Annotations: map[string]interface{}{"readOnlyHint": true}},
			{Type: "tool", Name: "delete_issue", Annotations: map[string]interface{}{"destructiveHint": true}},
		})}})
		assert.Equal(t, guardrails.CapabilityActing, s.Suggestion)
		require.Len(t, s.Reasons, 1)
		assert.Contains(t, s.Reasons[0], `"Jira"`)
		assert.Contains(t, s.Reasons[0], "delete_issue")
	})

	t.Run("an MCP server with unknown primitives is acting", func(t *testing.T) {
		s := SuggestAppCapability(&models.App{MCPServers: []models.MCPServer{capabilityMCPServer("Opaque", nil)}})
		assert.Equal(t, guardrails.CapabilityActing, s.Suggestion)
		assert.Contains(t, s.Reasons[0], `"Opaque"`)
	})
}

func TestSetAppGuardrailCapability(t *testing.T) {
	svc := NewService(setupTestDB(t))
	app := &models.App{Name: "Visor", UserID: 1}
	require.NoError(t, app.Create(svc.DB))

	got, err := svc.GetAppByID(app.ID)
	require.NoError(t, err)
	assert.Equal(t, "", got.GuardrailCapability, "new apps are unattested")
	assert.Equal(t, guardrails.CapabilityActing, guardrails.Capability(got.GuardrailCapability).Effective())

	_, err = svc.SetAppGuardrailCapability(app.ID, "superuser", 1)
	assert.ErrorIs(t, err, ErrInvalidGuardrailCapability)

	updated, err := svc.SetAppGuardrailCapability(app.ID, "read_only", 1)
	require.NoError(t, err)
	assert.Equal(t, "read_only", updated.GuardrailCapability)

	// An ordinary app update keeps the attested value.
	_, err = svc.UpdateApp(app.ID, "Visor 2", "", 1, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	got, err = svc.GetAppByID(app.ID)
	require.NoError(t, err)
	assert.Equal(t, "read_only", got.GuardrailCapability)

	cleared, err := svc.SetAppGuardrailCapability(app.ID, "", 1)
	require.NoError(t, err)
	assert.Equal(t, "", cleared.GuardrailCapability)
}

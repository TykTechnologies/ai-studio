package services

import (
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A tool is a chat capability first; REST and MCP on the gateway are opt-in.

func TestCreateTool_NewToolIsChatOnly(t *testing.T) {
	service := NewService(setupTestDBForTools(t))

	tool, err := service.CreateTool("Fresh Tool", "Description", models.ToolTypeREST, "OAS Spec", 1, "", "")
	require.NoError(t, err)

	stored, err := service.GetToolByID(tool.ID)
	require.NoError(t, err)
	assert.False(t, stored.RESTAccessEnabled())
	assert.False(t, stored.MCPAccessEnabled())
	assert.False(t, stored.AppGrantable())
}

// The admin API creates a tool in one step, so hooks and the created event see
// it as it will be served, not bare and then corrected by a second save.
func TestCreateToolWithOptions_CreatesTheToolFullyConfigured(t *testing.T) {
	service := NewService(setupTestDBForTools(t))
	on, off := true, false

	tool, err := service.CreateToolWithOptions(service.DB, "Configured", "Description", models.ToolTypeREST, "OAS Spec", 1, "", "",
		ToolCreateOptions{Operations: []string{"getThing", "listThings"}, Namespace: "team-a", Active: &off, MCPAccessEnabled: &on})
	require.NoError(t, err)

	stored, err := service.GetToolByID(tool.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"getThing", "listThings"}, stored.GetOperations())
	assert.Equal(t, "team-a", stored.Namespace)
	assert.False(t, stored.Active, "an explicit draft must survive the column's default of true")
	assert.False(t, tool.Active, "and the returned tool must say so too")
	assert.False(t, stored.RESTAccessEnabled(), "a method that was not asked for keeps the default")
	assert.True(t, stored.MCPAccessEnabled())

	// No options at all: active, chat only, nothing whitelisted.
	plain, err := service.CreateToolWithOptions(service.DB, "Plain", "Description", models.ToolTypeREST, "OAS Spec", 1, "", "", ToolCreateOptions{})
	require.NoError(t, err)
	stored, err = service.GetToolByID(plain.ID)
	require.NoError(t, err)
	assert.True(t, stored.Active)
	assert.False(t, stored.AppGrantable())
	assert.Empty(t, stored.GetOperations())
}

// Rows written before the switches existed carry the column default. They
// must keep both methods on, or an upgrade would cut off every tool in use.
func TestTool_RowWithoutSwitchesStaysEnabled(t *testing.T) {
	db := setupTestDBForTools(t)

	require.NoError(t, db.Exec(
		"INSERT INTO tools (name, slug, tool_type, active) VALUES (?, ?, ?, ?)",
		"Legacy Tool", "legacy-tool", models.ToolTypeREST, true).Error)

	var tool models.Tool
	require.NoError(t, db.Where("slug = ?", "legacy-tool").First(&tool).Error)
	assert.True(t, tool.RESTAccessEnabled())
	assert.True(t, tool.MCPAccessEnabled())
	assert.True(t, tool.AppGrantable())
}

func TestTool_ClientToolHasNoGatewayAccess(t *testing.T) {
	tool := &models.Tool{ToolType: models.ToolTypeClient}
	assert.False(t, tool.RESTAccessEnabled())
	assert.False(t, tool.MCPAccessEnabled())
	assert.False(t, tool.AppGrantable())
}

func TestTool_AllowsOperation(t *testing.T) {
	tool := &models.Tool{AvailableOperations: "getThing, listThings"}
	assert.True(t, tool.AllowsOperation("getThing"))
	assert.True(t, tool.AllowsOperation("listThings"), "whitespace around an entry is ignored")
	assert.False(t, tool.AllowsOperation("deleteThing"))
	assert.False(t, tool.AllowsOperation(""))
	assert.False(t, (&models.Tool{}).AllowsOperation("getThing"), "an empty whitelist allows nothing")
}

// The gateway resolves tools by slug. An inactive tool is never shipped to an
// edge gateway, so the embedded gateway must not resolve it either.
func TestGetToolBySlug_SkipsInactiveTool(t *testing.T) {
	service := NewService(setupTestDBForTools(t))

	tool, err := service.CreateTool("Sleeping Tool", "Description", models.ToolTypeREST, "OAS Spec", 1, "", "")
	require.NoError(t, err)

	_, err = service.GetToolBySlug("sleeping-tool")
	require.NoError(t, err)

	require.NoError(t, service.DB.Model(&models.Tool{}).Where("id = ?", tool.ID).Update("active", false).Error)
	_, err = service.GetToolBySlug("sleeping-tool")
	assert.Error(t, err)

	// The admin lookup by ID still finds it.
	_, err = service.GetToolByID(tool.ID)
	assert.NoError(t, err)
}

func TestApp_ChatOnlyToolCannotBeBound(t *testing.T) {
	service := NewService(setupTestDBForTools(t))

	user, err := service.CreateUser(UserDTO{Email: "binder@example.com", Name: "binder", Password: "password123"})
	require.NoError(t, err)

	chatOnly, err := service.CreateTool("Chat Only", "Description", models.ToolTypeREST, "OAS Spec", 1, "", "")
	require.NoError(t, err)
	exposed, err := service.CreateTool("Exposed", "Description", models.ToolTypeREST, "OAS Spec", 1, "", "")
	require.NoError(t, err)
	require.NoError(t, service.DB.Model(&models.Tool{}).Where("id = ?", exposed.ID).
		Update("mcp_access_disabled", false).Error)

	_, err = service.CreateApp("Refused", "desc", user.ID, nil, nil, []uint{chatOnly.ID}, nil, nil, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrResourceNotAppGranted))

	app, err := service.CreateApp("Accepted", "desc", user.ID, nil, nil, []uint{exposed.ID}, nil, nil, nil)
	require.NoError(t, err)

	_, err = service.AddToolToApp(app.ID, chatOnly.ID)
	assert.True(t, errors.Is(err, ErrResourceNotAppGranted))

	// An admin later switches the bound tool to chat only. The App keeps the
	// binding and can still be edited; the gateway refuses the tool instead.
	require.NoError(t, service.DB.Model(&models.Tool{}).Where("id = ?", exposed.ID).
		Update("mcp_access_disabled", true).Error)
	updated, err := service.UpdateApp(app.ID, "Accepted, renamed", "desc", user.ID, nil, nil, []uint{exposed.ID}, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, updated.Tools, 1)

	// Adding another chat-only tool in the same update is still refused.
	_, err = service.UpdateApp(app.ID, "Accepted", "desc", user.ID, nil, nil, []uint{exposed.ID, chatOnly.ID}, nil, nil, nil)
	assert.True(t, errors.Is(err, ErrResourceNotAppGranted))
}

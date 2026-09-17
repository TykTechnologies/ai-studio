//go:build enterprise

package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A tool catalogue page lists the MCP servers published in it, up to a limit,
// and says when the list was cut rather than passing a part off as the whole.
func TestToolCatalogueMCPServers_ReportsTruncation(t *testing.T) {
	api, db, service := setupTestAPIForCommonTests(t)
	catalogue := createTestToolCatalogue(t, service)

	for i, name := range []string{"Charlie", "Alpha", "Bravo"} {
		server := models.MCPServer{Name: name, Slug: strings.ToLower(name), TykAPIID: fmt.Sprintf("api-%d", i), Kind: "remote"}
		require.NoError(t, db.Create(&server).Error)
		require.NoError(t, db.Exec("INSERT INTO tool_catalogue_mcp_servers (tool_catalogue_id, mcp_server_id) VALUES (?, ?)",
			catalogue.ID, server.ID).Error)
	}

	original := toolCatalogueMCPServerLimit
	t.Cleanup(func() { toolCatalogueMCPServerLimit = original })

	toolCatalogueMCPServerLimit = 2
	servers, truncated, err := api.toolCatalogueMCPServers(catalogue.ID)
	require.NoError(t, err)
	assert.True(t, truncated)
	require.Len(t, servers, 2)
	assert.Equal(t, []string{"Alpha", "Bravo"}, []string{servers[0].Name, servers[1].Name}, "listed by name")

	toolCatalogueMCPServerLimit = 3
	servers, truncated, err = api.toolCatalogueMCPServers(catalogue.ID)
	require.NoError(t, err)
	assert.False(t, truncated, "exactly the limit is the whole list")
	assert.Len(t, servers, 3)
}

// The tool documentation page tells a developer how an App calls the tool on
// the gateway. A chat-only tool has no such endpoint, so it has no page.
func TestToolDocumentation_ChatOnlyToolHasNoPage(t *testing.T) {
	api, db, service := setupTestAPIForCommonTests(t)
	user := createTestUser(t, service)

	spec := testOASSpec(`{"openapi":"3.0.0","info":{"title":"T","version":"1"},"paths":{"/thing":{"get":{"operationId":"getThing","responses":{"200":{"description":"OK"}}}}}}`)
	tool, err := service.CreateTool("Docs Tool", "Description", models.ToolTypeREST, spec, 1, "", "")
	require.NoError(t, err)
	require.NoError(t, service.AddOperationToTool(tool.ID, "getThing"))

	w := portalGet(t, api.GetToolDocumentation, user, gin.Param{Key: "id", Value: idOf(tool.ID)})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	require.NoError(t, db.Model(&models.Tool{}).Where("id = ?", tool.ID).Update("mcp_access_disabled", false).Error)
	w = portalGet(t, api.GetToolDocumentation, user, gin.Param{Key: "id", Value: idOf(tool.ID)})
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

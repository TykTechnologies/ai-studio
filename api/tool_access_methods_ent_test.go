//go:build enterprise

package api

import (
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

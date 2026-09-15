package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOrCreateDefaultClientTools_SeedsPresentOnce(t *testing.T) {
	db := setupTestDB(t)
	catalogue, err := GetOrCreateDefaultToolCatalogue(db)
	require.NoError(t, err)

	require.NoError(t, GetOrCreateDefaultClientTools(db))
	require.NoError(t, GetOrCreateDefaultClientTools(db)) // idempotent

	var tools []Tool
	require.NoError(t, db.Where("tool_type = ?", ToolTypeClient).Find(&tools).Error)
	require.Len(t, tools, 1)
	tool := tools[0]
	assert.Equal(t, DefaultPresentToolName, tool.Name)
	assert.Equal(t, PresentToolOperation, tool.ClientOperation())
	assert.True(t, tool.Active)
	assert.Equal(t, 0, tool.PrivacyScore)
	assert.Equal(t, true, tool.Metadata["builtin"])

	def, err := tool.ClientDefinition()
	require.NoError(t, err)
	assert.Equal(t, ClientToolKindPresent, def.UI.Kind)
	assert.Contains(t, def.Parameters["properties"].(map[string]interface{}), "component")

	// It is visible to everyone through the Default tool catalogue.
	var count int64
	db.Table("tool_catalogue_tools").Where("tool_catalogue_id = ? AND tool_id = ?", catalogue.ID, tool.ID).Count(&count)
	assert.Equal(t, int64(1), count)
}

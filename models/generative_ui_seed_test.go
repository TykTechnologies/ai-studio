package models

import (
	"encoding/base64"
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

	// Stored base64 like every other tool spec: the tool editor and the
	// chat window's add-tool call both decode it.
	_, err = base64.StdEncoding.DecodeString(tool.OASSpec)
	require.NoError(t, err)

	def, err := tool.ClientDefinition()
	require.NoError(t, err)
	assert.Equal(t, ClientToolKindPresent, def.UI.Kind)
	assert.Contains(t, def.Parameters["properties"].(map[string]interface{}), "component")

	// It is visible to everyone through the Default tool catalogue.
	var count int64
	db.Table("tool_catalogue_tools").Where("tool_catalogue_id = ? AND tool_id = ?", catalogue.ID, tool.ID).Count(&count)
	assert.Equal(t, int64(1), count)
}

// Installations seeded before the spec was base64-encoded hold raw JSON; the
// next start re-encodes it in place.
func TestGetOrCreateDefaultClientTools_RepairsRawSpec(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, GetOrCreateDefaultClientTools(db))

	raw := `{"parameters":null,"ui":{"kind":"present","title":"Generative UI"}}`
	require.NoError(t, db.Model(&Tool{}).Where("tool_type = ?", ToolTypeClient).UpdateColumn("oas_spec", raw).Error)

	require.NoError(t, GetOrCreateDefaultClientTools(db))

	var tool Tool
	require.NoError(t, db.Where("tool_type = ?", ToolTypeClient).First(&tool).Error)
	decoded, err := base64.StdEncoding.DecodeString(tool.OASSpec)
	require.NoError(t, err)
	assert.JSONEq(t, raw, string(decoded))
	assert.Equal(t, DefaultPresentToolName, tool.Name)

	def, err := tool.ClientDefinition()
	require.NoError(t, err)
	assert.Equal(t, ClientToolKindPresent, def.UI.Kind)
}

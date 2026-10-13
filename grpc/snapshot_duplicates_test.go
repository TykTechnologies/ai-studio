package grpc

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Duplicates stored before the hub refused them (written here without the
// model hooks) do not break snapshot generation: the snapshot keeps the
// lowest ID per slug, leaves the others out with their App grants, and names
// them in omitted_entries, which the checksum covers.
func TestSnapshotLeavesOutStoredDuplicates(t *testing.T) {
	server, db := setupTestServer(t, nil)
	raw := db.Session(&gorm.Session{SkipHooks: true})

	tool1 := &models.Tool{Name: "Page fetcher", Slug: "page-fetcher", ToolType: models.ToolTypeREST, Active: true}
	tool2 := &models.Tool{Name: "page fetcher", Slug: "page-fetcher", ToolType: models.ToolTypeREST, Active: true}
	tool3 := &models.Tool{Name: "Other", Slug: "other", ToolType: models.ToolTypeREST, Active: true}
	for _, tool := range []*models.Tool{tool1, tool2, tool3} {
		require.NoError(t, raw.Create(tool).Error)
	}
	llm1 := &models.LLM{Name: "GPT 4", Vendor: models.OPENAI, APIKey: "k1", Active: true}
	llm2 := &models.LLM{Name: "gpt-4", Vendor: models.OPENAI, APIKey: "k2", Active: true}
	for _, llm := range []*models.LLM{llm1, llm2} {
		require.NoError(t, raw.Create(llm).Error)
	}
	app := &models.App{Name: "app", IsActive: true}
	require.NoError(t, db.Create(app).Error)
	for _, toolID := range []uint{tool1.ID, tool2.ID, tool3.ID} {
		require.NoError(t, db.Exec("INSERT INTO app_tools (app_id, tool_id) VALUES (?, ?)", app.ID, toolID).Error)
	}
	for _, llmID := range []uint{llm1.ID, llm2.ID} {
		require.NoError(t, db.Exec("INSERT INTO app_llms (app_id, llm_id) VALUES (?, ?)", app.ID, llmID).Error)
	}

	snap, err := server.getConfigurationSnapshot("")
	require.NoError(t, err)

	var toolIDs []uint32
	for _, tool := range snap.Tools {
		toolIDs = append(toolIDs, tool.Id)
	}
	assert.ElementsMatch(t, []uint32{uint32(tool1.ID), uint32(tool3.ID)}, toolIDs)
	require.Len(t, snap.Llms, 1)
	assert.Equal(t, uint32(llm1.ID), snap.Llms[0].Id)
	assert.Len(t, snap.OmittedEntries, 2)
	assert.Contains(t, snap.OmittedEntries[0], "LLM")
	assert.Contains(t, snap.OmittedEntries[1], `slug "page-fetcher" is already used by tool`)

	for _, a := range snap.Apps {
		if a.Id == uint32(app.ID) {
			assert.NotContains(t, a.ToolIds, uint32(tool2.ID))
			assert.NotContains(t, a.LlmIds, uint32(llm2.ID))
		}
	}

	require.NotEmpty(t, snap.Checksum)
	again, err := server.getConfigurationSnapshot("")
	require.NoError(t, err)
	assert.Equal(t, snap.Checksum, again.Checksum, "the checksum is stable")

	// Renaming the duplicate away puts it back and changes the checksum.
	require.NoError(t, db.Model(&models.Tool{}).Where("id = ?", tool2.ID).Updates(map[string]interface{}{"name": "Page fetcher 2", "slug": "page-fetcher-2"}).Error)
	fixed, err := server.getConfigurationSnapshot("")
	require.NoError(t, err)
	assert.Len(t, fixed.Tools, 3)
	assert.Len(t, fixed.OmittedEntries, 1)
	assert.NotEqual(t, snap.Checksum, fixed.Checksum)
}

package grpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snapshotHas[T interface{ GetId() uint32 }](items []T, id uint) bool {
	for _, it := range items {
		if it.GetId() == uint32(id) {
			return true
		}
	}
	return false
}

// A deleted filter keeps its tool_filters rows but filters nothing: the
// tool is not listed with it, and keeps being sent.
func TestSnapshotDeletedToolFilterKeepsTheTool(t *testing.T) {
	server, db := setupTestServer(t, nil)
	filter := &models.Filter{Name: "gone", Script: []byte("output = {block: false}")}
	require.NoError(t, db.Create(filter).Error)
	tool := &models.Tool{Name: "Fetcher", ToolType: models.ToolTypeREST, Active: true, Filters: []models.Filter{*filter}}
	require.NoError(t, db.Create(tool).Error)
	require.NoError(t, db.Delete(filter).Error)

	snap, err := server.getConfigurationSnapshot("")
	require.NoError(t, err)
	require.True(t, snapshotHas(snap.Tools, tool.ID), "omitted: %v", snap.OmittedEntries)
	for _, tc := range snap.Tools {
		if tc.Id == uint32(tool.ID) {
			assert.Empty(t, tc.FilterIds)
		}
	}
	assert.Empty(t, snap.OmittedEntries)
}

// A snapshot whose plugin attachments cannot be read is not sent: every LLM
// would go out without its plugins. A registering edge then gets no initial
// configuration (it keeps what it has) rather than an empty one.
func TestSnapshotFailsWhenAttachmentsCannotBeRead(t *testing.T) {
	server, db := setupTestServer(t, nil)
	createTestLLMs(db, "")
	require.NoError(t, db.Exec("DROP TABLE llm_plugins").Error)

	_, err := server.getConfigurationSnapshot("")
	require.Error(t, err)

	resp, err := server.RegisterEdge(context.Background(), &pb.EdgeRegistrationRequest{EdgeId: "edge-x", EdgeNamespace: "", Version: "1"})
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Nil(t, resp.InitialConfig, "no empty configuration that would retire the edge's own")
}

// A global LLM running a plugin filed under namespace "eu", and a global
// tool authenticated by it: edges of "eu" get both with the plugin. Other
// edges do not get that namespace's plugin, so they get neither: the LLM
// does not run without its plugin, and the tool does not fall back to App
// keys.
func TestSnapshotPluginFromAnotherNamespaceFailsClosed(t *testing.T) {
	server, db := setupTestServer(t, nil)
	plugin := &models.Plugin{Name: "eu guard", Command: "file:///bin/guard", HookType: models.HookTypePostAuth, IsActive: true, Namespace: "eu"}
	require.NoError(t, db.Create(plugin).Error)
	llm := &models.LLM{Name: "Global LLM", Vendor: models.OPENAI, APIKey: "k", Active: true}
	require.NoError(t, db.Create(llm).Error)
	require.NoError(t, db.Create(&models.LLMPlugin{LLMID: llm.ID, PluginID: plugin.ID, IsActive: true}).Error)
	tool := &models.Tool{Name: "Global tool", ToolType: models.ToolTypeREST, Active: true}
	require.NoError(t, db.Create(tool).Error)
	require.NoError(t, db.Create(&models.EndpointAuthPlugin{ObjectType: models.EndpointTypeTool, ObjectID: tool.ID, PluginID: plugin.ID}).Error)

	eu, err := server.getConfigurationSnapshot("eu")
	require.NoError(t, err)
	assert.True(t, snapshotHas(eu.Plugins, plugin.ID))
	require.True(t, snapshotHas(eu.Llms, llm.ID), "omitted: %v", eu.OmittedEntries)
	assert.True(t, snapshotHas(eu.Tools, tool.ID))
	for _, l := range eu.Llms {
		if l.Id == uint32(llm.ID) {
			assert.Equal(t, []uint32{uint32(plugin.ID)}, l.PluginIds)
		}
	}

	for _, ns := range []string{"", "us"} {
		other, err := server.getConfigurationSnapshot(ns)
		require.NoError(t, err)
		assert.False(t, snapshotHas(other.Plugins, plugin.ID), "namespace %q: another namespace's plugin is not sent", ns)
		assert.False(t, snapshotHas(other.Llms, llm.ID), "namespace %q: the LLM does not run without its plugin", ns)
		assert.False(t, snapshotHas(other.Tools, tool.ID), "namespace %q: the tool does not fall back to App keys", ns)
		assert.Contains(t, other.OmittedEntries, fmt.Sprintf("LLM %d left out: its plugin %d is not in the configuration, and it does not run without it", llm.ID, plugin.ID))
	}

	// A deactivated plugin drops off, as before: the LLM and the tool are
	// sent everywhere without it.
	require.NoError(t, db.Model(&models.Plugin{}).Where("id = ?", plugin.ID).Update("is_active", false).Error)
	us, err := server.getConfigurationSnapshot("us")
	require.NoError(t, err)
	assert.True(t, snapshotHas(us.Llms, llm.ID))
	assert.True(t, snapshotHas(us.Tools, tool.ID))
}

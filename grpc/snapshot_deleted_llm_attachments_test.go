package grpc

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A deleted LLM keeps its llm_plugins and llm_filters rows on the hub. The
// snapshot attaches plugins and filters only to the LLMs it carries: sent
// on, those rows made an edge store attachments to an LLM it never has, and
// its next sync into the same database failed (midsommar-clean-26ut).
func TestSnapshotLeavesOutAttachmentsOfDeletedLLMs(t *testing.T) {
	server, db := setupTestServer(t, nil)
	for _, namespace := range []string{"", "team-a"} {
		llms := createTestLLMs(db, namespace)
		live, gone := llms[0], llms[1]

		plugin := &models.Plugin{Name: "audit-" + namespace, Command: "/bin/audit", HookType: models.HookTypePostAuth, IsActive: true}
		require.NoError(t, db.Create(plugin).Error)
		require.NoError(t, models.UpdatePluginOrder(db, live.ID, []uint{plugin.ID}))
		require.NoError(t, models.UpdatePluginOrder(db, gone.ID, []uint{plugin.ID}))

		filter := &models.Filter{Name: "pii-" + namespace, Script: []byte("x")}
		require.NoError(t, db.Create(filter).Error)
		require.NoError(t, db.Exec("INSERT INTO llm_filters (llm_id, filter_id) VALUES (?, ?), (?, ?)", live.ID, filter.ID, gone.ID, filter.ID).Error)

		require.NoError(t, db.Delete(&models.LLM{}, gone.ID).Error, "soft delete; its attachments stay")

		snapshot, err := server.getConfigurationSnapshot(namespace)
		require.NoError(t, err)
		foundPlugin, foundFilter := false, false
		for _, p := range snapshot.Plugins {
			if p.Id == uint32(plugin.ID) {
				foundPlugin = true
				assert.Equal(t, []uint32{uint32(live.ID)}, p.LlmIds, "namespace %q", namespace)
			}
		}
		for _, f := range snapshot.Filters {
			if f.Id == uint32(filter.ID) {
				foundFilter = true
				assert.Equal(t, []uint32{uint32(live.ID)}, f.LlmIds, "namespace %q", namespace)
			}
		}
		assert.True(t, foundPlugin && foundFilter, "namespace %q: the plugin and the filter are in the snapshot", namespace)
	}
}

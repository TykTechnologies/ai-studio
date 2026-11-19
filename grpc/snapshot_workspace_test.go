package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Workspaces are an ownership stamp on the hub, not configuration: stamping
// every object (the upgrade backfill) or moving one to another Workspace
// leaves the configuration snapshot and its checksum exactly as they were,
// so an upgrade marks no edge as pending (midsommar-clean-oztk.2).
func TestSnapshotChecksum_UnchangedByWorkspaceStamp(t *testing.T) {
	server, db := setupTestServer(t, nil)

	llm := &models.LLM{Name: "Stamp LLM", Vendor: models.OPENAI, APIEndpoint: "https://x", Active: true}
	require.NoError(t, db.Create(llm).Error)
	app := &models.App{Name: "Stamp App", IsActive: true}
	require.NoError(t, db.Create(app).Error)
	tool := &models.Tool{Name: "Stamp tool", Slug: "stamp-tool", ToolType: models.ToolTypeREST, Active: true}
	require.NoError(t, db.Create(tool).Error)
	ds := &models.Datasource{Name: "Stamp datasource", Active: true, DBSourceType: "pinecone"}
	require.NoError(t, db.Create(ds).Error)
	filter := &models.Filter{Name: "Stamp filter", Kind: models.FilterKindGuardrail}
	require.NoError(t, db.Create(filter).Error)
	// As an older build left them: no Workspace.
	for _, m := range []interface{}{llm, app, tool, ds, filter} {
		require.NoError(t, db.Model(m).UpdateColumn("workspace_id", 0).Error)
	}

	checksum := func() string {
		t.Helper()
		snapshot, err := server.getConfigurationSnapshot("")
		require.NoError(t, err)
		require.NotEmpty(t, snapshot.Llms)
		require.NotEmpty(t, snapshot.Apps)
		// The checksum edges are compared against (secret ciphertexts
		// normalized to digests of their plaintext).
		require.NotEmpty(t, snapshot.Checksum)
		return snapshot.Checksum
	}
	before := checksum()
	require.Equal(t, before, checksum(), "the checksum is stable")

	_, err := models.EnsureDefaultWorkspace(db)
	require.NoError(t, err)
	require.NoError(t, models.BackfillWorkspaceIDs(db))
	assert.Equal(t, before, checksum(), "the upgrade backfill leaves the checksum unchanged")

	other := &models.Workspace{Name: "Elsewhere"}
	require.NoError(t, db.Create(other).Error)
	for _, m := range []interface{}{llm, app, tool, ds, filter} {
		require.NoError(t, db.Model(m).UpdateColumn("workspace_id", other.ID).Error)
	}
	assert.Equal(t, before, checksum(), "moving objects between Workspaces leaves the checksum unchanged")
}

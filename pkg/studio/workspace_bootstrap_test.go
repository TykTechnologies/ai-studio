//go:build !enterprise

package studio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// A fresh install ends with one Default workspace and every row of the
// stamped tables in it, including the defaults seeded after InitModels
// (catalogues, LLMs).
func TestFreshInstallStampsSeededDefaultsWithTheDefaultWorkspace(t *testing.T) {
	opts := newTestOptions(t)
	opts.SkipLLMDefaults = false
	s, err := New(opts)
	require.NoError(t, err)
	defer stopStudio(t, s)

	var workspaces []models.Workspace
	require.NoError(t, opts.DB.Find(&workspaces).Error)
	require.Len(t, workspaces, 1)
	require.True(t, workspaces[0].IsDefault)

	seeded := 0
	for _, m := range models.WorkspaceStampedModels() {
		stmt := &gorm.Statement{DB: opts.DB}
		require.NoError(t, stmt.Parse(m))
		var ids []uint
		require.NoError(t, opts.DB.Table(stmt.Schema.Table).Pluck("workspace_id", &ids).Error)
		seeded += len(ids)
		for _, id := range ids {
			assert.Equal(t, workspaces[0].ID, id, "a %s row is in the Default workspace", stmt.Schema.Table)
		}
	}
	for _, m := range []interface{}{&models.LLM{}, &models.Catalogue{}} {
		var n int64
		require.NoError(t, opts.DB.Model(m).Count(&n).Error)
		assert.NotZero(t, n, "%T defaults were seeded", m)
	}
	assert.NotZero(t, seeded)
}

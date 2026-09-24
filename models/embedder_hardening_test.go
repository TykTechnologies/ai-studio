package models

import (
	"errors"
	"testing"

	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Rows the migration does not move still lose their keys: soft-deleted
// datasources, and settings without a vendor. Neither gets an embedder.
func TestMigrateEmbedders_ClearsStaleColumns(t *testing.T) {
	db := setupEmbedderTestDB(t)
	withLegacyEmbedColumns(t, db)

	deleted := insertLegacyDatasource(t, db, legacyDS{name: "Deleted", vendor: "openai", key: "sk-deleted", model: "m"})
	require.NoError(t, db.Exec("UPDATE datasources SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?", deleted).Error)
	vendorless := insertLegacyDatasource(t, db, legacyDS{name: "Vendorless", url: "https://x", key: "sk-orphan", model: "m"})
	live := insertLegacyDatasource(t, db, legacyDS{name: "Live", vendor: "openai", key: "sk-live", model: "m"})

	require.NoError(t, MigrateEmbedders(db))

	for _, id := range []uint{deleted, vendorless, live} {
		for col, v := range legacyColumnsOf(t, db, id) {
			assert.Equal(t, "", v, "column %s of datasource %d should be cleared", col, id)
		}
	}
	var linked []uint
	require.NoError(t, db.Table("datasources").Where("embedder_id IS NOT NULL").Pluck("id", &linked).Error)
	assert.Equal(t, []uint{live}, linked, "only the live datasource with a vendor gets an embedder")
	var n int64
	require.NoError(t, db.Model(&Embedder{}).Count(&n).Error)
	assert.EqualValues(t, 1, n)
}

// A router whose embedding LLM was deleted is left as it was (it could not
// embed either way) and does not block the rest of the migration.
func TestMigrateEmbedders_RouterWithDeletedLLMIsSkipped(t *testing.T) {
	db := setupEmbedderTestDB(t)
	route, embedLLM := routerFixtures(t, db)
	gone := &LLM{Name: "Gone", Vendor: OPENAI, Active: true}
	require.NoError(t, db.Create(gone).Error)

	orphan := routerWithExamples("orphan", route.ID)
	orphan.Settings.Embedding = &sr.ModelRef{LLMID: gone.ID, Model: "m"}
	fine := routerWithExamples("fine", route.ID)
	fine.Settings.Embedding = &sr.ModelRef{LLMID: embedLLM.ID, Model: "m"}
	require.NoError(t, orphan.Create(db))
	require.NoError(t, fine.Create(db))
	require.NoError(t, db.Delete(gone).Error)

	require.NoError(t, MigrateEmbedders(db))

	var got SemanticRouter
	require.NoError(t, db.First(&got, orphan.ID).Error)
	assert.Nil(t, got.EmbedderID)
	assert.Equal(t, &sr.ModelRef{LLMID: gone.ID, Model: "m"}, got.Settings.Embedding, "left untouched")
	var migrated SemanticRouter
	require.NoError(t, db.First(&migrated, fine.ID).Error)
	assert.NotNil(t, migrated.EmbedderID, "the other router is still migrated")
}

// The migration is all or nothing: a failure part way leaves every
// datasource as it was (and Studio's startup reports the error).
func TestMigrateEmbedders_FailureRollsBackEverything(t *testing.T) {
	db := setupEmbedderTestDB(t)
	withLegacyEmbedColumns(t, db)
	a := insertLegacyDatasource(t, db, legacyDS{name: "A", vendor: "openai", key: "k1", model: "m"})
	b := insertLegacyDatasource(t, db, legacyDS{name: "B", vendor: "ollama", key: "k2", model: "n"})

	creates := 0
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail_second_embedder", func(tx *gorm.DB) {
		if tx.Statement.Table == "embedders" {
			creates++
			if creates == 2 {
				_ = tx.AddError(errors.New("disk full"))
			}
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove("test:fail_second_embedder") })

	err := MigrateEmbedders(db)
	require.ErrorContains(t, err, "disk full")

	var n int64
	require.NoError(t, db.Model(&Embedder{}).Count(&n).Error)
	assert.Zero(t, n, "no embedder survives a failed migration")
	assert.Equal(t, "k1", legacyColumnsOf(t, db, a)["embed_api_key"])
	assert.Equal(t, "k2", legacyColumnsOf(t, db, b)["embed_api_key"])

	require.NoError(t, db.Callback().Create().Remove("test:fail_second_embedder"))
	require.NoError(t, MigrateEmbedders(db), "the next start migrates normally")
	require.NoError(t, db.Model(&Embedder{}).Count(&n).Error)
	assert.EqualValues(t, 2, n)
}

func TestEmbedder_UsableInNamespace(t *testing.T) {
	id := uint(1)
	linked := func(ns string) *Embedder {
		return &Embedder{LLMID: &id, LLM: &LLM{Name: "L", Namespace: ns}, ModelName: "m"}
	}
	assert.NoError(t, linked("").UsableInNamespace("eu"), "a global LLM serves every namespace")
	assert.NoError(t, linked("global").UsableInNamespace(""))
	assert.NoError(t, linked("eu").UsableInNamespace("eu"))
	assert.ErrorIs(t, linked("eu").UsableInNamespace(""), ErrEmbedderNamespace, "a global datasource reaches every edge")
	assert.ErrorIs(t, linked("eu").UsableInNamespace("us"), ErrEmbedderNamespace)
	assert.NoError(t, (&Embedder{Vendor: OPENAI, ModelName: "m"}).UsableInNamespace("eu"), "standalone embedders have no namespace")
}

func TestIsUniqueViolation(t *testing.T) {
	db := setupEmbedderTestDB(t)
	require.NoError(t, (&Embedder{Name: "dup", Vendor: OPENAI, ModelName: "m"}).Create(db))
	err := (&Embedder{Name: "dup", Vendor: OPENAI, ModelName: "m"}).Create(db)
	assert.True(t, IsUniqueViolation(err))
	assert.False(t, IsUniqueViolation(errors.New("other")))
	assert.False(t, IsUniqueViolation(nil))

	// CreateWithDefaultName steps past a taken name.
	e := &Embedder{Vendor: OPENAI, ModelName: "m"}
	require.NoError(t, (&Embedder{Name: "openai · m", Vendor: OPENAI, ModelName: "m"}).Create(db))
	require.NoError(t, CreateWithDefaultName(db, e, "openai", "m"))
	assert.Equal(t, "openai · m (2)", e.Name)
}

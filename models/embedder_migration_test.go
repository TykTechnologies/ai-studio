package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// withLegacyEmbedColumns gives a fresh database the datasource columns
// Embedders replaced, as an install upgraded from before them has.
func withLegacyEmbedColumns(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, c := range legacyEmbedColumns {
		require.NoError(t, db.Exec("ALTER TABLE datasources ADD COLUMN "+c+" TEXT DEFAULT ''").Error)
	}
}

type legacyDS struct {
	name, vendor, url, key, model, conn, connKey string
	privacy                                      int
}

func insertLegacyDatasource(t *testing.T, db *gorm.DB, d legacyDS) uint {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO datasources
		(name, privacy_score, db_conn_string, db_conn_api_key, embed_vendor, embed_url, embed_api_key, embed_model, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		d.name, d.privacy, d.conn, d.connKey, d.vendor, d.url, d.key, d.model).Error)
	var id uint
	require.NoError(t, db.Raw("SELECT id FROM datasources WHERE name = ?", d.name).Scan(&id).Error)
	return id
}

func legacyColumnsOf(t *testing.T, db *gorm.DB, id uint) map[string]interface{} {
	t.Helper()
	row := map[string]interface{}{}
	require.NoError(t, db.Table("datasources").Select("embed_vendor, embed_url, embed_api_key, embed_model").Where("id = ?", id).Take(&row).Error)
	return row
}

func embedderOf(t *testing.T, db *gorm.DB, dsID uint) *Embedder {
	t.Helper()
	var ds Datasource
	require.NoError(t, ds.Get(db, dsID))
	require.NotNil(t, ds.EmbedderID, "datasource %d was not linked", dsID)
	require.NotNil(t, ds.Embedder)
	return ds.Embedder
}

func TestMigrateEmbedders(t *testing.T) {
	db := setupEmbedderTestDB(t)
	withLegacyEmbedColumns(t, db)

	a := insertLegacyDatasource(t, db, legacyDS{name: "A", vendor: "openai", url: "https://api.openai.com/v1", key: "$SECRET/OPENAI", model: "text-embedding-3-small", privacy: 30})
	b := insertLegacyDatasource(t, db, legacyDS{name: "B", vendor: "openai", url: "https://api.openai.com/v1", key: "$SECRET/OPENAI", model: "text-embedding-3-small", privacy: 80})
	c := insertLegacyDatasource(t, db, legacyDS{name: "C", vendor: "openai", url: "https://api.openai.com/v1", key: "$SECRET/OPENAI", model: "text-embedding-3-small", privacy: 10})
	otherKey := insertLegacyDatasource(t, db, legacyDS{name: "D", vendor: "openai", url: "https://api.openai.com/v1", key: "sk-other", model: "text-embedding-3-small"})
	otherModel := insertLegacyDatasource(t, db, legacyDS{name: "E", vendor: "openai", url: "https://api.openai.com/v1", key: "$SECRET/OPENAI", model: "text-embedding-3-large"})
	vertex := insertLegacyDatasource(t, db, legacyDS{name: "V", vendor: "vertex", model: "text-embedding-004", conn: "my-project:us-central1", connKey: "vertex-key"})
	noModel := insertLegacyDatasource(t, db, legacyDS{name: "N", vendor: "ollama", url: "http://ollama:11434"})
	noEmbed := insertLegacyDatasource(t, db, legacyDS{name: "Bare"})

	require.NoError(t, MigrateEmbedders(db))

	shared := embedderOf(t, db, a)
	assert.Equal(t, shared.ID, embedderOf(t, db, b).ID, "identical settings share one embedder")
	assert.Equal(t, shared.ID, embedderOf(t, db, c).ID)
	assert.Equal(t, OPENAI, shared.Vendor)
	assert.Equal(t, "https://api.openai.com/v1", shared.Endpoint)
	assert.Equal(t, "$SECRET/OPENAI", shared.APIKey, "secret references are kept verbatim")
	assert.Equal(t, "text-embedding-3-small", shared.ModelName)
	assert.Equal(t, 80, shared.PrivacyScore, "a shared embedder takes the group's highest privacy score")
	assert.False(t, shared.IsLinked())
	assert.Equal(t, "openai · text-embedding-3-small", shared.Name)

	assert.NotEqual(t, shared.ID, embedderOf(t, db, otherKey).ID, "a different key is a different embedder")
	byModel := embedderOf(t, db, otherModel)
	assert.NotEqual(t, shared.ID, byModel.ID, "a different model is a different embedder")

	v := embedderOf(t, db, vertex)
	assert.Equal(t, "my-project:us-central1", v.Endpoint, "Vertex took project:location from the vector store column")
	assert.Equal(t, "vertex-key", v.APIKey)

	n := embedderOf(t, db, noModel)
	assert.Equal(t, "", n.ModelName, "a datasource without a model is still linked")

	var bare Datasource
	require.NoError(t, bare.Get(db, noEmbed))
	assert.Nil(t, bare.EmbedderID, "a datasource without embedding settings gets no embedder")

	for _, id := range []uint{a, b, c, otherKey, otherModel, vertex, noModel} {
		for col, v := range legacyColumnsOf(t, db, id) {
			assert.Equal(t, "", v, "column %s of datasource %d should be cleared", col, id)
		}
	}

	var count int64
	require.NoError(t, db.Model(&Embedder{}).Count(&count).Error)
	assert.EqualValues(t, 5, count)

	t.Run("second run is a no-op", func(t *testing.T) {
		require.NoError(t, MigrateEmbedders(db))
		var again int64
		require.NoError(t, db.Model(&Embedder{}).Count(&again).Error)
		assert.Equal(t, count, again)
	})

	t.Run("names stay unique", func(t *testing.T) {
		d2 := insertLegacyDatasource(t, db, legacyDS{name: "F", vendor: "openai", url: "https://other", model: "text-embedding-3-small"})
		require.NoError(t, MigrateEmbedders(db))
		// "(2)" went to datasource D's embedder in the first run.
		assert.Equal(t, "openai · text-embedding-3-small (3)", embedderOf(t, db, d2).Name)
	})
}

// A database created after the columns went away has nothing to migrate.
func TestMigrateEmbedders_NoLegacyColumns(t *testing.T) {
	db := setupEmbedderTestDB(t)
	assert.False(t, db.Migrator().HasColumn("datasources", "embed_vendor"))
	assert.NoError(t, MigrateEmbedders(db))
}

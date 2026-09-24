package models

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// setupIsolatedPostgres opens DATABASE_URL in a fresh schema (dropped after
// the test), so the migration sees only its own rows. Skipped unless
// DATABASE_URL is set, like the other Postgres tests.
func setupIsolatedPostgres(t *testing.T) (*gorm.DB, func() *gorm.DB) {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set - skipping PostgreSQL tests")
	}
	admin, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		t.Skipf("Failed to connect to PostgreSQL: %v", err)
	}
	schema := fmt.Sprintf("emb_mig_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE") })

	sep := "?"
	if strings.Contains(dbURL, "?") {
		sep = "&"
	}
	open := func() *gorm.DB {
		db, err := gorm.Open(postgres.Open(dbURL+sep+"search_path="+schema), &gorm.Config{})
		require.NoError(t, err)
		return db
	}
	db := open()
	require.NoError(t, InitModels(db))
	return db, open
}

// Two replicas starting together must not both migrate the same rows: the
// advisory lock serialises them, and the second finds nothing left to do.
func TestMigrateEmbedders_ConcurrentReplicas_PostgreSQL(t *testing.T) {
	db, open := setupIsolatedPostgres(t)
	withLegacyEmbedColumns(t, db)
	for i := 0; i < 20; i++ {
		insertLegacyDatasource(t, db, legacyDS{name: fmt.Sprintf("DS%02d", i), vendor: "openai", url: "https://api.openai.com/v1", key: "k", model: "text-embedding-3-small", privacy: i})
	}
	insertLegacyDatasource(t, db, legacyDS{name: "Other", vendor: "ollama", url: "http://ollama:11434", model: "nomic-embed-text"})

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = MigrateEmbedders(open())
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		assert.NoError(t, err)
	}

	var embedders int64
	require.NoError(t, db.Model(&Embedder{}).Count(&embedders).Error)
	assert.EqualValues(t, 2, embedders, "one embedder per distinct configuration, however many replicas ran")

	var unlinked int64
	require.NoError(t, db.Table("datasources").Where("embedder_id IS NULL").Count(&unlinked).Error)
	assert.Zero(t, unlinked)

	var shared Embedder
	require.NoError(t, db.Where("vendor = ?", "openai").First(&shared).Error)
	assert.Equal(t, 19, shared.PrivacyScore)
}

// The router privacy score is a correlated subquery over a derived table;
// check it runs on Postgres as it does on SQLite.
func TestSemanticRouterPrivacy_StandaloneEmbedder_PostgreSQL(t *testing.T) {
	db, _ := setupIsolatedPostgres(t)
	route, _ := routerFixtures(t, db)
	e := &Embedder{Name: "standalone", Vendor: OLLAMA, ModelName: "nomic", PrivacyScore: 25}
	require.NoError(t, e.Create(db))
	r := routerWithExamples("pg", route.ID)
	r.EmbedderID = &e.ID
	require.NoError(t, r.Create(db))
	plain := routerWithExamples("pg-plain", route.ID)
	require.NoError(t, plain.Create(db))

	scores, err := SemanticRouterPrivacyScores(db, []uint{r.ID, plain.ID})
	require.NoError(t, err)
	assert.Equal(t, 25, scores[r.ID])
	assert.Equal(t, 80, scores[plain.ID])
}

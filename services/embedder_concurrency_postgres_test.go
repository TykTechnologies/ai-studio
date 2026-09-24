package services

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// setupIsolatedPostgresService opens DATABASE_URL in a fresh schema (dropped
// afterwards) with enough connections for real concurrency. Skipped unless
// DATABASE_URL is set.
func setupIsolatedPostgresService(t *testing.T) *Service {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set - skipping PostgreSQL tests")
	}
	admin, err := gorm.Open(postgres.Open(url), &gorm.Config{})
	if err != nil {
		t.Skipf("Failed to connect to PostgreSQL: %v", err)
	}
	schema := fmt.Sprintf("svc_emb_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE") })
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	db, err := gorm.Open(postgres.Open(url+sep+"search_path="+schema), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(40)
	require.NoError(t, models.InitModels(db))
	return NewService(db)
}

// runConcurrently starts n calls at once and returns their errors.
func runConcurrently(n int, f func(i int) error) []error {
	start := make(chan struct{})
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = f(i)
		}(i)
	}
	close(start)
	wg.Wait()
	return errs
}

// Datasources created at the same moment with the same legacy embedding
// settings (parallel SDK plugins, simultaneous approvals) share one embedder
// and none of them fails.
func TestEmbedderFindOrCreate_ConcurrentIdentical_Postgres(t *testing.T) {
	s := setupIsolatedPostgresService(t)
	errs := runConcurrently(40, func(i int) error {
		_, err := s.CreateDatasource(fmt.Sprintf("DS%02d", i), "", "", "", "", 10, 1, nil, "", "pgvector", "", "db",
			legacy("openai", "https://api.openai.com/v1", "sk", "text-embedding-3-small"), true)
		return err
	})
	for _, err := range errs {
		assert.NoError(t, err)
	}
	var n int64
	require.NoError(t, s.DB.Model(&models.Embedder{}).Count(&n).Error)
	assert.EqualValues(t, 1, n)
}

// Different configurations that share a default name ("openai · m") all get
// created, with distinct names, even when they race for the same name.
func TestEmbedderFindOrCreate_ConcurrentSameName_Postgres(t *testing.T) {
	s := setupIsolatedPostgresService(t)
	errs := runConcurrently(12, func(i int) error {
		_, err := s.CreateDatasource(fmt.Sprintf("DS%02d", i), "", "", "", "", 10, 1, nil, "", "pgvector", "", "db",
			legacy("openai", "https://x", fmt.Sprintf("sk-%d", i), "m"), true)
		return err
	})
	for _, err := range errs {
		assert.NoError(t, err)
	}
	var names []string
	require.NoError(t, s.DB.Model(&models.Embedder{}).Pluck("name", &names).Error)
	assert.Len(t, names, 12)
}

// Routers naming the same LLM and model at once share one linked embedder.
func TestFindOrCreateLinkedEmbedder_Concurrent_Postgres(t *testing.T) {
	s := setupIsolatedPostgresService(t)
	llm := &models.LLM{Name: "OpenAI", Vendor: models.OPENAI, Active: true}
	require.NoError(t, s.DB.Create(llm).Error)
	errs := runConcurrently(20, func(int) error {
		_, err := s.FindOrCreateLinkedEmbedder(llm.ID, "text-embedding-3-small", 1)
		return err
	})
	for _, err := range errs {
		assert.NoError(t, err)
	}
	var n int64
	require.NoError(t, s.DB.Model(&models.Embedder{}).Count(&n).Error)
	assert.EqualValues(t, 1, n)
}

package services

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// These tests exist because the rest of this package cannot catch the bug they
// cover. Every other services test opens sqlite ":memory:", which serialises
// all work onto one connection -- so a function that writes on a caller's
// transaction and then reaches for s.DB looks perfectly correct there.
//
// On Postgres those are two connections. The second one's INSERT waits on the
// row the first has not committed, and the first cannot commit because the
// goroutine driving it is blocked on the second. Approving any submission hung
// forever, holding an open transaction, while this package stayed green.
//
// Skipped unless DATABASE_URL is set, matching models/user_postgres_test.go.

func setupPostgresServiceTest(t *testing.T) *Service {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set - skipping PostgreSQL transaction tests")
	}

	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		t.Skipf("Failed to connect to PostgreSQL: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get SQL database: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("PostgreSQL not accessible: %v", err)
	}

	// More than one connection must be available, or the pool itself would
	// serialise the calls and hide exactly what this is testing.
	sqlDB.SetMaxOpenConns(5)

	if err := models.InitModels(db); err != nil {
		t.Fatalf("Failed to init models: %v", err)
	}

	return NewService(db)
}

// runWithinDeadline runs fn and fails the test if it has not returned in time.
//
// A regression here does not return an error, it stops responding, so an
// ordinary assertion would hang the whole package until the go test timeout.
func runWithinDeadline(t *testing.T, what string, fn func() error) error {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- fn() }()

	select {
	case err := <-done:
		return err
	case <-time.After(20 * time.Second):
		t.Fatalf("%s did not return within 20s: it is deadlocked against the caller's own "+
			"transaction, which means something on this path is using s.DB instead of the "+
			"db handed to it", what)
		return nil
	}
}

func TestCreateToolWithDB_InTransaction_DoesNotDeadlock(t *testing.T) {
	s := setupPostgresServiceTest(t)

	name := fmt.Sprintf("PG Tx Test Tool %d", time.Now().UnixNano())
	t.Cleanup(func() {
		s.DB.Exec("DELETE FROM tool_catalogue_tools WHERE tool_id IN (SELECT id FROM tools WHERE name = ?)", name)
		s.DB.Exec("DELETE FROM tools WHERE name = ?", name)
	})

	tx := s.DB.Begin()
	assert.NoError(t, tx.Error)

	var tool *models.Tool
	err := runWithinDeadline(t, "CreateToolWithDB inside a transaction", func() error {
		var createErr error
		tool, createErr = s.CreateToolWithDB(tx, name, "created inside a transaction", "openapi", "", 3, "", "")
		return createErr
	})
	if err != nil {
		tx.Rollback()
		t.Fatalf("CreateToolWithDB returned an error: %v", err)
	}

	assert.NoError(t, tx.Commit().Error, "the transaction must still be committable")

	// The tool is published into Default, which is what the auto-assignment is
	// for -- proving the call did the work rather than merely not hanging.
	var count int64
	assert.NoError(t, s.DB.Table("tool_catalogue_tools").Where("tool_id = ?", tool.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "tool should be in exactly one (Default) catalogue")
}

func TestCreateDatasourceWithDB_InTransaction_DoesNotDeadlock(t *testing.T) {
	s := setupPostgresServiceTest(t)

	name := fmt.Sprintf("PG Tx Test Datasource %d", time.Now().UnixNano())
	t.Cleanup(func() {
		s.DB.Exec("DELETE FROM data_catalogue_data_sources WHERE datasource_id IN (SELECT id FROM datasources WHERE name = ?)", name)
		s.DB.Exec("DELETE FROM datasources WHERE name = ?", name)
	})

	tx := s.DB.Begin()
	assert.NoError(t, tx.Error)

	var ds *models.Datasource
	err := runWithinDeadline(t, "CreateDatasourceWithDB inside a transaction", func() error {
		var createErr error
		ds, createErr = s.CreateDatasourceWithDB(tx, name, "short", "long", "", "https://example.com",
			3, 0, nil, "", "", "", "", "", "", "", "", false)
		return createErr
	})
	if err != nil {
		tx.Rollback()
		t.Fatalf("CreateDatasourceWithDB returned an error: %v", err)
	}

	assert.NoError(t, tx.Commit().Error, "the transaction must still be committable")

	var count int64
	assert.NoError(t, s.DB.Table("data_catalogue_data_sources").Where("datasource_id = ?", ds.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "datasource should be in exactly one (Default) catalogue")
}

// A second call must not add the resource to Default twice. The guard that
// prevents this used to ask for an association models.Tool does not declare,
// so it always reported zero memberships and appended unconditionally.
func TestEnsureInDefaultCatalogue_IsIdempotent(t *testing.T) {
	s := setupPostgresServiceTest(t)

	name := fmt.Sprintf("PG Tx Test Tool Idem %d", time.Now().UnixNano())
	t.Cleanup(func() {
		s.DB.Exec("DELETE FROM tool_catalogue_tools WHERE tool_id IN (SELECT id FROM tools WHERE name = ?)", name)
		s.DB.Exec("DELETE FROM tools WHERE name = ?", name)
	})

	tool, err := s.CreateToolWithDB(s.DB, name, "idempotency check", "openapi", "", 3, "", "")
	assert.NoError(t, err)

	assert.NoError(t, s.ensureToolInDefaultCatalogueTx(s.DB, tool))

	var count int64
	assert.NoError(t, s.DB.Table("tool_catalogue_tools").Where("tool_id = ?", tool.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "a repeated ensure call must not duplicate the membership")
}

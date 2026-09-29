package database

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/testutil/schemasnapshot"
)

// The schema the gateway's migrations produce is pinned by golden files, so
// a change underneath them (a gorm upgrade, a custom column type such as JSON
// that stops implementing GormDBDataType) cannot alter it unnoticed. A
// deliberate model change updates the goldens with UPDATE_SCHEMA_GOLDEN=1.
func TestGatewaySchemaSnapshotSQLite(t *testing.T) {
	db := openTestDB(t)
	schemasnapshot.AssertGolden(t, db, filepath.Join("testdata", "schema", "sqlite.golden"))
}

// The Postgres golden is generated on postgres:16, the version CI runs.
func TestGatewaySchemaSnapshotPostgres(t *testing.T) {
	dsn := os.Getenv("MGW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("MGW_TEST_POSTGRES_DSN not set")
	}
	admin, err := Connect(DatabaseConfig{Type: "postgres", DSN: dsn, MaxOpenConns: 2, MaxIdleConns: 2, LogLevel: "silent"})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("schema_snapshot_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		Close(admin)
	})

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := Connect(DatabaseConfig{Type: "postgres", DSN: u.String(), MaxOpenConns: 2, MaxIdleConns: 2, LogLevel: "silent"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Close(db) })
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	schemasnapshot.AssertGolden(t, db, filepath.Join("testdata", "schema", "postgres.golden"))
}

package studio

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Helpers shared by the Community tests here and the Enterprise tests
// linked into this package (enterprise/_coretests/pkg/studio).

// legacyCredentials is the credentials table as 2.2.2 created it (and as
// every 2.3 release candidate before this change did: the table had not
// changed): the secret in plaintext, found by its value.
const legacyCredentials = "CREATE TABLE `credentials` (`id` integer PRIMARY KEY AUTOINCREMENT,`created_at` datetime,`updated_at` datetime,`deleted_at` datetime,`key_id` text,`secret` text,`active` numeric DEFAULT false)"

func legacyKeys(t *testing.T, db *gorm.DB, n int) map[uint]string {
	t.Helper()
	keys := map[uint]string{}
	for i := 0; i < n; i++ {
		secret := "legacy-app-key-" + string(rune('a'+i))
		require.NoError(t, db.Exec("INSERT INTO credentials (key_id, secret, active, created_at, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
			"legacy-"+string(rune('a'+i)), secret, i%2 == 0).Error)
		var id uint
		require.NoError(t, db.Table("credentials").Select("id").Where("secret = ?", secret).Scan(&id).Error)
		keys[id] = secret
	}
	return keys
}

// seed222Database leaves db as a 2.2.2 Studio would for the purposes of
// these tests: its credentials table in the old form, with keys, and no
// schema version record (2.2 had none).
func seed222Database(t *testing.T, db *gorm.DB) map[uint]string {
	t.Helper()
	if db.Dialector.Name() == "postgres" {
		require.NoError(t, db.Exec("CREATE TABLE credentials (id bigserial PRIMARY KEY, created_at timestamptz, updated_at timestamptz, deleted_at timestamptz, key_id text, secret text, active boolean DEFAULT false)").Error)
		require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_credentials_key_id ON credentials (key_id)").Error)
		require.NoError(t, db.Exec("CREATE INDEX idx_credentials_deleted_at ON credentials (deleted_at)").Error)
	} else {
		require.NoError(t, db.Exec(legacyCredentials).Error)
		require.NoError(t, db.Exec("CREATE UNIQUE INDEX `idx_credentials_key_id` ON `credentials`(`key_id`)").Error)
		require.NoError(t, db.Exec("CREATE INDEX `idx_credentials_deleted_at` ON `credentials`(`deleted_at`)").Error)
	}
	return legacyKeys(t, db, 5)
}

// forEachStudioDatabase runs body with test options on SQLite and, when
// DATABASE_URL names a Postgres server, on Postgres in a schema of its own.
func forEachStudioDatabase(t *testing.T, body func(t *testing.T, opts Options)) {
	t.Run("sqlite", func(t *testing.T) {
		t.Setenv("STUDIO_TEST_POSTGRES_DSN", "")
		body(t, newTestOptions(t))
	})
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("DATABASE_URL")
		if !strings.HasPrefix(dsn, "postgres") {
			t.Skip("DATABASE_URL is not a Postgres URL")
		}
		t.Setenv("STUDIO_TEST_POSTGRES_DSN", dsn)
		body(t, newTestOptions(t))
	})
}

// databaseFingerprint is everything in the test's database, as text: every
// table, column and index definition and every row. Two equal fingerprints
// mean nothing was created, altered, inserted, updated or deleted.
func databaseFingerprint(t *testing.T, db *gorm.DB) string {
	t.Helper()
	pool, err := db.DB()
	require.NoError(t, err)

	var out []string
	strs := func(query string) []string {
		// The simple protocol on Postgres: a statement the driver has
		// cached fails once a migration has changed the table under it.
		var args []any
		if db.Dialector.Name() == "postgres" {
			args = append(args, pgx.QueryExecModeSimpleProtocol)
		}
		rows, err := pool.Query(query, args...)
		require.NoError(t, err, query)
		defer rows.Close()
		var got []string
		for rows.Next() {
			cols, err := rows.Columns()
			require.NoError(t, err)
			vals := make([]sql.NullString, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			require.NoError(t, rows.Scan(ptrs...))
			parts := make([]string, len(cols))
			for i, v := range vals {
				parts[i] = cols[i] + "=" + v.String
				if !v.Valid {
					parts[i] = cols[i] + "=<null>"
				}
			}
			got = append(got, strings.Join(parts, " | "))
		}
		require.NoError(t, rows.Err())
		return got
	}

	var tables []string
	if db.Dialector.Name() == "postgres" {
		out = append(out, strs("SELECT table_name, column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_schema = current_schema() ORDER BY table_name, ordinal_position")...)
		out = append(out, strs("SELECT tablename, indexname, indexdef FROM pg_indexes WHERE schemaname = current_schema() ORDER BY tablename, indexname")...)
		for _, row := range strs("SELECT tablename FROM pg_tables WHERE schemaname = current_schema() ORDER BY tablename") {
			tables = append(tables, strings.TrimPrefix(row, "tablename="))
		}
	} else {
		out = append(out, strs("SELECT type, name, tbl_name, sql FROM sqlite_master ORDER BY type, name")...)
		for _, row := range strs("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name") {
			tables = append(tables, strings.TrimPrefix(row, "name="))
		}
	}
	for _, table := range tables {
		rows := strs(fmt.Sprintf("SELECT * FROM %q", table))
		sort.Strings(rows)
		out = append(out, fmt.Sprintf("TABLE %s: %d row(s)", table, len(rows)))
		out = append(out, rows...)
	}
	return strings.Join(out, "\n")
}

package studio

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
)

func TestDSNWithSearchPath(t *testing.T) {
	cases := []struct {
		name, dsn, want, err string
	}{
		{name: "url", dsn: "postgres://u:p@h:5432/db", want: "postgres://u:p@h:5432/db?search_path=studio"},
		{name: "url with query", dsn: "postgresql://u:p@h/db?sslmode=disable", want: "postgresql://u:p@h/db?search_path=studio&sslmode=disable"},
		{name: "url already on schema", dsn: "postgres://h/db?search_path=studio", want: "postgres://h/db?search_path=studio"},
		{name: "url on another schema", dsn: "postgres://h/db?search_path=public", err: "search_path=public"},
		{name: "keywords", dsn: "host=h dbname=db sslmode=disable", want: "host=h dbname=db sslmode=disable search_path=studio"},
		{name: "keywords already on schema", dsn: "host=h search_path=studio", want: "host=h search_path=studio"},
		{name: "keywords on another schema", dsn: "host=h search_path='public'", err: "search_path='public'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := dsnWithSearchPath(tc.dsn, "studio")
			if tc.err != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// Schema names are checked before any connection is made, and they must be
// plain lower-case identifiers: search_path would fold an unquoted name.
func TestWithSchemaRejectsBadNames(t *testing.T) {
	for _, name := range []string{"Studio", "1studio", "studio-ai", "studio;drop", `a"b`, strings.Repeat("s", 64)} {
		_, err := withSchema(postgres.Open, "postgres://127.0.0.1:1/db", name)
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "lower-case identifier", name)
	}
}

func TestOpenDatabaseSchemaIsPostgresOnly(t *testing.T) {
	_, err := OpenDatabase(&config.AppConf{DatabaseType: "sqlite", DatabaseURL: t.TempDir() + "/s.db", DatabaseSchema: "studio"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "postgres only")
}

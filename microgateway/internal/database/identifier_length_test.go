package database

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// gorm shortens a generated index, constraint or join-table name longer than
// 64 characters with a SHA-1 suffix (schema.NamingStrategy.formatName), and
// SHA-1 panics under GODEBUG=fips140=only. The vendored gorm is never edited
// by hand, so instead no microgateway model may generate such a name: this checks
// the schema goldens (every name the migrations create) for identifiers of
// 64 characters or more. Postgres would also truncate anything over 63.
// See midsommar-clean-8dy7.27.
var identifier = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

func TestSchemaIdentifiersNeedNoSHA1Shortening(t *testing.T) {
	for _, golden := range []string{"sqlite.golden", "postgres.golden"} {
		b, err := os.ReadFile(filepath.Join("testdata", "schema", golden))
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range identifier.FindAllString(string(b), -1) {
			if len(id) >= 64 {
				t.Errorf("%s: identifier %q is %d characters; gorm would shorten it with SHA-1, which fails under fips140=only: give the index or constraint an explicit shorter name", golden, id, len(id))
			}
		}
	}
}

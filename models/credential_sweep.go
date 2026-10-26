package models

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// encryptedColumns lists every column that holds secrets.EncryptValue output
// ("$ENC/..."), either as the whole value or inside a JSON document. The
// Secret rows have their own sweep (secrets.ReencryptLegacySecrets).
var encryptedColumns = []struct {
	table   string
	columns []string
}{
	{"llms", []string{"api_key", "metadata", "resource_signing_secret", "resource_signing_secret_prev"}},
	{"webhook_targets", []string{"headers", "signing_secret", "prev_signing_secret"}},
	{"tyk_connections", []string{"dashboard_access_token", "mdcb_access_token"}},
	{"tyk_gateway_definitions", []string{"definition"}},
	{"submissions", []string{"resource_payload"}},
	{"mcp_credentials", []string{"tyk_key_plain_token"}},
}

// reencryptBatch bounds the rows read at a time.
var reencryptBatch = 100

// ReencryptStoredCredentials moves every "$ENC/" value outside the Secret
// table to the format a FIPS-mode process writes (v3): legacy AES-CFB and v2
// (scrypt) values are re-encrypted, v3 values left alone. It runs only in
// FIPS 140-3 mode and does nothing otherwise, so standard builds behave as
// before. Each row is written only while it still holds what was read, so a
// value saved meanwhile (through the API, or by another replica) keeps its
// new value. Returns how many values were re-encrypted.
func ReencryptStoredCredentials(db *gorm.DB) (int, error) {
	if !secrets.WritesFIPSFormat() || !secrets.EncryptionKeyConfigured() {
		return 0, nil
	}
	total := 0
	migrator := db.Migrator()
	for _, t := range encryptedColumns {
		if !migrator.HasTable(t.table) {
			continue
		}
		n, err := reencryptTable(db, t.table, t.columns)
		total += n
		if err != nil {
			return total, err
		}
	}
	if total > 0 {
		// The LLM credential cache remembers ciphertexts read before the
		// sweep; drop it so an unchanged save does not write them back.
		llmCiphers.Range(func(k, _ any) bool { llmCiphers.Delete(k); return true })
	}
	return total, nil
}

func reencryptTable(db *gorm.DB, table string, columns []string) (int, error) {
	// CAST(... AS TEXT) so json columns (Postgres has no = or LIKE on json)
	// compare as text in both databases.
	var like []string
	for _, c := range columns {
		like = append(like, fmt.Sprintf("CAST(%s AS TEXT) LIKE ?", c))
	}
	where := strings.Join(like, " OR ")
	args := make([]any, len(columns))
	for i := range args {
		args[i] = "%$ENC/%"
	}

	migrated := 0
	// Keyset paging on id, which is an integer or a UUID string depending on
	// the table; the first page has no lower bound.
	var lastID any
	for {
		sel := "id"
		for _, c := range columns {
			sel += fmt.Sprintf(", CAST(%s AS TEXT)", c)
		}
		q := db.Table(table).Select(sel).Where(where, args...)
		if lastID != nil {
			q = q.Where("id > ?", lastID)
		}
		type row struct {
			id     any
			values []sql.NullString
		}
		var batch []row
		rows, err := q.Order("id").Limit(reencryptBatch).Rows()
		if err != nil {
			return migrated, fmt.Errorf("read %s: %w", table, err)
		}
		for rows.Next() {
			r := row{values: make([]sql.NullString, len(columns))}
			dest := []any{&r.id}
			for i := range r.values {
				dest = append(dest, &r.values[i])
			}
			if err := rows.Scan(dest...); err != nil {
				rows.Close()
				return migrated, fmt.Errorf("read %s: %w", table, err)
			}
			if b, ok := r.id.([]byte); ok { // a string id, as some drivers return it
				r.id = string(b)
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return migrated, fmt.Errorf("read %s: %w", table, err)
		}
		if len(batch) == 0 {
			return migrated, nil
		}
		for _, r := range batch {
			lastID = r.id
			for i, c := range columns {
				old := r.values[i].String
				if old == "" {
					continue
				}
				updated, n := secrets.ReencryptEncValues(old)
				if n == 0 {
					continue
				}
				res := db.Exec(fmt.Sprintf("UPDATE %s SET %s = ? WHERE id = ? AND CAST(%s AS TEXT) = ?", table, c, c), updated, r.id, old)
				if res.Error != nil {
					return migrated, fmt.Errorf("write %s.%s (id %v): %w", table, c, r.id, res.Error)
				}
				if res.RowsAffected == 1 {
					migrated += n
				}
			}
		}
	}
}

// Package schemasnapshot renders the schema a migration produced as stable
// text, so tests can compare it with a golden file. It guards the schema
// against changes made underneath it, such as a gorm upgrade or a custom
// type that stops implementing GormDBDataType.
package schemasnapshot

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// UpdateEnv set to 1 rewrites the golden files instead of comparing with them.
const UpdateEnv = "UPDATE_SCHEMA_GOLDEN"

// AssertGolden compares the schema of db with the golden file at path.
func AssertGolden(t *testing.T, db *gorm.DB, path string) {
	t.Helper()
	got, err := Dump(db)
	if err != nil {
		t.Fatalf("dump schema: %v", err)
	}
	if os.Getenv(UpdateEnv) == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (make schema-golden creates it): %v", path, err)
	}
	if got != string(want) {
		t.Fatalf("schema differs from %s; if the change is deliberate, make schema-golden updates the goldens:\n%s",
			path, lineDiff(string(want), got))
	}
}

// Dump renders the schema of db as text that does not depend on the order
// in which gorm emitted constraints or on the Postgres schema name.
func Dump(db *gorm.DB) (string, error) {
	switch name := db.Dialector.Name(); name {
	case "sqlite":
		return dumpSQLite(db)
	case "postgres":
		return dumpPostgres(db)
	default:
		return "", fmt.Errorf("schemasnapshot: unsupported dialect %q", name)
	}
}

func dumpSQLite(db *gorm.DB) (string, error) {
	var tables []struct{ Name, SQL string }
	if err := db.Raw(`SELECT name, sql FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`).Scan(&tables).Error; err != nil {
		return "", err
	}
	var b strings.Builder
	for _, tbl := range tables {
		fmt.Fprintf(&b, "TABLE %s\n", tbl.Name)
		columns, constraints := splitCreateTable(tbl.SQL)
		for _, c := range columns {
			fmt.Fprintf(&b, "  column %s\n", c)
		}
		sort.Strings(constraints)
		for _, c := range constraints {
			fmt.Fprintf(&b, "  %s\n", c)
		}

		// Explicit indexes by name. Automatic indexes (from UNIQUE and
		// PRIMARY KEY) are numbered in DDL order, so they are listed by
		// their columns instead.
		var indexes []struct {
			Name   string
			Unique int
			Origin string
		}
		if err := db.Raw(fmt.Sprintf("PRAGMA index_list(%q)", tbl.Name)).Scan(&indexes).Error; err != nil {
			return "", err
		}
		var lines []string
		for _, idx := range indexes {
			if idx.Origin == "c" {
				var sql string
				if err := db.Raw(`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?`, idx.Name).
					Scan(&sql).Error; err != nil {
					return "", err
				}
				lines = append(lines, "index "+sql)
				continue
			}
			var cols []struct{ Name string }
			if err := db.Raw(fmt.Sprintf("PRAGMA index_info(%q)", idx.Name)).Scan(&cols).Error; err != nil {
				return "", err
			}
			names := make([]string, len(cols))
			for i, c := range cols {
				names[i] = c.Name
			}
			lines = append(lines, fmt.Sprintf("autoindex origin=%s unique=%d (%s)", idx.Origin, idx.Unique, strings.Join(names, ", ")))
		}
		sort.Strings(lines)
		for _, l := range lines {
			fmt.Fprintf(&b, "  %s\n", l)
		}
	}
	return b.String(), nil
}

// splitCreateTable splits a CREATE TABLE statement into its column
// definitions, in order, and its table constraints.
func splitCreateTable(sql string) (columns, constraints []string) {
	open := strings.Index(sql, "(")
	closing := strings.LastIndex(sql, ")")
	if open < 0 || closing <= open {
		return []string{sql}, nil
	}
	for _, item := range splitTopLevel(sql[open+1 : closing]) {
		upper := strings.ToUpper(item)
		switch {
		case strings.HasPrefix(upper, "CONSTRAINT "), strings.HasPrefix(upper, "PRIMARY KEY"),
			strings.HasPrefix(upper, "UNIQUE"), strings.HasPrefix(upper, "FOREIGN KEY"),
			strings.HasPrefix(upper, "CHECK"):
			constraints = append(constraints, "constraint "+item)
		default:
			columns = append(columns, item)
		}
	}
	return columns, constraints
}

// splitTopLevel splits s at commas outside parentheses and quotes.
func splitTopLevel(s string) []string {
	var items []string
	depth, start := 0, 0
	var quote rune
	for i, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"' || r == '`':
			quote = r
		case r == '(':
			depth++
		case r == ')':
			depth--
		case r == ',' && depth == 0:
			items = append(items, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	if rest := strings.TrimSpace(s[start:]); rest != "" {
		items = append(items, rest)
	}
	return items
}

func dumpPostgres(db *gorm.DB) (string, error) {
	var schema string
	if err := db.Raw(`SELECT current_schema()`).Scan(&schema).Error; err != nil {
		return "", err
	}
	unqualify := func(s string) string { return strings.ReplaceAll(s, schema+".", "") }

	var columns []struct {
		TableName, ColumnName, DataType, UdtName, IsNullable string
		CharacterMaximumLength, NumericPrecision, NumericScale *int
		ColumnDefault                                          *string
	}
	if err := db.Raw(`SELECT table_name, column_name, data_type, udt_name, is_nullable,
			character_maximum_length, numeric_precision, numeric_scale, column_default
		FROM information_schema.columns WHERE table_schema = current_schema()
		ORDER BY table_name, ordinal_position`).Scan(&columns).Error; err != nil {
		return "", err
	}

	var constraints []struct{ TableName, Name, Def string }
	if err := db.Raw(`SELECT c.conrelid::regclass::text AS table_name, c.conname AS name,
			pg_get_constraintdef(c.oid) AS def
		FROM pg_constraint c JOIN pg_namespace n ON n.oid = c.connamespace
		WHERE n.nspname = current_schema() AND c.conrelid <> 0
		ORDER BY 1, 2`).Scan(&constraints).Error; err != nil {
		return "", err
	}

	var indexes []struct{ TableName, Name, Def string }
	if err := db.Raw(`SELECT tablename AS table_name, indexname AS name, indexdef AS def
		FROM pg_indexes WHERE schemaname = current_schema() ORDER BY 1, 2`).Scan(&indexes).Error; err != nil {
		return "", err
	}

	var b strings.Builder
	table := ""
	for _, c := range columns {
		if c.TableName != table {
			table = c.TableName
			fmt.Fprintf(&b, "TABLE %s\n", table)
			for _, con := range constraints {
				if unqualify(con.TableName) == table {
					fmt.Fprintf(&b, "  constraint %s %s\n", con.Name, unqualify(con.Def))
				}
			}
			for _, idx := range indexes {
				if idx.TableName == table {
					fmt.Fprintf(&b, "  index %s\n", unqualify(idx.Def))
				}
			}
		}
		fmt.Fprintf(&b, "  column %s %s(%s) nullable=%s", c.ColumnName, c.DataType, c.UdtName, c.IsNullable)
		if c.CharacterMaximumLength != nil {
			fmt.Fprintf(&b, " len=%d", *c.CharacterMaximumLength)
		}
		if c.NumericPrecision != nil {
			fmt.Fprintf(&b, " precision=%d", *c.NumericPrecision)
		}
		if c.NumericScale != nil {
			fmt.Fprintf(&b, " scale=%d", *c.NumericScale)
		}
		if c.ColumnDefault != nil {
			fmt.Fprintf(&b, " default=%s", unqualify(*c.ColumnDefault))
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// lineDiff lists the lines only in want (-) and only in got (+), each with
// the table it belongs to.
func lineDiff(want, got string) string {
	count := map[string]int{}
	add := func(s string, n int) {
		table := ""
		for _, l := range strings.Split(s, "\n") {
			if strings.HasPrefix(l, "TABLE ") {
				table = l
				count[l] += n
				continue
			}
			count[table+" |"+l] += n
		}
	}
	add(want, 1)
	add(got, -1)
	var out []string
	for l, n := range count {
		for ; n > 0; n-- {
			out = append(out, "- "+l)
		}
		for ; n < 0; n++ {
			out = append(out, "+ "+l)
		}
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

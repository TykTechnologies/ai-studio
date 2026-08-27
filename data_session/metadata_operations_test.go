package data_session

import (
	"strings"
	"testing"
)

func TestValidateMetadataFilterKey(t *testing.T) {
	valid := []string{
		"source",
		"file_name",
		"chunk-id",
		"doc.section",
		"Key123",
		"_internal",
	}
	for _, key := range valid {
		if err := validateMetadataFilterKey(key); err != nil {
			t.Errorf("expected key %q to be valid, got error: %v", key, err)
		}
	}

	invalid := []string{
		"",
		"a' = '' OR '1'='1",
		"key'; DROP TABLE users; --",
		"key\\",
		"key with spaces",
		"key\"quoted",
		"key$1",
		"key;",
		"key(",
		strings.Repeat("a", 129), // too long
	}
	for _, key := range invalid {
		if err := validateMetadataFilterKey(key); err == nil {
			t.Errorf("expected key %q to be rejected", key)
		}
	}
}

func TestValidatePGTableName(t *testing.T) {
	valid := []string{
		"documents",
		"my_table",
		"MyTable2",
		"_private",
		"public.documents",
		"myschema.my_table",
	}
	for _, name := range valid {
		if err := validatePGTableName(name); err != nil {
			t.Errorf("expected table name %q to be valid, got error: %v", name, err)
		}
	}

	invalid := []string{
		"",
		"1table",              // cannot start with digit
		"table; DROP TABLE x", // injection
		"table name",          // space
		"table\"name",         // quote
		"table'name",          // quote
		"a.b.c",               // too many qualifiers
		"schema.",             // empty part
		".table",              // empty part
		strings.Repeat("a", 64), // exceeds postgres identifier limit
	}
	for _, name := range invalid {
		if err := validatePGTableName(name); err == nil {
			t.Errorf("expected table name %q to be rejected", name)
		}
	}
}

func TestBuildPGVectorWhereClause(t *testing.T) {
	ds := &DataSession{}

	t.Run("empty filter", func(t *testing.T) {
		clause, args, err := ds.buildPGVectorWhereClause(map[string]string{}, "AND")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if clause != "1=1" {
			t.Errorf("expected clause '1=1', got %q", clause)
		}
		if len(args) != 0 {
			t.Errorf("expected no args, got %v", args)
		}
	})

	t.Run("single key", func(t *testing.T) {
		clause, args, err := ds.buildPGVectorWhereClause(map[string]string{"source": "doc.pdf"}, "AND")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if clause != "metadata->>'source' = $1" {
			t.Errorf("unexpected clause: %q", clause)
		}
		if len(args) != 1 || args[0] != "doc.pdf" {
			t.Errorf("unexpected args: %v", args)
		}
	})

	t.Run("multiple keys OR", func(t *testing.T) {
		clause, args, err := ds.buildPGVectorWhereClause(map[string]string{"a": "1", "b": "2"}, "OR")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(clause, " OR ") {
			t.Errorf("expected OR operator in clause: %q", clause)
		}
		if len(args) != 2 {
			t.Errorf("expected 2 args, got %v", args)
		}
	})

	t.Run("malicious key is rejected", func(t *testing.T) {
		mal := "x' = '' OR '1'='1"
		_, _, err := ds.buildPGVectorWhereClause(map[string]string{mal: "v"}, "AND")
		if err == nil {
			t.Fatal("expected error for malicious filter key, got nil")
		}
	})

	t.Run("malicious key never reaches SQL", func(t *testing.T) {
		mal := "x'; DELETE FROM t; --"
		clause, _, err := ds.buildPGVectorWhereClause(map[string]string{"ok": "v", mal: "v"}, "AND")
		if err == nil && strings.Contains(clause, "DELETE") {
			t.Fatalf("malicious key was interpolated into SQL: %q", clause)
		}
		if err == nil {
			t.Fatal("expected error for malicious filter key, got nil")
		}
	})
}

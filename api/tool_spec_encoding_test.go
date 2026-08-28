package api

import (
	"encoding/base64"
	"testing"
)

func TestValidateOASSpecEncoding(t *testing.T) {
	const spec = `{"openapi":"3.0.0","info":{"title":"Canada’s holidays — v1"}}`

	t.Run("empty spec is allowed", func(t *testing.T) {
		if err := validateOASSpecEncoding(""); err != nil {
			t.Fatalf("expected no error for empty spec, got %v", err)
		}
	})

	t.Run("base64 of a UTF-8 spec is accepted", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte(spec))
		if err := validateOASSpecEncoding(encoded); err != nil {
			t.Fatalf("expected base64 spec to be accepted, got %v", err)
		}
	})

	t.Run("raw JSON is rejected", func(t *testing.T) {
		// The natural mistake when scripting the API: the field is a string, so
		// raw JSON binds and used to be stored, failing much later at
		// /spec-operations with "illegal base64 data".
		err := validateOASSpecEncoding(spec)
		if err == nil {
			t.Fatal("expected raw JSON to be rejected")
		}
	})

	t.Run("raw YAML is rejected", func(t *testing.T) {
		if err := validateOASSpecEncoding("openapi: 3.0.0\npaths: {}\n"); err == nil {
			t.Fatal("expected raw YAML to be rejected")
		}
	})
}

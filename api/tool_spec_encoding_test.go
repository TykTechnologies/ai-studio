package api

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// A spec that is base64 but not a usable OpenAPI document is refused on
// create, and one stored before validation existed makes spec-operations
// answer 422, not 500.
func TestToolSpec_APIRejectsGarbageAndFlagsStoredOnes(t *testing.T) {
	api, db := setupTestAPI(t)
	garbage := base64.StdEncoding.EncodeToString([]byte("test"))

	w := performRequest(api.router, "POST", "/api/v1/tools", map[string]interface{}{
		"data": map[string]interface{}{"type": "tools", "attributes": map[string]interface{}{
			"name": "Garbage Spec", "tool_type": models.ToolTypeREST, "oas_spec": garbage, "privacy_score": 1,
		}},
	})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "OpenAPI") {
		t.Fatalf("create with a garbage spec: %d %s, want 400 naming the spec problem", w.Code, w.Body.String())
	}

	legacy := &models.Tool{Name: "Legacy Tool", ToolType: models.ToolTypeREST, OASSpec: garbage, PrivacyScore: 1}
	if err := db.Create(legacy).Error; err != nil {
		t.Fatal(err)
	}
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/tools/%d/spec-operations", legacy.ID), nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("spec-operations for a stored garbage spec: %d %s, want 422", w.Code, w.Body.String())
	}
}

func TestValidateToolOASSpec(t *testing.T) {
	// UTF-8 in the title: the import path once mangled it (#481).
	const spec = `{"openapi":"3.0.0","info":{"title":"Canada’s holidays — v1","version":"1"},` +
		`"servers":[{"url":"https://holidays.example.com"}],"paths":{}}`

	t.Run("empty spec is allowed", func(t *testing.T) {
		if err := services.ValidateToolOASSpec(""); err != nil {
			t.Fatalf("expected no error for empty spec, got %v", err)
		}
	})

	t.Run("base64 of a UTF-8 spec is accepted", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte(spec))
		if err := services.ValidateToolOASSpec(encoded); err != nil {
			t.Fatalf("expected base64 spec to be accepted, got %v", err)
		}
	})

	rejected := map[string]string{
		// The natural mistake when scripting the API: the field is a string, so
		// raw JSON binds and used to be stored, failing much later at
		// /spec-operations with "illegal base64 data".
		"raw JSON": spec,
		"raw YAML": "openapi: 3.0.0\npaths: {}\n",
		// Valid base64 that is not a usable spec used to be stored too, and
		// /spec-operations then returned 500 forever.
		"base64 of text":               base64.StdEncoding.EncodeToString([]byte("test")),
		"base64 of a spec, no servers": base64.StdEncoding.EncodeToString([]byte(`{"openapi":"3.0.0","info":{"title":"t","version":"1"},"paths":{}}`)),
		"base64 of a Swagger 2 spec":   base64.StdEncoding.EncodeToString([]byte(`{"swagger":"2.0","info":{"title":"t","version":"1"},"paths":{}}`)),
	}
	for name, value := range rejected {
		t.Run(name+" is rejected", func(t *testing.T) {
			err := services.ValidateToolOASSpec(value)
			if !errors.Is(err, services.ErrInvalidToolSpec) {
				t.Fatalf("ValidateToolOASSpec() error = %v, want ErrInvalidToolSpec", err)
			}
		})
	}
}

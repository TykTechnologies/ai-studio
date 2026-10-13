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

	// The legacy tool can still be renamed: an unchanged spec is not checked.
	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tools/%d", legacy.ID), toolBody("Legacy Tool renamed", models.ToolTypeREST, garbage))
	if w.Code != http.StatusOK {
		t.Fatalf("renaming a legacy tool without touching its spec: %d %s, want 200", w.Code, w.Body.String())
	}
}

func toolBody(name, toolType, spec string) map[string]interface{} {
	return map[string]interface{}{
		"data": map[string]interface{}{"type": "tools", "attributes": map[string]interface{}{
			"name": name, "tool_type": toolType, "oas_spec": spec, "privacy_score": 1,
		}},
	}
}

// CLIENT tools keep a client tool definition in oas_spec, not an OpenAPI
// document, so the OpenAPI check does not apply to them.
func TestToolSpec_ClientToolsAreNotOpenAPIChecked(t *testing.T) {
	api, db := setupTestAPI(t)
	if err := models.GetOrCreateDefaultClientTools(db); err != nil {
		t.Fatal(err)
	}
	var present models.Tool
	if err := db.Where("tool_type = ?", models.ToolTypeClient).First(&present).Error; err != nil {
		t.Fatal(err)
	}

	w := performRequest(api.router, "POST", "/api/v1/tools", toolBody("My Client Tool", models.ToolTypeClient, present.OASSpec))
	if w.Code != http.StatusCreated {
		t.Fatalf("create a CLIENT tool: %d %s, want 201", w.Code, w.Body.String())
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tools/%d", present.ID), toolBody("Present (renamed)", models.ToolTypeClient, present.OASSpec))
	if w.Code != http.StatusOK {
		t.Fatalf("rename the present tool: %d %s, want 200", w.Code, w.Body.String())
	}

	// The same definition on a REST tool is not an OpenAPI document.
	w = performRequest(api.router, "POST", "/api/v1/tools", toolBody("Not REST", models.ToolTypeREST, present.OASSpec))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create a REST tool with a client tool definition: %d %s, want 400", w.Code, w.Body.String())
	}
}

func TestValidateToolOASSpec(t *testing.T) {
	// UTF-8 in the title: the import path once mangled it (#481).
	const spec = `{"openapi":"3.0.0","info":{"title":"Canada’s holidays — v1","version":"1"},` +
		`"servers":[{"url":"https://holidays.example.com"}],"paths":{}}`

	t.Run("empty spec is allowed", func(t *testing.T) {
		if err := services.ValidateToolOASSpec(models.ToolTypeREST, ""); err != nil {
			t.Fatalf("expected no error for empty spec, got %v", err)
		}
	})

	t.Run("base64 of a UTF-8 spec is accepted", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte(spec))
		if err := services.ValidateToolOASSpec(models.ToolTypeREST, encoded); err != nil {
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
			err := services.ValidateToolOASSpec(models.ToolTypeREST, value)
			if !errors.Is(err, services.ErrInvalidToolSpec) {
				t.Fatalf("ValidateToolOASSpec() error = %v, want ErrInvalidToolSpec", err)
			}
		})
	}
}

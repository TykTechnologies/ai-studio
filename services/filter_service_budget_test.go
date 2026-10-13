package services

import (
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

func guardrailSpec(extra map[string]any) FilterSpec {
	cfg := models.JSONMap{
		"provider":  guardrails.ProviderBuiltin,
		"detectors": []any{map[string]any{"name": guardrails.CategoryInjection}},
		"on_detect": guardrails.ActionLog,
	}
	for k, v := range extra {
		cfg[k] = v
	}
	return FilterSpec{Name: "g", Kind: models.FilterKindGuardrail, Config: cfg}
}

// A saved guardrail stores only the budget values its admin set: the
// defaults derive from other settings, so storing them would make a later
// edit of those settings fail validation (midsommar-clean-4uc.34).
func TestGuardrailSaveStoresOnlyTheBudgetTheAdminSet(t *testing.T) {
	_, stored, err := guardrailSpec(nil).validate()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := stored["budget"]; ok {
		t.Errorf("stored config carries derived budget defaults: %v", stored["budget"])
	}
	if stored["timeout_ms"] == nil || stored["scope"] == nil {
		t.Errorf("the other defaults are still stored: %v", stored)
	}

	// Saving the stored config again with a much longer timeout succeeds:
	// no pinned time budget contradicts it.
	again := map[string]any{}
	for k, v := range stored {
		again[k] = v
	}
	again["timeout_ms"] = 60000
	if _, _, err := guardrailSpec(again).validate(); err != nil {
		t.Errorf("raising timeout_ms on a saved filter: %v", err)
	}

	// What the admin set is stored as set, and still validated.
	_, stored, err = guardrailSpec(map[string]any{"budget": map[string]any{"max_pieces": 7}}).validate()
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := stored["budget"].(map[string]any); len(b) != 1 || b["max_pieces"] != float64(7) {
		t.Errorf("stored budget = %v, want only max_pieces", stored["budget"])
	}
	if _, _, err := guardrailSpec(map[string]any{"timeout_ms": 2000, "budget": map[string]any{"time_ms": 100}}).validate(); !errors.Is(err, guardrails.ErrInvalidConfig) {
		t.Errorf("a time budget under the timeout: err %v, want ErrInvalidConfig", err)
	}
}

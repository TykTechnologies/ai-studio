//go:build enterprise

package proxy

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Issue #535: a filter script that panics inside the Tengo VM used to unwind
// through the response path and take the whole gateway process down with it —
// every tenant's traffic, not just the request carrying the bad filter.
//
// ExecuteResponseFilters is the single response-side call site: the Bedrock
// translators and the streaming path all route through it.
func TestExecuteResponseFilters_PanickingScriptFailsTheRequestNotTheProcess(t *testing.T) {
	llm := &models.LLM{
		Vendor: models.OPENAI,
		Filters: []*models.Filter{
			{
				Name:           "panicking-filter",
				Script:         []byte(`output := 5 / 0`),
				ResponseFilter: true,
			},
		},
	}

	responseBody := []byte(`{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`)
	r := httptest.NewRequest("POST", "/openai/v1/chat/completions", nil)

	// If the panic escaped RunScript, this call would kill the test binary.
	blocked, blockMessage, err := ExecuteResponseFilters(llm, nil, responseBody, 200, false, false, 0, "", r)

	if err == nil {
		t.Fatal("ExecuteResponseFilters() returned no error for a panicking filter script")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Errorf("ExecuteResponseFilters() error = %q, want it to name the panic", err)
	}
	if !strings.Contains(err.Error(), "panicking-filter") {
		t.Errorf("ExecuteResponseFilters() error = %q, want it to name the offending filter", err)
	}

	// Response filters fail open by design: the error is reported, the response
	// is not reported as blocked.
	if blocked {
		t.Errorf("ExecuteResponseFilters() blocked = true, want false (response filters fail open)")
	}
	if blockMessage != "" {
		t.Errorf("ExecuteResponseFilters() blockMessage = %q, want empty", blockMessage)
	}
}

// A panicking filter must not stop the filters behind it from being evaluated
// in a later request, and must not leave the runner in a broken state.
func TestExecuteResponseFilters_SurvivingFilterStillRuns(t *testing.T) {
	llm := &models.LLM{
		Vendor: models.OPENAI,
		Filters: []*models.Filter{
			{
				Name:           "panicking-filter",
				Script:         []byte(`output := 5 / 0`),
				ResponseFilter: true,
			},
		},
	}

	responseBody := []byte(`{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`)
	r := httptest.NewRequest("POST", "/openai/v1/chat/completions", nil)

	if _, _, err := ExecuteResponseFilters(llm, nil, responseBody, 200, false, false, 0, "", r); err == nil {
		t.Fatal("expected an error from the panicking filter")
	}

	// Swap in a healthy blocking filter and confirm the path still works.
	llm.Filters[0].Script = []byte(`output := {block: true, payload: input.raw_input, message: "nope"}`)

	blocked, blockMessage, err := ExecuteResponseFilters(llm, nil, responseBody, 200, false, false, 0, "", r)
	if err != nil {
		t.Fatalf("ExecuteResponseFilters() error = %v, want nil", err)
	}
	if !blocked {
		t.Error("ExecuteResponseFilters() blocked = false, want true")
	}
	if blockMessage != "nope" {
		t.Errorf("ExecuteResponseFilters() blockMessage = %q, want %q", blockMessage, "nope")
	}
}

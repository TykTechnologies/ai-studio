//go:build enterprise

package chat_session

import (
	"context"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Issue #535: a chat filter script that panics inside the Tengo VM used to
// unwind through the chat session goroutine and take the process down. It must
// now surface as an ordinary filter error.
func TestExecuteResponseFilters_PanickingScriptDoesNotCrash(t *testing.T) {
	filters := []*models.Filter{
		{
			Name:           "panicking-filter",
			Script:         []byte(`output := 5 / 0`),
			ResponseFilter: true,
		},
	}

	// If the panic escaped RunScript, this call would kill the test binary.
	blocked, blockMessage, err := ExecuteResponseFilters(
		context.Background(), filters, nil,
		"hello from the model", "openai", "gpt-4",
		false, false, 0, "", "session-1", 1, 2,
	)

	if err == nil {
		t.Fatal("ExecuteResponseFilters() returned no error for a panicking filter script")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Errorf("ExecuteResponseFilters() error = %q, want it to name the panic", err)
	}
	if !strings.Contains(err.Error(), "panicking-filter") {
		t.Errorf("ExecuteResponseFilters() error = %q, want it to name the offending filter", err)
	}

	// Chat response filters fail open, matching the LLM response path.
	if blocked {
		t.Error("ExecuteResponseFilters() blocked = true, want false (response filters fail open)")
	}
	if blockMessage != "" {
		t.Errorf("ExecuteResponseFilters() blockMessage = %q, want empty", blockMessage)
	}
}

// A healthy filter on the same path must keep working after the recover was
// added.
func TestExecuteResponseFilters_HealthyFilterStillBlocks(t *testing.T) {
	filters := []*models.Filter{
		{
			Name:           "blocking-filter",
			Script:         []byte(`output := {block: true, payload: input.raw_input, message: "nope"}`),
			ResponseFilter: true,
		},
	}

	blocked, blockMessage, err := ExecuteResponseFilters(
		context.Background(), filters, nil,
		"hello from the model", "openai", "gpt-4",
		false, false, 0, "", "session-1", 1, 2,
	)

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

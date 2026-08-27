package services

import (
	"strings"
	"testing"
)

// TestAsAIStudioPluginClient_WrongType verifies that a dispensed value of the
// wrong type produces an error instead of terminating the host process
// (regression test for log.Fatal on dispense type mismatch).
func TestAsAIStudioPluginClient_WrongType(t *testing.T) {
	cases := []struct {
		name string
		raw  interface{}
	}{
		{"nil value", nil},
		{"string value", "not a plugin client"},
		{"struct value", struct{ X int }{X: 1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := asAIStudioPluginClient(tc.raw)
			if err == nil {
				t.Fatalf("expected error for %T, got nil", tc.raw)
			}
			if client != nil {
				t.Fatalf("expected nil client on type mismatch, got %v", client)
			}
			if !strings.Contains(err.Error(), "type mismatch") {
				t.Errorf("error should describe the type mismatch, got: %v", err)
			}
		})
	}
}

// TestAsAIStudioPluginClient_CorrectType verifies the happy path returns the
// wrapper unchanged.
func TestAsAIStudioPluginClient_CorrectType(t *testing.T) {
	wrapper := &AIStudioPluginClient{}
	client, err := asAIStudioPluginClient(wrapper)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client != wrapper {
		t.Fatalf("expected the same wrapper instance back")
	}
}

package services

import (
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// Saving an HTTP classifier with a socket endpoint needs the operator's
// allowlist: without it, docker.sock (or any socket) is refused.
func TestGuardrailSaveRefusesUnlistedUnixSocket(t *testing.T) {
	spec := FilterSpec{Name: "g", Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider":   guardrails.ProviderHTTP,
		"connection": map[string]any{"endpoint": "unix:///var/run/docker.sock"},
		"detectors":  []any{map[string]any{"name": "prompt_attack"}},
		"on_detect":  guardrails.ActionLog,
	}}
	t.Setenv(guardrails.EnvUnixSocketAllowlist, "")
	if _, _, err := spec.validate(); !errors.Is(err, guardrails.ErrUnixSocketRefused) || !errors.Is(err, guardrails.ErrInvalidConfig) {
		t.Fatalf("validate() error = %v, want a refused socket", err)
	}
	t.Setenv(guardrails.EnvUnixSocketAllowlist, "/run/guard/")
	if _, _, err := spec.validate(); !errors.Is(err, guardrails.ErrUnixSocketRefused) {
		t.Fatalf("docker.sock outside the allowlist: error = %v", err)
	}
	spec.Config["connection"] = map[string]any{"endpoint": "unix:///run/guard/guard.sock", "path": "/v1/classify"}
	if _, _, err := spec.validate(); err != nil {
		t.Fatalf("an allowed socket (not mounted on this node) was refused: %v", err)
	}
}

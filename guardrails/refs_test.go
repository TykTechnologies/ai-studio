package guardrails

import (
	"sort"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// Guardrail connections follow the platform rule: $ENV/ resolves only names
// on AI_STUDIO_ENV_REF_ALLOWLIST. There is no GUARDRAIL_ prefix exemption.
func TestResolveConnectionValue_PlatformAllowlist(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "studio-encryption-key")
	t.Setenv("GUARDRAIL_LAKERA_KEY", "lakera-key")
	t.Setenv("CLASSIFIER_TOKEN", "classifier-token")
	t.Setenv(secrets.EnvRefAllowlistVar, "CLASSIFIER_TOKEN")

	for in, want := range map[string]string{
		"$ENV/TYK_AI_SECRET_KEY":       "$ENV/TYK_AI_SECRET_KEY",
		"$ENV/GUARDRAIL_LAKERA_KEY":    "$ENV/GUARDRAIL_LAKERA_KEY", // not listed
		"$ENV/CLASSIFIER_TOKEN":        "classifier-token",
		"$SECRET/UNKNOWN":              "$SECRET/UNKNOWN", // no secret store here
		"https://classifier.example/x": "https://classifier.example/x",
	} {
		if got := ResolveConnectionValue(in); got != want {
			t.Errorf("ResolveConnectionValue(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewConnectionReferences(t *testing.T) {
	old := map[string]string{"api_key": "$SECRET/A", "endpoint": "https://x"}
	updated := map[string]string{"api_key": "$SECRET/A", "token": "$SECRET/B", "endpoint": "$SECRET/C", "env": "$ENV/X"}
	got := NewConnectionReferences(old, updated)
	sort.Strings(got)
	if strings.Join(got, ",") != "connection.endpoint,connection.env,connection.token" {
		t.Errorf("NewConnectionReferences = %v", got)
	}
	if got := NewConnectionReferences(old, old); len(got) != 0 {
		t.Errorf("unchanged references reported: %v", got)
	}
}

// What an edge gets for a connection value the hub left unresolved: a dud key
// for a secret (or undeclared) field, so no provider client falls back to
// credentials from the edge's own environment; a bare "$ENV/" for other
// fields. Resolved values pass through.
func TestEdgeConnectionValue(t *testing.T) {
	cases := []struct{ provider, field, in, want string }{
		{ProviderHTTP, "api_key", "$ENV/X", secrets.RefusedCredential},
		{ProviderHTTP, "endpoint", "$ENV/X", "$ENV/"},
		{ProviderHTTP, "undeclared", "$SECRET/Y", secrets.RefusedCredential},
		{"no_such_provider", "endpoint", "$ENV/X", secrets.RefusedCredential},
		{ProviderHTTP, "api_key", "resolved-key", "resolved-key"},
		{ProviderHTTP, "endpoint", "https://x", "https://x"},
	}
	for _, c := range cases {
		if got := EdgeConnectionValue(c.provider, c.field, c.in); got != c.want {
			t.Errorf("EdgeConnectionValue(%s, %s, %q) = %q, want %q", c.provider, c.field, c.in, got, c.want)
		}
	}
	if !IsReference(secrets.RefusedCredential) {
		t.Error("the dud key must count as unresolved, so the runner never calls the provider with it")
	}
}

package secrets

import (
	"sort"
	"strings"
	"testing"
)

// $ENV/ resolves only names on AI_STUDIO_ENV_REF_ALLOWLIST: nothing by
// default, no prefixes, never the setting itself, never a malformed name.
func TestResolveRef_Allowlist(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "studio-key")
	t.Setenv("REFPOLICY_ALLOWED", "allowed-value")
	t.Setenv("GUARDRAIL_X", "prefixed-value")

	t.Setenv(EnvRefAllowlistVar, "")
	if got := ResolveRef("$ENV/REFPOLICY_ALLOWED"); got != "$ENV/REFPOLICY_ALLOWED" {
		t.Errorf("empty allowlist resolved $ENV/: %q", got)
	}

	t.Setenv(EnvRefAllowlistVar, " REFPOLICY_ALLOWED , OTHER,"+EnvRefAllowlistVar)
	for in, want := range map[string]string{
		"$ENV/REFPOLICY_ALLOWED":     "allowed-value",
		"$ENV/TYK_AI_SECRET_KEY":     "$ENV/TYK_AI_SECRET_KEY",
		"$ENV/GUARDRAIL_X":           "$ENV/GUARDRAIL_X",
		"$ENV/" + EnvRefAllowlistVar: "$ENV/" + EnvRefAllowlistVar,
		"$ENV/":                      "$ENV/",
		"literal":                    "literal",
	} {
		if got := ResolveRef(in); got != want {
			t.Errorf("ResolveRef(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "A/B", "A B", "A\tB", EnvRefAllowlistVar} {
		if EnvRefAllowed(bad) {
			t.Errorf("EnvRefAllowed(%q) = true", bad)
		}
	}
}

// Edges call DisableEnvRefs: nothing resolves, even a listed name.
func TestDisableEnvRefs(t *testing.T) {
	t.Setenv("REFPOLICY_ALLOWED", "allowed-value")
	t.Setenv(EnvRefAllowlistVar, "REFPOLICY_ALLOWED")
	DisableEnvRefs()
	t.Cleanup(func() { envRefsDisabled.Store(false) })
	if got := ResolveRef("$ENV/REFPOLICY_ALLOWED"); got != "$ENV/REFPOLICY_ALLOWED" {
		t.Errorf("resolved with env refs disabled: %q", got)
	}
}

// What an edge gets for a value still holding a reference: a bare "$ENV/".
// A pre-2.3 edge resolves that with GetValue to os.Getenv(""), i.e. empty,
// never one of its own variables; a current edge sees a reference.
func TestForEdge(t *testing.T) {
	for in, want := range map[string]string{
		"$ENV/MICROGATEWAY_ENCRYPTION_KEY": "$ENV/",
		"$SECRET/UNKNOWN":                  "$ENV/",
		"resolved-value":                   "resolved-value",
		"":                                 "",
	} {
		if got := ForEdge(in); got != want {
			t.Errorf("ForEdge(%q) = %q, want %q", in, got, want)
		}
	}
	t.Setenv("MICROGATEWAY_ENCRYPTION_KEY", "edge-key")
	if got := GetValue(ForEdge("$ENV/MICROGATEWAY_ENCRYPTION_KEY"), false); got != "" {
		t.Errorf("old-edge resolution of the shipped value = %q, want empty", got)
	}
	if !IsReference(ForEdge("$ENV/X")) || EnvRefAllowed("") {
		t.Error("a bare $ENV/ must stay an unresolved reference")
	}
}

// New, changed or moved $SECRET/ and $ENV/ references need secrets read.
func TestNewReferences(t *testing.T) {
	old := map[string]string{"a": "$SECRET/A", "e": "https://x", "v": "$ENV/X"}
	updated := map[string]string{"a": "$SECRET/A", "b": "$SECRET/B", "e": "$SECRET/C", "v": "$ENV/X", "w": "$ENV/Y", "l": "literal"}
	got := NewReferences(old, updated)
	sort.Strings(got)
	if strings.Join(got, ",") != "b,e,w" {
		t.Errorf("NewReferences = %v, want [b e w]", got)
	}
}

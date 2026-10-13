package secrets

import "testing"

// A refused credential reaches edges as a dud key that is neither empty
// (vendor clients fill an empty key from their own environment) nor a
// reference (released edges resolve $ENV/ themselves).
func TestForEdgeCredential(t *testing.T) {
	for in, want := range map[string]string{
		"$ENV/OPENAI_API_KEY": RefusedCredential,
		"$SECRET/MISSING":     RefusedCredential,
		"$ENV/":               RefusedCredential,
		"sk-resolved":         "sk-resolved",
		"":                    "",
	} {
		if got := ForEdgeCredential(in); got != want {
			t.Errorf("ForEdgeCredential(%q) = %q, want %q", in, got, want)
		}
	}
	if RefusedCredential == "" || IsReference(RefusedCredential) {
		t.Fatal("the placeholder must be a non-empty, non-reference value")
	}
	if got := GetValue(RefusedCredential, false); got != RefusedCredential {
		t.Errorf("an edge resolving the placeholder got %q", got)
	}
}

// GetValue applies the policy itself, so no caller can resolve an $ENV/
// name the operator has not listed; references are kept with preserveRef.
func TestGetValueEnvPolicy(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "canary-master-key")
	t.Setenv("VAULT_TOKEN", "vault-value")
	t.Setenv(EnvRefAllowlistVar, "")

	for _, ref := range []string{"$ENV/TYK_AI_SECRET_KEY", "$ENV/VAULT_TOKEN", "$ENV/", "$ENV/" + EnvRefAllowlistVar} {
		if got := GetValue(ref, false); got != ref {
			t.Errorf("GetValue(%q) = %q with nothing allowlisted, want it unchanged", ref, got)
		}
	}

	t.Setenv(EnvRefAllowlistVar, "VAULT_TOKEN, "+EnvRefAllowlistVar)
	if got := GetValue("$ENV/VAULT_TOKEN", false); got != "vault-value" {
		t.Errorf("allowlisted GetValue = %q, want the value", got)
	}
	if got := GetValue("$ENV/TYK_AI_SECRET_KEY", false); got != "$ENV/TYK_AI_SECRET_KEY" {
		t.Errorf("unlisted GetValue = %q, want it unchanged", got)
	}
	if got := GetValue("$ENV/"+EnvRefAllowlistVar, false); got != "$ENV/"+EnvRefAllowlistVar {
		t.Errorf("the allowlist variable itself resolved to %q", got)
	}
	if got := GetValue("$ENV/VAULT_TOKEN", true); got != "$ENV/VAULT_TOKEN" {
		t.Errorf("GetValue with preserveRef = %q, want the reference", got)
	}
	if got := ResolveRef("$ENV/VAULT_TOKEN"); got != "vault-value" {
		t.Errorf("ResolveRef = %q", got)
	}
}

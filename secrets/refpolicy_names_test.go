package secrets

import "testing"

// A malformed name is refused even when an operator lists it.
func TestEnvRefAllowed_RefusesMalformedListedNames(t *testing.T) {
	t.Setenv(EnvRefAllowlistVar, "A/B,A B,OK")
	for _, name := range []string{"A/B", "A B"} {
		if EnvRefAllowed(name) {
			t.Errorf("EnvRefAllowed(%q) = true for a listed but malformed name", name)
		}
	}
	if !EnvRefAllowed("OK") {
		t.Error("a listed well-formed name was refused")
	}
}

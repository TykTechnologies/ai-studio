package ollamaclient

import "testing"

// Tyk: with no server URL the client uses the local default, never
// OLLAMA_HOST from the process environment.
func TestNoEnvironmentFallback(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "https://canary.invalid:443")

	c, err := NewClient(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.base.String(); got != "http://127.0.0.1:11434" {
		t.Errorf("base = %q, want http://127.0.0.1:11434", got)
	}
}

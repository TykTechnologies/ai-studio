package anthropic

import (
	"errors"
	"testing"
)

// Tyk: an empty token never falls back to ANTHROPIC_API_KEY; AI Studio
// passes every credential explicitly.
func TestNoEnvironmentFallback(t *testing.T) {
	t.Setenv(tokenEnvVarName, "canary-anthropic-key")

	for name, opts := range map[string][]Option{
		"no token":    nil,
		"empty token": {WithToken("")},
	} {
		if _, err := New(opts...); !errors.Is(err, ErrMissingToken) {
			t.Errorf("%s: err = %v, want ErrMissingToken", name, err)
		}
	}
}

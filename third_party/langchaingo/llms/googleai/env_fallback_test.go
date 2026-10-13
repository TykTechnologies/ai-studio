package googleai

import (
	"context"
	"errors"
	"testing"
)

// Tyk: an empty key never falls back to GOOGLE_API_KEY or to the host's
// Application Default Credentials (which would send the host's Google
// identity to the configured endpoint).
func TestNoEnvironmentFallback(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "canary-google-key")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nonexistent/canary-adc.json")

	for name, opts := range map[string][]Option{
		"no key":    nil,
		"empty key": {WithAPIKey("")},
	} {
		if _, err := New(context.Background(), opts...); !errors.Is(err, ErrMissingAPIKey) {
			t.Errorf("%s: err = %v, want ErrMissingAPIKey", name, err)
		}
	}

	o := DefaultOptions()
	o.EnsureAuthPresent()
	if hasAuthOptions(o.ClientOptions) {
		t.Error("EnsureAuthPresent added a key from the environment")
	}

	g, err := New(context.Background(), WithAPIKey("refused-credential-reference"))
	if err != nil {
		t.Fatalf("New with a key: %v", err)
	}
	_ = g.Close()
}

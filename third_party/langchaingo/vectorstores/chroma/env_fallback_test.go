//go:build cgo

package chroma

import (
	"errors"
	"testing"
)

// Tyk: the store never takes its URL from CHROMA_URL or an OpenAI key (and
// organization) from the process environment.
func TestNoEnvironmentFallback(t *testing.T) {
	t.Setenv(ChromaURLKeyEnvVarName, "http://canary.invalid:8000")
	t.Setenv(OpenAIAPIKeyEnvVarName, "canary-openai-key")
	t.Setenv(OpenAIOrgIDEnvVarName, "canary-org")

	if s, err := applyClientOptions(WithOpenAIAPIKey("k")); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("no URL: err = %v (url %q), want ErrInvalidOptions", err, s.chromaURL)
	}
	s, err := applyClientOptions(WithChromaURL("http://chroma.example.invalid:8000"))
	if !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("no embedder or key: err = %v (key %q, org %q), want ErrInvalidOptions", err, s.openaiAPIKey, s.openaiOrganization)
	}
}

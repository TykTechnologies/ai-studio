package pinecone

import (
	"context"
	"errors"
	"testing"
)

type fakeEmbedder struct{}

func (fakeEmbedder) EmbedDocuments(context.Context, []string) ([][]float32, error) { return nil, nil }
func (fakeEmbedder) EmbedQuery(context.Context, string) ([]float32, error)         { return nil, nil }

// Tyk: an empty API key never falls back to PINECONE_API_KEY.
func TestNoEnvironmentFallback(t *testing.T) {
	t.Setenv("PINECONE_API_KEY", "canary-pinecone-key")

	for name, opts := range map[string][]Option{
		"no key":    {WithHost("https://index.example.invalid"), WithEmbedder(fakeEmbedder{})},
		"empty key": {WithHost("https://index.example.invalid"), WithEmbedder(fakeEmbedder{}), WithAPIKey("")},
	} {
		s, err := applyClientOptions(opts...)
		if !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("%s: err = %v (key %q), want ErrInvalidOptions", name, err, s.apiKey)
		}
	}
}

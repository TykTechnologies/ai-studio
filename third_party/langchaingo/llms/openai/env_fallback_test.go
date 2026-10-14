package openai

import (
	"errors"
	"testing"
)

// Tyk: the client never fills a credential or destination from the process
// environment. AI Studio builds clients from stored LLM and embedder
// settings; an empty key falling back to OPENAI_API_KEY sent the host's key
// to whatever endpoint the LLM's writer chose.
func TestNoEnvironmentFallback(t *testing.T) {
	t.Setenv(tokenEnvVarName, "canary-openai-key")
	t.Setenv(modelEnvVarName, "canary-model")
	t.Setenv(baseURLEnvVarName, "http://canary.invalid/v1")
	t.Setenv(baseAPIBaseEnvVarName, "http://canary-base.invalid/v1")
	t.Setenv(organizationEnvVarName, "canary-org")

	if _, err := New(); !errors.Is(err, ErrMissingToken) {
		t.Fatalf("New() without a token: err = %v, want ErrMissingToken", err)
	}
	if _, err := New(WithToken("")); !errors.Is(err, ErrMissingToken) {
		t.Fatalf("New(WithToken(\"\")): err = %v, want ErrMissingToken", err)
	}

	opts, _, err := newClient(WithToken("refused-credential-reference"))
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	if opts.token != "refused-credential-reference" {
		t.Errorf("token = %q, want the one passed", opts.token)
	}
	if opts.baseURL != "" || opts.model != "" || opts.organization != "" {
		t.Errorf("options read the environment: baseURL=%q model=%q organization=%q",
			opts.baseURL, opts.model, opts.organization)
	}
}

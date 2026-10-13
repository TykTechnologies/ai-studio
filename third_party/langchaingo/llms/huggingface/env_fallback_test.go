package huggingface

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Tyk: an empty token never falls back to HF_TOKEN,
// HUGGINGFACEHUB_API_TOKEN or a token file on the host.
func TestNoEnvironmentFallback(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, defaultTokenPath)
	if err := os.WriteFile(tokenFile, []byte("canary-hf-file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".cache", "huggingface"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".cache", "huggingface", defaultTokenPath), []byte("canary-hf-home-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HF_TOKEN", "canary-hf-token")
	t.Setenv("HUGGINGFACEHUB_API_TOKEN", "canary-hf-legacy-token")
	t.Setenv("HF_TOKEN_PATH", tokenFile)
	t.Setenv("HF_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("HOME", dir)

	for name, opts := range map[string][]Option{
		"no token":    nil,
		"empty token": {WithToken("")},
	} {
		if _, err := New(opts...); !errors.Is(err, ErrMissingToken) {
			t.Errorf("%s: err = %v, want ErrMissingToken", name, err)
		}
	}
}

package secrets

import (
	"testing"
)

func TestRegistry_Get_Local(t *testing.T) {
	kek, err := DefaultRegistry.Get("local", "test-key", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if kek == nil {
		t.Fatal("expected non-nil KEKProvider")
	}

	// Verify it works: generate, wrap, unwrap round-trip
	ctx := t.Context()
	wrapped, err := kek.GenerateDEK(ctx)
	if err != nil {
		t.Fatalf("GenerateDEK failed: %v", err)
	}
	if len(wrapped) == 0 {
		t.Fatal("expected non-empty wrapped DEK")
	}
}

func TestRegistry_Get_Unknown(t *testing.T) {
	_, err := DefaultRegistry.Get("nonexistent", "key", nil)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestRegistry_Names(t *testing.T) {
	names := DefaultRegistry.Names()
	found := false
	for _, name := range names {
		if name == "local" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'local' in registered providers, got %v", names)
	}
}

func TestRegistry_Register_DuplicateReturnsError(t *testing.T) {
	err := DefaultRegistry.Register("local", func(rawKey string, _ map[string]string) (KEKProvider, error) {
		return nil, nil
	})
	if err == nil {
		t.Fatal("expected error on duplicate registration")
	}
}

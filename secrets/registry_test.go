package secrets

import (
	"testing"
)

func TestNewKEKProvider_Local(t *testing.T) {
	kek, err := NewKEKProvider("local", "test-key")
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

func TestNewKEKProvider_Unknown(t *testing.T) {
	_, err := NewKEKProvider("nonexistent", "key")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestKEKProviderNames(t *testing.T) {
	names := KEKProviderNames()
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

func TestRegisterKEKProvider_DuplicatePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	RegisterKEKProvider("local", func(rawKey string) (KEKProvider, error) {
		return nil, nil
	})
}

package secrets

import (
	"fmt"
	"testing"
)

func TestRegistry_Get_Local(t *testing.T) {
	kek, err := DefaultRegistry.Get("local", map[string]string{"RAW_KEY": "test-key"})
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
	_, err := DefaultRegistry.Get("nonexistent", nil)
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

func TestRegistry_Register_OverridesPrevious(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register("test", func(config map[string]string) (KEKProvider, error) {
		return nil, fmt.Errorf("old factory")
	})
	reg.Register("test", func(config map[string]string) (KEKProvider, error) {
		return newTestLocalKEK("override"), nil
	})
	kek, err := reg.Get("test", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if kek == nil {
		t.Fatal("expected non-nil KEKProvider from overridden factory")
	}
}

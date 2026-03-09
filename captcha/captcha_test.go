package captcha

import (
	"fmt"
	"testing"
)

func TestIsVerificationError(t *testing.T) {
	if !IsVerificationError(ErrVerificationFailed) {
		t.Fatal("expected true for ErrVerificationFailed")
	}
	wrapped := fmt.Errorf("wrapped: %w", ErrVerificationFailed)
	if !IsVerificationError(wrapped) {
		t.Fatal("expected true for wrapped ErrVerificationFailed")
	}
	if IsVerificationError(fmt.Errorf("other")) {
		t.Fatal("expected false for unrelated error")
	}
}

func TestRegisterAndNewProvider(t *testing.T) {
	// Save and restore registry state
	mu.Lock()
	saved := registry
	registry = map[string]FactoryFunc{}
	mu.Unlock()
	defer func() {
		mu.Lock()
		registry = saved
		mu.Unlock()
	}()

	Register("test", func(siteKey, secretKey string, opts map[string]string) (Provider, error) {
		return NewHTTPProvider(HTTPProviderConfig{
			Name:      "test",
			SiteKey:   siteKey,
			SecretKey: secretKey,
			VerifyURL: "http://localhost",
		}), nil
	})

	p, err := NewProvider("test", "site", "secret", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "test" {
		t.Fatalf("expected name 'test', got %q", p.Name())
	}
	if p.SiteKey() != "site" {
		t.Fatalf("expected site key 'site', got %q", p.SiteKey())
	}
}

func TestNewProvider_Unknown(t *testing.T) {
	_, err := NewProvider("nonexistent", "site", "secret", nil)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestRegisteredProviders(t *testing.T) {
	mu.Lock()
	saved := registry
	registry = map[string]FactoryFunc{}
	mu.Unlock()
	defer func() {
		mu.Lock()
		registry = saved
		mu.Unlock()
	}()

	Register("beta", func(string, string, map[string]string) (Provider, error) { return nil, nil })
	Register("alpha", func(string, string, map[string]string) (Provider, error) { return nil, nil })

	names := RegisteredProviders()
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("expected [alpha beta], got %v", names)
	}
}

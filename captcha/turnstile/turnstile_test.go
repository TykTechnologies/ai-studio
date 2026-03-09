package turnstile

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/captcha"
)

func TestTurnstile(t *testing.T) {
	p, err := newTurnstile("site", "secret", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "turnstile" {
		t.Fatalf("expected name 'turnstile', got %q", p.Name())
	}
	if p.SiteKey() != "site" {
		t.Fatalf("expected site key 'site', got %q", p.SiteKey())
	}
}

func TestRegistration(t *testing.T) {
	providers := captcha.RegisteredProviders()
	found := false
	for _, p := range providers {
		if p == "turnstile" {
			found = true
		}
	}
	if !found {
		t.Fatal("turnstile not registered")
	}
}

package hcaptcha

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/captcha"
)

func TestHCaptcha(t *testing.T) {
	p, err := newHCaptcha("site", "secret", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "hcaptcha" {
		t.Fatalf("expected name 'hcaptcha', got %q", p.Name())
	}
	if p.SiteKey() != "site" {
		t.Fatalf("expected site key 'site', got %q", p.SiteKey())
	}
}

func TestRegistration(t *testing.T) {
	providers := captcha.RegisteredProviders()
	found := false
	for _, p := range providers {
		if p == "hcaptcha" {
			found = true
		}
	}
	if !found {
		t.Fatal("hcaptcha not registered")
	}
}

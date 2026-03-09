package recaptcha

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/captcha"
)

func TestRecaptchaV2(t *testing.T) {
	p, err := newRecaptchaV2("site", "secret", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "recaptcha_v2" {
		t.Fatalf("expected name 'recaptcha_v2', got %q", p.Name())
	}
	if p.SiteKey() != "site" {
		t.Fatalf("expected site key 'site', got %q", p.SiteKey())
	}
}

func TestRecaptchaV3_DefaultScore(t *testing.T) {
	p, err := newRecaptchaV3("site", "secret", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "recaptcha_v3" {
		t.Fatalf("expected name 'recaptcha_v3', got %q", p.Name())
	}
}

func TestRecaptchaV3_CustomScore(t *testing.T) {
	opts := map[string]string{"min_score": "0.8"}
	_, err := newRecaptchaV3("site", "secret", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegistration(t *testing.T) {
	// init() should have registered both providers
	providers := captcha.RegisteredProviders()
	found := map[string]bool{}
	for _, p := range providers {
		found[p] = true
	}
	if !found["recaptcha_v2"] {
		t.Fatal("recaptcha_v2 not registered")
	}
	if !found["recaptcha_v3"] {
		t.Fatal("recaptcha_v3 not registered")
	}
}

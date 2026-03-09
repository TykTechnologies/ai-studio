package config

import (
	"os"
	"testing"
)

func TestGetRecaptchaConfig(t *testing.T) {
	t.Setenv("TYK_AI_RECAPTCHA_SITE_KEY", "rc-site")
	t.Setenv("TYK_AI_RECAPTCHA_SECRET_KEY", "rc-secret")
	os.Unsetenv("TYK_AI_RECAPTCHA_MIN_SCORE")

	cfg := getRecaptchaConfig()
	if cfg.SiteKey != "rc-site" {
		t.Fatalf("expected site key 'rc-site', got %q", cfg.SiteKey)
	}
	if cfg.SecretKey != "rc-secret" {
		t.Fatalf("expected secret key 'rc-secret', got %q", cfg.SecretKey)
	}
	if cfg.MinScore != 0.5 {
		t.Fatalf("expected default min score 0.5, got %f", cfg.MinScore)
	}
}

func TestGetRecaptchaConfig_CustomScore(t *testing.T) {
	t.Setenv("TYK_AI_RECAPTCHA_SITE_KEY", "site")
	t.Setenv("TYK_AI_RECAPTCHA_SECRET_KEY", "secret")
	t.Setenv("TYK_AI_RECAPTCHA_MIN_SCORE", "0.7")

	cfg := getRecaptchaConfig()
	if cfg.MinScore != 0.7 {
		t.Fatalf("expected min score 0.7, got %f", cfg.MinScore)
	}
}

func TestGetRecaptchaConfig_InvalidScore(t *testing.T) {
	t.Setenv("TYK_AI_RECAPTCHA_SITE_KEY", "site")
	t.Setenv("TYK_AI_RECAPTCHA_SECRET_KEY", "secret")
	t.Setenv("TYK_AI_RECAPTCHA_MIN_SCORE", "abc")

	cfg := getRecaptchaConfig()
	if cfg.MinScore != 0.5 {
		t.Fatalf("expected default 0.5 for invalid input, got %f", cfg.MinScore)
	}
}

func TestGetHCaptchaConfig(t *testing.T) {
	t.Setenv("TYK_AI_HCAPTCHA_SITE_KEY", "hc-site")
	t.Setenv("TYK_AI_HCAPTCHA_SECRET_KEY", "hc-secret")

	cfg := getHCaptchaConfig()
	if cfg.SiteKey != "hc-site" {
		t.Fatalf("expected site key 'hc-site', got %q", cfg.SiteKey)
	}
	if cfg.SecretKey != "hc-secret" {
		t.Fatalf("expected secret key 'hc-secret', got %q", cfg.SecretKey)
	}
}

func TestGetTurnstileConfig(t *testing.T) {
	t.Setenv("TYK_AI_TURNSTILE_SITE_KEY", "ts-site")
	t.Setenv("TYK_AI_TURNSTILE_SECRET_KEY", "ts-secret")

	cfg := getTurnstileConfig()
	if cfg.SiteKey != "ts-site" {
		t.Fatalf("expected site key 'ts-site', got %q", cfg.SiteKey)
	}
	if cfg.SecretKey != "ts-secret" {
		t.Fatalf("expected secret key 'ts-secret', got %q", cfg.SecretKey)
	}
}

func TestGetCaptchaConfig_NotEnabled(t *testing.T) {
	os.Unsetenv("TYK_AI_CAPTCHA_ENABLED")
	cfg := getCaptchaConfig(&AppConf{})
	if cfg.Enabled {
		t.Fatal("expected disabled when TYK_AI_CAPTCHA_ENABLED is not set")
	}
}

func TestGetCaptchaConfig_EnabledNoProvider(t *testing.T) {
	t.Setenv("TYK_AI_CAPTCHA_ENABLED", "true")
	os.Unsetenv("TYK_AI_CAPTCHA_PROVIDER")
	cfg := getCaptchaConfig(&AppConf{})
	if cfg.Enabled {
		t.Fatal("expected disabled when provider is not set")
	}
}

func TestGetCaptchaConfig_InvalidProvider(t *testing.T) {
	t.Setenv("TYK_AI_CAPTCHA_ENABLED", "true")
	t.Setenv("TYK_AI_CAPTCHA_PROVIDER", "invalid")
	cfg := getCaptchaConfig(&AppConf{})
	if cfg.Enabled {
		t.Fatal("expected disabled for invalid provider")
	}
}

func TestGetCaptchaConfig_MissingKeys(t *testing.T) {
	t.Setenv("TYK_AI_CAPTCHA_ENABLED", "true")
	t.Setenv("TYK_AI_CAPTCHA_PROVIDER", "turnstile")
	cfg := getCaptchaConfig(&AppConf{})
	if cfg.Enabled {
		t.Fatal("expected disabled without provider keys")
	}
}

func TestGetCaptchaConfig_Turnstile(t *testing.T) {
	t.Setenv("TYK_AI_CAPTCHA_ENABLED", "true")
	t.Setenv("TYK_AI_CAPTCHA_PROVIDER", "turnstile")

	conf := &AppConf{
		Turnstile: TurnstileConfig{SiteKey: "ts-site", SecretKey: "ts-secret"},
	}
	cfg := getCaptchaConfig(conf)
	if !cfg.Enabled {
		t.Fatal("expected enabled")
	}
	if cfg.Provider != "turnstile" {
		t.Fatalf("expected provider 'turnstile', got %q", cfg.Provider)
	}
}

func TestGetCaptchaConfig_HCaptcha(t *testing.T) {
	t.Setenv("TYK_AI_CAPTCHA_ENABLED", "true")
	t.Setenv("TYK_AI_CAPTCHA_PROVIDER", "hcaptcha")

	conf := &AppConf{
		HCaptcha: HCaptchaConfig{SiteKey: "hc-site", SecretKey: "hc-secret"},
	}
	cfg := getCaptchaConfig(conf)
	if !cfg.Enabled {
		t.Fatal("expected enabled")
	}
}

func TestGetCaptchaConfig_RecaptchaV2(t *testing.T) {
	t.Setenv("TYK_AI_CAPTCHA_ENABLED", "true")
	t.Setenv("TYK_AI_CAPTCHA_PROVIDER", "recaptcha_v2")

	conf := &AppConf{
		Recaptcha: RecaptchaConfig{SiteKey: "rc-site", SecretKey: "rc-secret"},
	}
	cfg := getCaptchaConfig(conf)
	if !cfg.Enabled {
		t.Fatal("expected enabled")
	}
}

func TestGetCaptchaConfig_RecaptchaV3(t *testing.T) {
	t.Setenv("TYK_AI_CAPTCHA_ENABLED", "true")
	t.Setenv("TYK_AI_CAPTCHA_PROVIDER", "recaptcha_v3")

	conf := &AppConf{
		Recaptcha: RecaptchaConfig{SiteKey: "rc-site", SecretKey: "rc-secret", MinScore: 0.8},
	}
	cfg := getCaptchaConfig(conf)
	if !cfg.Enabled {
		t.Fatal("expected enabled")
	}
}

func TestGetCaptchaConfig_CaseInsensitive(t *testing.T) {
	t.Setenv("TYK_AI_CAPTCHA_ENABLED", "true")
	t.Setenv("TYK_AI_CAPTCHA_PROVIDER", "TURNSTILE")

	conf := &AppConf{
		Turnstile: TurnstileConfig{SiteKey: "site", SecretKey: "secret"},
	}
	cfg := getCaptchaConfig(conf)
	if !cfg.Enabled {
		t.Fatal("expected enabled for uppercase provider")
	}
	if cfg.Provider != "turnstile" {
		t.Fatalf("expected lowercased provider, got %q", cfg.Provider)
	}
}

func TestProviderEnvPrefix(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"recaptcha_v2", "RECAPTCHA"},
		{"recaptcha_v3", "RECAPTCHA"},
		{"hcaptcha", "HCAPTCHA"},
		{"turnstile", "TURNSTILE"},
		{"unknown", "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := providerEnvPrefix(tt.provider); got != tt.want {
			t.Errorf("providerEnvPrefix(%q) = %q, want %q", tt.provider, got, tt.want)
		}
	}
}

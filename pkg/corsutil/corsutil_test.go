package corsutil

import (
	"net/http/httptest"
	"testing"
)

func TestAllowOriginUnconfiguredKeepsWildcard(t *testing.T) {
	t.Setenv(EnvAllowedOrigins, "")

	origin, configured := AllowOrigin("https://anything.example.com")
	if origin != "*" || configured {
		t.Errorf("unconfigured must preserve wildcard behavior, got (%q, %v)", origin, configured)
	}
}

func TestAllowOriginExplicitWildcard(t *testing.T) {
	t.Setenv(EnvAllowedOrigins, "*")

	origin, configured := AllowOrigin("https://anything.example.com")
	if origin != "*" || configured {
		t.Errorf("explicit wildcard must behave as wildcard, got (%q, %v)", origin, configured)
	}
}

func TestAllowOriginConfiguredList(t *testing.T) {
	t.Setenv(EnvAllowedOrigins, "https://app.example.com, https://admin.example.com")

	origin, configured := AllowOrigin("https://app.example.com")
	if origin != "https://app.example.com" || !configured {
		t.Errorf("allowlisted origin must be echoed, got (%q, %v)", origin, configured)
	}

	origin, configured = AllowOrigin("HTTPS://APP.EXAMPLE.COM")
	if origin == "" || !configured {
		t.Errorf("origin matching must be case-insensitive, got (%q, %v)", origin, configured)
	}

	origin, _ = AllowOrigin("https://evil.example.com")
	if origin != "" {
		t.Errorf("non-allowlisted origin must get no CORS header, got %q", origin)
	}

	origin, _ = AllowOrigin("")
	if origin != "" {
		t.Errorf("requests without an Origin header get no CORS header, got %q", origin)
	}
}

func TestSetCORSHeaders(t *testing.T) {
	t.Run("unconfigured wildcard", func(t *testing.T) {
		t.Setenv(EnvAllowedOrigins, "")
		w := httptest.NewRecorder()
		SetCORSHeaders(w.Header(), "https://x.example.com", "GET, OPTIONS")
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("expected wildcard, got %q", got)
		}
		if w.Header().Get("Access-Control-Allow-Methods") != "GET, OPTIONS" {
			t.Error("methods header missing")
		}
	})

	t.Run("configured match echoes origin and varies", func(t *testing.T) {
		t.Setenv(EnvAllowedOrigins, "https://app.example.com")
		w := httptest.NewRecorder()
		SetCORSHeaders(w.Header(), "https://app.example.com", "POST, OPTIONS")
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
			t.Errorf("expected echoed origin, got %q", got)
		}
		if got := w.Header().Get("Vary"); got != "Origin" {
			t.Errorf("expected Vary: Origin, got %q", got)
		}
	})

	t.Run("configured mismatch omits allow-origin", func(t *testing.T) {
		t.Setenv(EnvAllowedOrigins, "https://app.example.com")
		w := httptest.NewRecorder()
		SetCORSHeaders(w.Header(), "https://evil.example.com", "GET, OPTIONS")
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("expected no allow-origin header, got %q", got)
		}
	})
}

package mcaptcha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/captcha"
)

func newMockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestVerify_Success(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/pow/siteverify" {
			t.Errorf("expected path /api/v1/pow/siteverify, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected JSON content type")
		}
		var req verifyRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Secret != "my-secret" || req.Key != "my-site" || req.Token != "valid-token" {
			t.Errorf("unexpected request: %+v", req)
		}
		json.NewEncoder(w).Encode(verifyResponse{Valid: true})
	})

	p := &provider{siteKey: "my-site", secret: "my-secret", instanceURL: srv.URL}

	if err := p.Verify(context.Background(), "valid-token", "1.2.3.4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerify_EmptyToken(t *testing.T) {
	p := &provider{siteKey: "k", secret: "s", instanceURL: "http://unused"}
	err := p.Verify(context.Background(), "", "1.2.3.4")
	if !captcha.IsVerificationError(err) {
		t.Fatalf("expected verification error, got: %v", err)
	}
}

func TestVerify_Rejected(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(verifyResponse{Valid: false})
	})

	p := &provider{siteKey: "k", secret: "s", instanceURL: srv.URL}
	err := p.Verify(context.Background(), "bad-token", "")
	if !captcha.IsVerificationError(err) {
		t.Fatalf("expected verification error, got: %v", err)
	}
}

func TestVerify_NetworkError(t *testing.T) {
	p := &provider{siteKey: "k", secret: "s", instanceURL: "http://127.0.0.1:1"}
	err := p.Verify(context.Background(), "token", "")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	if captcha.IsVerificationError(err) {
		t.Fatal("network errors should not be verification errors")
	}
}

func TestVerify_InvalidJSON(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	})

	p := &provider{siteKey: "k", secret: "s", instanceURL: srv.URL}
	err := p.Verify(context.Background(), "token", "")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestNewMCaptcha(t *testing.T) {
	p, err := newMCaptcha("site", "secret", map[string]string{"instance_url": "https://captcha.example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "mcaptcha" {
		t.Fatalf("expected name 'mcaptcha', got %q", p.Name())
	}
	if p.SiteKey() != "site" {
		t.Fatalf("expected site key 'site', got %q", p.SiteKey())
	}
}

func TestNewMCaptcha_MissingInstanceURL(t *testing.T) {
	_, err := newMCaptcha("site", "secret", nil)
	if err == nil {
		t.Fatal("expected error for missing instance_url")
	}
}

func TestRegistration(t *testing.T) {
	providers := captcha.RegisteredProviders()
	found := false
	for _, p := range providers {
		if p == "mcaptcha" {
			found = true
		}
	}
	if !found {
		t.Fatal("mcaptcha not registered")
	}
}

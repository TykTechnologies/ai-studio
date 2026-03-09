package captcha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newMockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestVerify_Success(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(verifyResponse{Success: true})
	})

	p := NewHTTPProvider(HTTPProviderConfig{
		Name: "test", SiteKey: "site", SecretKey: "secret", VerifyURL: srv.URL,
	})

	if err := p.Verify(context.Background(), "valid-token", "1.2.3.4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerify_EmptyToken(t *testing.T) {
	p := NewHTTPProvider(HTTPProviderConfig{
		Name: "test", SiteKey: "k", SecretKey: "s", VerifyURL: "http://unused",
	})

	err := p.Verify(context.Background(), "", "1.2.3.4")
	if !IsVerificationError(err) {
		t.Fatalf("expected verification error, got: %v", err)
	}
}

func TestVerify_Failed(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(verifyResponse{
			Success:    false,
			ErrorCodes: []string{"invalid-input-response"},
		})
	})

	p := NewHTTPProvider(HTTPProviderConfig{
		Name: "test", SiteKey: "k", SecretKey: "s", VerifyURL: srv.URL,
	})

	err := p.Verify(context.Background(), "bad-token", "1.2.3.4")
	if !IsVerificationError(err) {
		t.Fatalf("expected verification error, got: %v", err)
	}
}

func TestVerify_ScoreTooLow(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(verifyResponse{Success: true, Score: 0.2})
	})

	p := NewHTTPProvider(HTTPProviderConfig{
		Name: "recaptcha_v3", SiteKey: "k", SecretKey: "s", VerifyURL: srv.URL, MinScore: 0.5,
	})

	err := p.Verify(context.Background(), "token", "1.2.3.4")
	if !IsVerificationError(err) {
		t.Fatalf("expected verification error for low score, got: %v", err)
	}
}

func TestVerify_ScoreAboveThreshold(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(verifyResponse{Success: true, Score: 0.9})
	})

	p := NewHTTPProvider(HTTPProviderConfig{
		Name: "recaptcha_v3", SiteKey: "k", SecretKey: "s", VerifyURL: srv.URL, MinScore: 0.5,
	})

	if err := p.Verify(context.Background(), "token", "1.2.3.4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerify_NetworkError(t *testing.T) {
	p := NewHTTPProvider(HTTPProviderConfig{
		Name: "test", SiteKey: "k", SecretKey: "s", VerifyURL: "http://127.0.0.1:1",
	})

	err := p.Verify(context.Background(), "token", "1.2.3.4")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	if IsVerificationError(err) {
		t.Fatal("network errors should not be verification errors")
	}
}

func TestVerify_InvalidJSON(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	})

	p := NewHTTPProvider(HTTPProviderConfig{
		Name: "test", SiteKey: "k", SecretKey: "s", VerifyURL: srv.URL,
	})

	err := p.Verify(context.Background(), "token", "1.2.3.4")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestVerify_SendsCorrectParams(t *testing.T) {
	srv := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("secret") != "my-secret" {
			t.Errorf("expected secret 'my-secret', got %q", r.URL.Query().Get("secret"))
		}
		if r.URL.Query().Get("response") != "my-token" {
			t.Errorf("expected response 'my-token', got %q", r.URL.Query().Get("response"))
		}
		if r.URL.Query().Get("remoteip") != "10.0.0.1" {
			t.Errorf("expected remoteip '10.0.0.1', got %q", r.URL.Query().Get("remoteip"))
		}
		json.NewEncoder(w).Encode(verifyResponse{Success: true})
	})

	p := NewHTTPProvider(HTTPProviderConfig{
		Name: "test", SiteKey: "k", SecretKey: "my-secret", VerifyURL: srv.URL,
	})
	p.Verify(context.Background(), "my-token", "10.0.0.1")
}

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/captcha"
	"github.com/gin-gonic/gin"
)

type stubProvider struct {
	name    string
	siteKey string
	err     error
}

func (s *stubProvider) Verify(_ context.Context, _, _ string) error { return s.err }
func (s *stubProvider) Name() string                                { return s.name }
func (s *stubProvider) SiteKey() string                             { return s.siteKey }

func init() { gin.SetMode(gin.TestMode) }

func setupCaptchaRouter(provider captcha.Provider) *gin.Engine {
	r := gin.New()
	r.POST("/test", captchaMiddleware(provider), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func TestCaptchaMiddleware_ValidToken(t *testing.T) {
	provider := &stubProvider{name: "test", siteKey: "key"}
	r := setupCaptchaRouter(provider)

	body, _ := json.Marshal(map[string]string{"captcha_token": "valid"})
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCaptchaMiddleware_MissingToken(t *testing.T) {
	provider := &stubProvider{name: "test", siteKey: "key"}
	r := setupCaptchaRouter(provider)

	body, _ := json.Marshal(map[string]string{"email": "user@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestCaptchaMiddleware_EmptyBody(t *testing.T) {
	provider := &stubProvider{name: "test", siteKey: "key"}
	r := setupCaptchaRouter(provider)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestCaptchaMiddleware_VerificationFailed(t *testing.T) {
	provider := &stubProvider{
		name: "test", siteKey: "key",
		err: captcha.ErrVerificationFailed,
	}
	r := setupCaptchaRouter(provider)

	body, _ := json.Marshal(map[string]string{"captcha_token": "bad"})
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestCaptchaMiddleware_BackendError_FailOpen(t *testing.T) {
	provider := &stubProvider{
		name: "test", siteKey: "key",
		err: errors.New("network timeout"),
	}
	r := setupCaptchaRouter(provider)

	body, _ := json.Marshal(map[string]string{"captcha_token": "token"})
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (fail-open), got %d", w.Code)
	}
}

func TestCaptchaMiddleware_InvalidJSON(t *testing.T) {
	provider := &stubProvider{name: "test", siteKey: "key"}
	r := setupCaptchaRouter(provider)

	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for invalid JSON (no token), got %d", w.Code)
	}
}

func TestCaptchaMiddleware_BodyRestoredForDownstream(t *testing.T) {
	provider := &stubProvider{name: "test", siteKey: "key"}

	r := gin.New()
	r.POST("/test", captchaMiddleware(provider), func(c *gin.Context) {
		var parsed map[string]string
		if err := c.ShouldBindJSON(&parsed); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, parsed)
	})

	payload := map[string]string{"captcha_token": "tok", "email": "user@test.com"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["email"] != "user@test.com" {
		t.Fatalf("expected email in downstream body, got %v", resp)
	}
}

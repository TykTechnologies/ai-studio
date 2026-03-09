package captcha

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

var httpClient = &http.Client{Timeout: 5 * time.Second}

// verifyResponse is the common response shape from reCAPTCHA, hCaptcha, and Turnstile.
type verifyResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
	Score      float64  `json:"score"`
	Action     string   `json:"action"`
	Hostname   string   `json:"hostname"`
}

// HTTPProviderConfig configures a shared HTTP-based CAPTCHA verifier.
type HTTPProviderConfig struct {
	Name      string
	SiteKey   string
	SecretKey string
	VerifyURL string
	MinScore  float64 // only used by reCAPTCHA v3 (0 = disabled)
}

// NewHTTPProvider creates a Provider that verifies tokens via HTTP POST.
// reCAPTCHA v2/v3, hCaptcha, and Cloudflare Turnstile all share this protocol.
func NewHTTPProvider(cfg HTTPProviderConfig) Provider {
	return &httpProvider{
		name:      cfg.Name,
		siteKey:   cfg.SiteKey,
		secretKey: cfg.SecretKey,
		verifyURL: cfg.VerifyURL,
		minScore:  cfg.MinScore,
	}
}

type httpProvider struct {
	name      string
	siteKey   string
	secretKey string
	verifyURL string
	minScore  float64
}

func (p *httpProvider) Name() string    { return p.name }
func (p *httpProvider) SiteKey() string { return p.siteKey }

func (p *httpProvider) Verify(ctx context.Context, token, remoteIP string) error {
	if token == "" {
		return fmt.Errorf("%w: empty token", ErrVerificationFailed)
	}

	form := url.Values{
		"secret":   {p.secretKey},
		"response": {token},
	}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.verifyURL, nil)
	if err != nil {
		return fmt.Errorf("captcha request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.URL.RawQuery = form.Encode()

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("captcha verify request: %w", err)
	}
	defer resp.Body.Close()

	var result verifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("captcha decode response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("%w: %v", ErrVerificationFailed, result.ErrorCodes)
	}

	if p.minScore > 0 && result.Score < p.minScore {
		return fmt.Errorf("%w: score %.2f below threshold %.2f", ErrVerificationFailed, result.Score, p.minScore)
	}

	return nil
}

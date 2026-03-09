package mcaptcha

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/v2/captcha"
)

func init() {
	captcha.Register("mcaptcha", newMCaptcha)
}

var httpClient = &http.Client{Timeout: 5 * time.Second}

type provider struct {
	siteKey     string
	secret      string
	instanceURL string
}

func (p *provider) Name() string    { return "mcaptcha" }
func (p *provider) SiteKey() string { return p.siteKey }

type verifyRequest struct {
	Secret string `json:"secret"`
	Key    string `json:"key"`
	Token  string `json:"token"`
}

type verifyResponse struct {
	Valid bool `json:"valid"`
}

func (p *provider) Verify(ctx context.Context, token, _ string) error {
	if token == "" {
		return fmt.Errorf("%w: empty token", captcha.ErrVerificationFailed)
	}

	body, err := json.Marshal(verifyRequest{
		Secret: p.secret,
		Key:    p.siteKey,
		Token:  token,
	})
	if err != nil {
		return fmt.Errorf("mcaptcha marshal: %w", err)
	}

	url := p.instanceURL + "/api/v1/pow/siteverify"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("mcaptcha request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("mcaptcha verify request: %w", err)
	}
	defer resp.Body.Close()

	var result verifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("mcaptcha decode response: %w", err)
	}

	if !result.Valid {
		return fmt.Errorf("%w: token rejected", captcha.ErrVerificationFailed)
	}

	return nil
}

func newMCaptcha(siteKey, secretKey string, opts map[string]string) (captcha.Provider, error) {
	instanceURL := opts["instance_url"]
	if instanceURL == "" {
		return nil, fmt.Errorf("mcaptcha: instance_url is required")
	}
	return &provider{
		siteKey:     siteKey,
		secret:      secretKey,
		instanceURL: instanceURL,
	}, nil
}

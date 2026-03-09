package turnstile

import "github.com/TykTechnologies/midsommar/v2/captcha"

func init() {
	captcha.Register("turnstile", newTurnstile)
}

func newTurnstile(siteKey, secretKey string, _ map[string]string) (captcha.Provider, error) {
	return captcha.NewHTTPProvider(captcha.HTTPProviderConfig{
		Name:      "turnstile",
		SiteKey:   siteKey,
		SecretKey: secretKey,
		VerifyURL: "https://challenges.cloudflare.com/turnstile/v0/siteverify",
	}), nil
}

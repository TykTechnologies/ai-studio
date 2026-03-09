package hcaptcha

import "github.com/TykTechnologies/midsommar/v2/captcha"

func init() {
	captcha.Register("hcaptcha", newHCaptcha)
}

func newHCaptcha(siteKey, secretKey string, _ map[string]string) (captcha.Provider, error) {
	return captcha.NewHTTPProvider(captcha.HTTPProviderConfig{
		Name:      "hcaptcha",
		SiteKey:   siteKey,
		SecretKey: secretKey,
		VerifyURL: "https://api.hcaptcha.com/siteverify",
	}), nil
}

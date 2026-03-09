package recaptcha

import (
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/captcha"
)

func init() {
	captcha.Register("recaptcha_v2", newRecaptchaV2)
	captcha.Register("recaptcha_v3", newRecaptchaV3)
}

func newRecaptchaV2(siteKey, secretKey string, _ map[string]string) (captcha.Provider, error) {
	return captcha.NewHTTPProvider(captcha.HTTPProviderConfig{
		Name:      "recaptcha_v2",
		SiteKey:   siteKey,
		SecretKey: secretKey,
		VerifyURL: "https://www.google.com/recaptcha/api/siteverify",
	}), nil
}

func newRecaptchaV3(siteKey, secretKey string, opts map[string]string) (captcha.Provider, error) {
	minScore := 0.5
	if s, ok := opts["min_score"]; ok {
		if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
			minScore = v
		}
	}
	return captcha.NewHTTPProvider(captcha.HTTPProviderConfig{
		Name:      "recaptcha_v3",
		SiteKey:   siteKey,
		SecretKey: secretKey,
		VerifyURL: "https://www.google.com/recaptcha/api/siteverify",
		MinScore:  minScore,
	}), nil
}

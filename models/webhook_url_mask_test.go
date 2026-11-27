package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskWebhookURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://hooks.slack.com/services/T0123ABC/B0456DEF/xYz789secretTokenValue":                                                   "https://hooks.slack.com/services/***",
		"https://discord.com/api/webhooks/123456789012345678/abcDEF-ghi_jkl":                                                          "https://discord.com/api/webhooks/***",
		"https://contoso.webhook.office.com/webhookb2/aaaa@bbbb/IncomingWebhook/cc/dd":                                                "https://contoso.webhook.office.com/webhookb2/***",
		"https://prod-12.westeurope.logic.azure.com:443/workflows/abc/triggers/manual/paths/invoke?api-version=2016-06-01&sig=SECRET": "https://prod-12.westeurope.logic.azure.com:443/***?api-version=***&sig=***",
		"https://hooks.zapier.com/hooks/catch/123/abcdef/":                                                                            "https://hooks.zapier.com/hooks/catch/***",
		"https://chat.googleapis.com/v1/spaces/AAA/messages?key=K&token=T":                                                            "https://chat.googleapis.com/***?key=***&token=***",
		// Any host: query values, user info and token-like segments.
		"https://receiver.example.com/events?token=abc":                    "https://receiver.example.com/events?token=***",
		"https://user:pass@receiver.example.com/events":                    "https://***@receiver.example.com/events",
		"https://chat.example.com/hooks/k7x3m9p2q8r5t1w4y6z0abcd":          "https://chat.example.com/hooks/***",
		"https://chat.example.com/hooks/k7x3m9p2q8r5t1w4y6z0abcd/user/123": "https://chat.example.com/hooks/***",
		"https://receiver.example.com/webhooks/incoming-events/v1":         "https://receiver.example.com/webhooks/incoming-events/v1",
		"https://receiver.example.com/":                                    "https://receiver.example.com/",
		"https://receiver.example.com/a#frag":                              "https://receiver.example.com/a",
		"not a url":                                                        WebhookURLMask,
		"":                                                                 "",
	} {
		got := MaskWebhookURL(in)
		assert.Equal(t, want, got, in)
		assert.NotContains(t, got, "secret", in)
		assert.Equal(t, got, MaskWebhookURL(got), "idempotent: %s", in)
	}
	assert.True(t, WebhookURLIsMasked("https://hooks.slack.com/services/***"))
	assert.False(t, WebhookURLIsMasked("https://hooks.slack.com/services/T/B/x"))
}

func TestMaskWebhookURLsInText(t *testing.T) {
	got := MaskWebhookURLsInText(`Post "https://hooks.slack.com/services/T1/B2/secret": dial tcp: lookup hooks.slack.com: no such host`)
	assert.Equal(t, `Post "https://hooks.slack.com/services/***": dial tcp: lookup hooks.slack.com: no such host`, got)
	assert.Equal(t, "status 500", MaskWebhookURLsInText("status 500"))
}

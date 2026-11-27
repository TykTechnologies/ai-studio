package models

import (
	"net/url"
	"regexp"
	"strings"
)

// WebhookURLMask replaces the secret parts of a webhook URL.
const WebhookURLMask = "***"

// secretPathHosts are webhook hosts whose URL path is itself the
// credential: anyone holding the URL can post to the channel. The value is
// how many leading path segments are a fixed, non-secret prefix and stay
// visible (hooks.slack.com/services/***).
var secretPathHosts = []struct {
	host   string // exact host, or ".suffix"
	prefix int
}{
	{"hooks.slack.com", 1},     // /services/T.../B.../token
	{"discord.com", 2},         // /api/webhooks/id/token
	{"discordapp.com", 2},      // legacy Discord host
	{"ptb.discord.com", 2},     // Discord test builds
	{"canary.discord.com", 2},  // Discord canary
	{".webhook.office.com", 1}, // /webhookb2/guid@guid/IncomingWebhook/...
	{"outlook.office.com", 1},  // /webhook/guid@guid/IncomingWebhook/...
	{".logic.azure.com", 0},    // Teams workflows (sig= in the query too)
	{"hooks.zapier.com", 2},    // /hooks/catch/id/key
	{"chat.googleapis.com", 0}, // key= and token= in the query
	{"api.telegram.org", 0},    // /bot<token>/sendMessage
	{"maker.ifttt.com", 2},     // /trigger/event/with/key/<key>
	{"webhook.site", 0},        // the path is the inbox id
}

// MaskWebhookURL returns a webhook URL with its credentials hidden, for
// every place a target URL is shown or kept outside the target row: API
// responses, the delivery log, notifications and logs. It keeps the scheme
// and host (what an operator needs to recognise a target) and masks:
//   - user info (a password in https://user:pass@host),
//   - every query value (keys stay: ?token=***),
//   - the fragment,
//   - the path of hosts whose path is the credential (Slack, Discord, Teams,
//     Zapier...), past their fixed prefix,
//   - any path segment that looks like a token (long and high-entropy), on
//     any host, so self-hosted chat webhooks (Mattermost /hooks/<key>,
//     Rocket.Chat /hooks/<id>/<token>) are covered too.
//
// It is idempotent: masking a masked URL returns it unchanged. A value that
// does not parse as an absolute URL is masked entirely.
func MaskWebhookURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return WebhookURLMask
	}
	var b strings.Builder
	b.WriteString(u.Scheme)
	b.WriteString("://")
	if u.User != nil {
		b.WriteString(WebhookURLMask)
		b.WriteString("@")
	}
	b.WriteString(u.Host)

	segments := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(segments) == 1 && segments[0] == "" {
		segments = nil
	}
	keep := len(segments)
	if p, ok := secretPathPrefix(strings.ToLower(u.Hostname())); ok && p < keep {
		keep = p
	}
	for i, seg := range segments {
		b.WriteString("/")
		if i >= keep || looksLikeToken(seg) {
			// Everything from the first secret segment on is masked as
			// one: what follows a token can name what it unlocks.
			b.WriteString(WebhookURLMask)
			keep = i
			break
		}
		b.WriteString(seg)
	}
	if strings.HasSuffix(u.EscapedPath(), "/") && keep >= len(segments) {
		b.WriteString("/")
	}
	if u.RawQuery != "" {
		b.WriteString("?")
		for i, pair := range strings.Split(u.RawQuery, "&") {
			if i > 0 {
				b.WriteString("&")
			}
			key, _, hasValue := strings.Cut(pair, "=")
			b.WriteString(key)
			if hasValue {
				b.WriteString("=")
				b.WriteString(WebhookURLMask)
			}
		}
	}
	return b.String()
}

// WebhookURLIsMasked reports whether s carries the mask, i.e. came from
// MaskWebhookURL rather than from the user.
func WebhookURLIsMasked(s string) bool {
	return strings.Contains(s, WebhookURLMask)
}

func secretPathPrefix(host string) (int, bool) {
	for _, h := range secretPathHosts {
		if strings.HasPrefix(h.host, ".") {
			if strings.HasSuffix(host, h.host) || host == strings.TrimPrefix(h.host, ".") {
				return h.prefix, true
			}
			continue
		}
		if host == h.host {
			return h.prefix, true
		}
	}
	return 0, false
}

// looksLikeToken reports whether a path segment is probably a secret: 20 or
// more characters mixing letters and digits (a Mattermost hook key, a
// generated id). Readable segments (webhook, incoming-events, v1) stay.
func looksLikeToken(seg string) bool {
	if seg == WebhookURLMask || len(seg) < 20 {
		return false
	}
	letters, digits := 0, 0
	for _, r := range seg {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			letters++
		}
	}
	return letters > 0 && digits > 0
}

// urlInTextRe finds http(s) URLs in free text such as a Go transport error
// (Post "https://hooks.slack.com/services/...": dial tcp ...).
var urlInTextRe = regexp.MustCompile(`https?://[^\s"'<>]+`)

// MaskWebhookURLsInText masks every URL in a free-text message (a delivery
// error), so an error that quotes the target URL does not carry its
// credential.
func MaskWebhookURLsInText(s string) string {
	if !strings.Contains(s, "://") {
		return s
	}
	return urlInTextRe.ReplaceAllStringFunc(s, MaskWebhookURL)
}

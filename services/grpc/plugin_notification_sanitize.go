package grpc

import (
	"regexp"
	"strings"
)

// Plugin-authored notification content is rendered as Markdown in the admin UI.
// The renderer (react-markdown without rehype-raw) already refuses raw HTML and
// javascript: links, but plugins are not trusted input, so the same guarantees
// are applied before the content is stored: raw HTML tags are removed and link
// destinations with script-capable schemes are neutralised. Ordinary Markdown
// (emphasis, lists, http(s)/mailto links, code) passes through unchanged.
var (
	// An HTML tag: optional "/", a tag name, then attributes or nothing. Requiring
	// the name to be followed by whitespace, "/" or ">" keeps Markdown autolinks
	// such as <https://example.com> and comparisons like "a < b > c" intact.
	htmlTagPattern = regexp.MustCompile(`<\s*/?\s*(?:[a-zA-Z][a-zA-Z0-9-]*(?:\s[^<>]*)?|![^<>]*|\?[^<>]*)\s*/?\s*>`)
	// Markdown link/image destinations: ](  javascript:... ) and autolinks <javascript:...>.
	unsafeLinkPattern     = regexp.MustCompile(`(?i)\]\(\s*(?:javascript|vbscript|data|file)\s*:`)
	unsafeAutolinkPattern = regexp.MustCompile(`(?i)<\s*(?:javascript|vbscript|data|file)\s*:[^>]*>`)
)

// sanitizePluginNotificationContent returns Markdown safe to hand to the admin UI.
func sanitizePluginNotificationContent(content string) string {
	if content == "" {
		return content
	}
	// Autolinks with unsafe schemes are dropped before generic tag stripping so
	// their payload does not survive as plain text with a live scheme.
	out := unsafeAutolinkPattern.ReplaceAllString(content, "")
	out = htmlTagPattern.ReplaceAllString(out, "")
	out = unsafeLinkPattern.ReplaceAllString(out, "](#")
	return strings.TrimSpace(out)
}

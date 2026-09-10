package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizePluginNotificationContent(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"plain markdown passes through": {
			"**alice** asked for access to [Triage Agent](/portal/assets/ast_1) – see https://example.com",
			"**alice** asked for access to [Triage Agent](/portal/assets/ast_1) – see https://example.com",
		},
		"script tags are removed": {
			"hello <script>alert(1)</script> world", "hello alert(1) world",
		},
		"event handlers on tags are removed with the tag": {
			`<img src=x onerror="alert(1)"> title`, "title",
		},
		"closing and self-closing tags": {
			"a <b>bold</b> <br/> c", "a bold  c",
		},
		"javascript link destination is neutralised": {
			"[click](javascript:alert(1))", "[click](#alert(1))",
		},
		"scheme check is case and whitespace insensitive": {
			"[x]( JavaScript:alert(1))", "[x](#alert(1))",
		},
		"data url image": {
			"![x](data:text/html;base64,AAAA)", "![x](#text/html;base64,AAAA)",
		},
		"javascript autolink is dropped": {
			"see <javascript:alert(1)> now", "see  now",
		},
		"http autolink and comparisons survive": {
			"cost < 5 and > 2, see <https://example.com>", "cost < 5 and > 2, see <https://example.com>",
		},
		"empty": {"", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, sanitizePluginNotificationContent(tc.in))
		})
	}
}

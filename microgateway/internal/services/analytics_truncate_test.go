package services

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

// A stored body cut inside a multi-byte character is not valid UTF-8, and the
// analytics pulse could not send it (prod, 2026-10-21).
func TestTruncateBody_CutsOnACharacterBoundary(t *testing.T) {
	h := &MicrogatewaAnalyticsHandler{}
	body := strings.Repeat("é", 10) // 2 bytes each
	for max := 1; max <= len(body); max++ {
		got := h.truncateBody(body, max)
		assert.True(t, utf8.ValidString(got), "max %d: %q", max, got)
		if max < len(body) {
			kept := strings.TrimSuffix(got, "... [truncated]")
			assert.LessOrEqual(t, len(kept), max)
			assert.GreaterOrEqual(t, len(kept), max-1)
			assert.True(t, strings.HasPrefix(body, kept))
		} else {
			assert.Equal(t, body, got)
		}
	}
	assert.Equal(t, "ab... [truncated]", h.truncateBody("abc", 2), "ASCII is cut where it was")
	assert.Equal(t, "", h.truncateBody("abc", 0), "0 disables bodies")
}

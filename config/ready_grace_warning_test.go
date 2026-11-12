package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The readiness grace must be longer than the hold window: otherwise a
// warning at start (never a refusal).
func TestReadyGraceWarning(t *testing.T) {
	for _, tc := range []struct {
		grace, window time.Duration
		warn          bool
	}{
		{30 * time.Second, 15 * time.Second, false}, // the defaults
		{15 * time.Second, 15 * time.Second, true},
		{10 * time.Second, 15 * time.Second, true},
		{0, 15 * time.Second, false}, // grace off: /ready as before 2.3
		{30 * time.Second, 0, false}, // no hold
		{10 * time.Second, 20 * time.Second, true},
	} {
		msg := readyGraceWarning(tc.grace, tc.window)
		assert.Equal(t, tc.warn, msg != "", "grace %s window %s: %q", tc.grace, tc.window, msg)
		if tc.warn {
			assert.Contains(t, msg, "DATABASE_READY_GRACE")
			assert.Contains(t, msg, "DATABASE_WRITE_RETRY_WINDOW")
		}
	}
	// Loading with the defaults does not warn, and loading never fails.
	conf := LoadFrom(func(string) string { return "" })
	assert.Empty(t, readyGraceWarning(conf.DatabaseReadyGrace, conf.DatabaseWriteRetryWindow))
	conf = LoadFrom(func(k string) string {
		return map[string]string{"DATABASE_READY_GRACE": "10s", "DATABASE_WRITE_RETRY_WINDOW": "15s"}[k]
	})
	assert.Equal(t, 10*time.Second, conf.DatabaseReadyGrace)
	assert.NoError(t, conf.Err())
}

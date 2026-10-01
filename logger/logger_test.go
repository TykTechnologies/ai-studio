package logger

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Before Init or Use, Current falls back to zerolog's global logger, so what
// logs through it is never discarded by Log's zero value; after Use it is
// the host's logger.
func TestCurrentFollowsUse(t *testing.T) {
	if configured.Load() {
		t.Skip("the logger was configured before this test")
	}
	if Current() != &log.Logger {
		t.Fatal("Current() before Init/Use is not zerolog's global logger")
	}

	var buf bytes.Buffer
	Use(zerolog.New(&buf))
	t.Cleanup(func() { configured.Store(false); Log = zerolog.Logger{} })
	if Current() != &Log {
		t.Fatal("Current() after Use is not Log")
	}
	Current().Error().Msg("seen by the host")
	if !bytes.Contains(buf.Bytes(), []byte("seen by the host")) {
		t.Fatalf("the host's logger did not get the line: %q", buf.String())
	}
}

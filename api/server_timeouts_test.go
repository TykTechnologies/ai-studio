package api

import (
	"testing"
	"time"
)

// Issue #401: the API server must set ReadHeaderTimeout to defend against
// slow-header (Slowloris) resource exhaustion.

func TestNewHTTPServerTimeouts(t *testing.T) {
	srv := newHTTPServer(":0", nil)

	if srv.ReadHeaderTimeout <= 0 {
		t.Error("ReadHeaderTimeout must be set to a positive duration")
	}
	if srv.ReadHeaderTimeout > 60*time.Second {
		t.Errorf("ReadHeaderTimeout should be at most 60s, got %v", srv.ReadHeaderTimeout)
	}

	if srv.IdleTimeout <= 0 {
		t.Error("IdleTimeout must be set to a positive duration")
	}

	// The API serves long-lived streaming responses (chat/SSE), so global
	// read/write timeouts must remain unset to avoid severing active streams.
	if srv.ReadTimeout != 0 {
		t.Errorf("ReadTimeout must stay 0 to keep streaming endpoints working, got %v", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout must stay 0 to keep streaming endpoints working, got %v", srv.WriteTimeout)
	}

	if srv.Addr != ":0" {
		t.Errorf("Addr not propagated, got %q", srv.Addr)
	}
}

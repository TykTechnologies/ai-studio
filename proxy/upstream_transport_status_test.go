package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamTransportStatus(t *testing.T) {
	assert.Equal(t, http.StatusBadGateway, upstreamTransportStatus(io.EOF))
	assert.Equal(t, http.StatusBadGateway, upstreamTransportStatus(&url.Error{Op: "Post", URL: "http://x", Err: io.EOF}))
	assert.Equal(t, http.StatusGatewayTimeout, upstreamTransportStatus(context.DeadlineExceeded))
	assert.Equal(t, http.StatusGatewayTimeout, upstreamTransportStatus(&url.Error{Op: "Post", URL: "http://x", Err: timeoutErr{}}))
	assert.Equal(t, http.StatusBadGateway, upstreamTransportStatus(errors.New("connection refused")))
}

// A vendor that drops the connection without answering is the upstream's
// failure: both paths return 502, never 500.
func TestUpstreamConnectionDropIsBadGateway(t *testing.T) {
	h := newTimingHarness(t, func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		require.NoError(t, err)
		_ = conn.Close()
	}, false)

	for _, tc := range []struct{ path, body string }{
		{"/llm/rest/primary/v1/chat/completions", failoverChatBody},
		{"/llm/stream/primary/v1/chat/completions", failoverStreamBody},
	} {
		resp, body := h.do(tc.path, tc.body)
		assert.Equal(t, http.StatusBadGateway, resp.StatusCode, "%s: %s", tc.path, body)
		assert.Contains(t, string(body), "failed to make upstream request", tc.path)
	}
}

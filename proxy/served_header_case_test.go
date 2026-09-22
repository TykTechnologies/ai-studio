package proxy

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rawResponseHeaderLines sends body to path over a bare TCP connection and
// returns the status line and header lines exactly as the server wrote them.
// http.Client canonicalises incoming keys, so this is the only way to see
// the case the wire carries.
func (h *failoverHarness) rawResponseHeaderLines(path, body string) []string {
	h.t.Helper()
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", h.port))
	require.NoError(h.t, err)
	defer conn.Close()

	fmt.Fprintf(conn, "POST %s HTTP/1.1\r\nHost: 127.0.0.1\r\nAuthorization: Bearer %s\r\n"+
		"Content-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		path, h.apiKey, len(body), body)

	var lines []string
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		require.NoError(h.t, err)
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return lines
		}
		lines = append(lines, line)
	}
}

func headerLine(lines []string, key string) (string, bool) {
	for _, l := range lines {
		if strings.HasPrefix(l, key+":") {
			return strings.TrimSpace(strings.TrimPrefix(l, key+":")), true
		}
	}
	return "", false
}

// The docs promise X-Tyk-Served-LLM. Header.Set canonicalises the key to
// X-Tyk-Served-Llm, which is what every client saw on the wire; a client
// that reads headers case-sensitively (or a proxy that forwards a fixed
// list) found nothing.
func TestServedHeaders_ExactCaseOnTheWire(t *testing.T) {
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIText("from the fallback"), nil)

	for name, body := range map[string]string{"non-streaming": failoverChatBody, "streaming": failoverStreamBody} {
		t.Run(name, func(t *testing.T) {
			lines := h.rawResponseHeaderLines("/ai/primary/v1/chat/completions", body)
			require.NotEmpty(t, lines)
			assert.True(t, strings.HasPrefix(lines[0], "HTTP/1.1 200"), "status line: %s", lines[0])

			served, ok := headerLine(lines, hdrServedLLM)
			assert.True(t, ok, "exact-case %s missing from: %v", hdrServedLLM, lines)
			assert.Equal(t, "fallback", served)
			_, canonical := headerLine(lines, http.CanonicalHeaderKey(hdrServedLLM))
			assert.False(t, canonical, "canonicalised copy must not be written: %v", lines)

			_, ok = headerLine(lines, hdrServedModel)
			assert.True(t, ok, "%s missing from: %v", hdrServedModel, lines)
			_, ok = headerLine(lines, hdrFailover)
			assert.True(t, ok, "%s missing from: %v", hdrFailover, lines)
		})
	}
}

// clearServedHeaders runs after setServedHeaders on the streaming path when
// the rung fails before its first frame; it has to remove the exact key,
// not only the canonical one Header.Del looks for.
func TestClearServedHeaders_RemovesTheExactKey(t *testing.T) {
	w := &headerOnlyWriter{h: http.Header{}}
	setServedHeaders(w, llmAttempt{slug: "primary", model: "gpt-4o", index: 1})
	require.Equal(t, []string{"primary"}, w.h[hdrServedLLM])
	_, canonical := w.h[http.CanonicalHeaderKey(hdrServedLLM)]
	assert.False(t, canonical)

	clearServedHeaders(w)
	assert.Empty(t, w.h, "all served headers gone: %v", w.h)
}

type headerOnlyWriter struct{ h http.Header }

func (w *headerOnlyWriter) Header() http.Header       { return w.h }
func (w *headerOnlyWriter) Write([]byte) (int, error) { return 0, nil }
func (w *headerOnlyWriter) WriteHeader(int)           {}
func (w *headerOnlyWriter) Flush()                    {}
func (w *headerOnlyWriter) Context() context.Context  { return context.Background() }

// A browser client can only read the served headers when the response
// exposes them.
func TestServedHeaders_ExposedForCORS(t *testing.T) {
	for _, hdr := range []string{hdrServedLLM, hdrServedModel, hdrFailover} {
		assert.Contains(t, servedCORSExposeHeaders, hdr)
		assert.Contains(t, mcpCORSExposeHeaders, hdr)
	}

	hook := NewCORSResponseHook()
	resp, err := hook.OnBeforeWriteHeaders(context.Background(), &HeadersRequest{Headers: map[string]string{}})
	require.NoError(t, err)
	for _, hdr := range []string{hdrServedLLM, hdrServedModel, hdrFailover} {
		assert.Contains(t, resp.Headers["Access-Control-Expose-Headers"], hdr)
	}
}

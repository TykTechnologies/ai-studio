package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A streamed exchange whose caller leaves before the last frame is still an
// exchange the upstream answered and billed. The proxy log used to drop it
// because a failed write to the caller was treated like an upstream failure.
// The /ai/ loopback made this routine: its driver closes the connection as
// soon as it has the finish event, so the inner hop's final write fails
// after a complete response, and the request vanished from the log.
func TestStreaming_ClientGoneStillLogsTheExchange(t *testing.T) {
	// Ten frames, spaced out, so the proxy is still writing after the caller
	// has hung up and sees the failed write.
	slowStream := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < 10; i++ {
			fmt.Fprintf(w, "data: %s\n\n", openAIChunk(fmt.Sprintf("part%d ", i), nil))
			w.(http.Flusher).Flush()
			time.Sleep(60 * time.Millisecond)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		w.(http.Flusher).Flush()
	}
	h := newFailoverHarness(t, slowStream, serveOpenAIText("never"), nil)

	req, err := http.NewRequest(http.MethodPost,
		"http://127.0.0.1:"+strconv.Itoa(h.port)+"/llm/stream/primary/v1/chat/completions",
		bytes.NewBufferString(failoverStreamBody))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	// A fresh transport so closing the body closes this connection rather than
	// returning it to a shared pool.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Read one frame, then hang up with most of the stream unread.
	first, err := bufio.NewReader(resp.Body).ReadString('\n')
	require.NoError(t, err)
	assert.Contains(t, first, "part0")
	require.NoError(t, resp.Body.Close())

	// The exchange is logged as the 200 the upstream returned, against the
	// primary route, with the request body.
	waitForProxyLog(t, h.db, h.app.ID, http.StatusOK)
	logs := h.proxyLogs()
	require.Len(t, logs, 1)
	assert.Equal(t, h.primary.ID, logs[0].LLMID)
	assert.Equal(t, http.StatusOK, logs[0].ResponseCode)
	assert.Contains(t, logs[0].RequestBody, "gpt-4o")
	assert.Len(t, h.primaryVendor.calls(), 1)
}

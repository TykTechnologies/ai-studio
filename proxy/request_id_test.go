package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"

	"github.com/TykTechnologies/midsommar/v2/pkg/requestid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A request through the bridge (/ai/ -> the loopback /llm/call/ hop, where
// analytics run) is answered and logged under one request id, every failover
// attempt included; the caller's X-Request-ID is recorded beside it and never
// overwritten; and a client cannot choose the gateway's id.
func TestRequestID_OneIDPerRequestAcrossTheLoopbackAndFailover(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIText("from the fallback"), nil)

	resp, body := h.post("/ai/primary/v1/chat/completions", failoverChatBody,
		requestid.ClientHeader, "lb-7f3a",
		requestid.Header, "tyai_00000000000000000000000000000000") // a forged id is ignored
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	id := resp.Header.Get(requestid.Header)
	require.True(t, requestid.Valid(id), "response id %q", id)
	assert.NotEqual(t, "tyai_00000000000000000000000000000000", id, "the client chose the request id")
	assert.NotEqual(t, id, resp.Header.Get(requestid.ClientHeader), "our id was written into X-Request-ID")

	waitForProxyLog(t, h.db, h.app.ID, http.StatusServiceUnavailable)
	waitForProxyLog(t, h.db, h.app.ID, http.StatusOK)
	logs := h.proxyLogs()
	require.Len(t, logs, 2)
	for _, l := range logs {
		assert.Equal(t, id, l.RequestID, "attempt on LLM %d", l.LLMID)
		assert.Equal(t, "lb-7f3a", l.ClientRequestID, "attempt on LLM %d", l.LLMID)
	}

	// A second request gets its own id.
	resp2, _ := h.post("/ai/primary/v1/chat/completions", failoverChatBody)
	assert.NotEqual(t, id, resp2.Header.Get(requestid.Header))

	// The ids the loopback hop carries never reach the vendor.
	for _, c := range append(h.primaryVendor.calls(), h.fallbackVendor.calls()...) {
		assert.Empty(t, c.Headers.Get(requestid.Header), "loopback request id sent to the vendor")
		assert.Empty(t, c.Headers.Get(hdrInternalClientRequestID), "loopback client request id sent to the vendor")
	}
}

// On a direct call the caller's X-Request-ID is passed through to the
// vendor, but a forged gateway id header is not.
func TestRequestID_DirectCallSendsOnlyTheClientIDUpstream(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveOpenAIText("ok"), serveOpenAIText("unused"), nil)

	resp, body := h.post("/llm/rest/primary/v1/chat/completions", failoverChatBody,
		requestid.ClientHeader, "lb-direct",
		requestid.Header, "tyai_00000000000000000000000000000000",
		hdrInternalClientRequestID, "forged")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	calls := h.primaryVendor.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "lb-direct", calls[0].Headers.Get(requestid.ClientHeader))
	assert.Empty(t, calls[0].Headers.Get(requestid.Header))
	assert.Empty(t, calls[0].Headers.Get(hdrInternalClientRequestID))
}

func TestRequestIDs_TrustsTheHopOnlyFromTheLoopbackHop(t *testing.T) {
	inherited := requestid.New()
	hop := func(remote, token, id string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/llm/call/x/v1/chat/completions", nil)
		r.RemoteAddr = remote
		if token != "" {
			r.Header.Set(hdrInternalHop, token)
		}
		r.Header.Set(requestid.Header, id)
		r.Header.Set(hdrInternalClientRequestID, "lb-1")
		r.Header.Set(requestid.ClientHeader, "direct-1")
		return r
	}

	id, client := RequestIDs(hop("127.0.0.1:5000", internalHopToken, inherited))
	assert.Equal(t, inherited, id, "the loopback hop keeps the outer hop's id")
	assert.Equal(t, "lb-1", client)

	for name, r := range map[string]*http.Request{
		"no hop token":      hop("127.0.0.1:5000", "", inherited),
		"wrong hop token":   hop("127.0.0.1:5000", "nope", inherited),
		"not over loopback": hop("203.0.113.9:5000", internalHopToken, inherited),
		"not an id of ours": hop("127.0.0.1:5000", internalHopToken, "evil\nline"),
	} {
		id, client := RequestIDs(r)
		assert.NotEqual(t, inherited, id, name)
		assert.True(t, requestid.Valid(id), "%s: %q", name, id)
		if name != "not an id of ours" {
			assert.Equal(t, "direct-1", client, name)
		}
	}
}

func TestWithRequestID_AnswersTheIDAndKeepsOneAlreadyInTheContext(t *testing.T) {
	var seen string
	h := withRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = requestid.FromContext(r.Context())
		w.Header().Set(requestid.ClientHeader, "vendor-req-9") // the vendor's own header stays
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/llm/rest/x/v1/chat/completions", nil))
	assert.True(t, requestid.Valid(seen))
	assert.Equal(t, seen, rec.Header().Get(requestid.Header))
	assert.Equal(t, "vendor-req-9", rec.Header().Get(requestid.ClientHeader))

	// The Microgateway's middleware runs first and puts the id in the context.
	pre := requestid.New()
	r := httptest.NewRequest(http.MethodPost, "/llm/rest/x/v1/chat/completions", nil)
	r = r.WithContext(requestid.WithID(r.Context(), pre, "c"))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	assert.Equal(t, pre, seen)
	assert.Equal(t, pre, rec.Header().Get(requestid.Header))
}

// Start, which serves Studio's embedded gateway on PROXY_PORT, answers the
// request id too, not only embedders that mount Handler.
func TestStart_AnswersTheRequestID(t *testing.T) {
	db, _ := setupParallelTest(t)
	service := services.NewService(db)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())

	p := NewProxy(service, &Config{Port: port}, budget.NewService(db, services.NewTestNotificationService(db)))
	done := make(chan error, 1)
	go func() { done <- p.Start() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
		<-done
	})

	url := fmt.Sprintf("http://127.0.0.1:%d/llm/rest/no-such-llm/v1/chat/completions", port)
	var resp *http.Response
	require.Eventually(t, func() bool {
		resp, err = http.Post(url, "application/json", nil)
		return err == nil
	}, 5*time.Second, 20*time.Millisecond)
	defer resp.Body.Close()
	assert.True(t, requestid.Valid(resp.Header.Get(requestid.Header)), "status %d, id %q", resp.StatusCode, resp.Header.Get(requestid.Header))
}

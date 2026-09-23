package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// timingHarness is one live proxy on a real port in front of one OpenAI-shaped
// fake vendor, with Server-Timing switched on or off.
type timingHarness struct {
	t      *testing.T
	port   int
	apiKey string
}

func newTimingHarness(t *testing.T, serve http.HandlerFunc, serverTiming bool) *timingHarness {
	t.Helper()
	db, cancel := setupTest(t)
	t.Cleanup(func() { tearDownTest(db, cancel) })

	vendor := newFakeVendor(t, serve)
	service := services.NewService(db)
	budgetSvc := budget.NewService(db, services.NewTestNotificationService(db))

	user, err := service.CreateUser(services.UserDTO{Email: "timing@example.com", Name: "timing", Password: "password123"})
	require.NoError(t, err)
	llm := &models.LLM{
		Name: "Primary", Vendor: models.OPENAI, Active: true,
		APIEndpoint: vendor.server.URL, APIKey: "vendor-key", DefaultModel: "gpt-4o",
	}
	require.NoError(t, db.Create(llm).Error)
	app, err := service.CreateApp("Timing App", "", user.ID, nil, []uint{llm.ID}, nil, nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, service.ActivateAppCredential(app.ID))
	app, err = service.GetAppByID(app.ID)
	require.NoError(t, err)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	h := &timingHarness{t: t, port: ln.Addr().(*net.TCPAddr).Port, apiKey: app.Credential.Secret}

	p := NewProxy(service, &Config{Port: h.port, ServerTiming: serverTiming}, budgetSvc)
	require.NoError(t, p.loadResources())
	srv := &http.Server{Handler: p.createHandler()}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		p.waitForAnalyzers()
	})
	return h
}

// do sends a request and reads the body to EOF, so trailers are populated.
func (h *timingHarness) do(path, body string) (*http.Response, []byte) {
	h.t.Helper()
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+strconv.Itoa(h.port)+path, bytes.NewBufferString(body))
	require.NoError(h.t, err)
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(h.t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(h.t, err)
	return resp, b
}

// parseServerTiming turns `a;dur=1.5, b;desc="x"` into {a:"1.5", b:"x"}.
func parseServerTiming(v string) map[string]string {
	out := map[string]string{}
	for _, metric := range strings.Split(v, ",") {
		fields := strings.Split(strings.TrimSpace(metric), ";")
		if fields[0] == "" {
			continue
		}
		out[fields[0]] = ""
		for _, f := range fields[1:] {
			if k, val, ok := strings.Cut(f, "="); ok && (k == "dur" || k == "desc") {
				out[fields[0]] = strings.Trim(val, `"`)
			}
		}
	}
	return out
}

func durOf(t *testing.T, m map[string]string, name string) float64 {
	t.Helper()
	v, ok := m[name]
	require.True(t, ok, "metric %q missing from %v", name, m)
	f, err := strconv.ParseFloat(v, 64)
	require.NoError(t, err)
	return f
}

// delayedOpenAI waits before answering, then streams or returns JSON.
func delayedOpenAI(delay time.Duration, chunks int, gap time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		if !strings.Contains(readAll(r), `"stream":true`) {
			serveOpenAIText("hello world")(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for i := 0; i < chunks; i++ {
			if i > 0 {
				time.Sleep(gap)
			}
			fmt.Fprintf(w, "data: %s\n\n", openAIChunk(fmt.Sprintf("p%d ", i), nil))
			f.Flush()
		}
		stop := "stop"
		fmt.Fprintf(w, "data: %s\n\n", openAIChunk("", &stop))
		fmt.Fprint(w, "data: [DONE]\n\n")
		f.Flush()
	}
}

func readAll(r *http.Request) string {
	b, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(b))
	return string(b)
}

func TestServerTiming_RESTHeaderSplitsGatewayFromUpstream(t *testing.T) {
	h := newTimingHarness(t, delayedOpenAI(60*time.Millisecond, 0, 0), true)

	resp, _ := h.do("/llm/rest/primary/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	m := parseServerTiming(resp.Header.Get("Server-Timing"))
	assert.GreaterOrEqual(t, durOf(t, m, "upstream-ttfb"), 60.0)
	assert.Less(t, durOf(t, m, "gw-pre"), 60.0)
	assert.GreaterOrEqual(t, durOf(t, m, "elapsed"), durOf(t, m, "gw-pre")+durOf(t, m, "upstream-ttfb"))
	assert.Equal(t, "new", m["conn"], "first request opens a connection")
	assert.Empty(t, resp.Header.Get(internalTimingHeader))
}

func TestServerTiming_StreamTrailerHasFullBreakdown(t *testing.T) {
	h := newTimingHarness(t, delayedOpenAI(40*time.Millisecond, 3, 30*time.Millisecond), true)

	resp, body := h.do("/llm/stream/primary/v1/chat/completions", failoverStreamBody)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "p2 ")

	m := parseServerTiming(resp.Trailer.Get("Server-Timing"))
	upstream := durOf(t, m, "upstream")
	total := durOf(t, m, "total")
	assert.GreaterOrEqual(t, upstream, 100.0, "40ms first byte + 2 x 30ms gaps")
	assert.GreaterOrEqual(t, total, upstream)
	assert.InDelta(t, total-upstream, durOf(t, m, "gw"), 0.01)
	assert.GreaterOrEqual(t, durOf(t, m, "gw-ttfb"), 0.0)
	_, hasPre := m["gw-pre"]
	assert.True(t, hasPre)
	assert.Empty(t, resp.Trailer.Get(internalTimingHeader))
}

func TestServerTiming_SecondRequestReusesUpstreamConnection(t *testing.T) {
	h := newTimingHarness(t, delayedOpenAI(0, 0, 0), true)

	h.do("/llm/rest/primary/v1/chat/completions", failoverChatBody)
	resp, _ := h.do("/llm/rest/primary/v1/chat/completions", failoverChatBody)
	assert.Equal(t, "reused", parseServerTiming(resp.Header.Get("Server-Timing"))["conn"])
}

// The /ai/ shim reaches the vendor through a loopback call to /llm/call/. The
// outer response must report the vendor's timing, not the loopback's, and must
// not leak the internal header either as a header or a trailer.
func TestServerTiming_LoopbackReportsVendorTiming(t *testing.T) {
	h := newTimingHarness(t, delayedOpenAI(60*time.Millisecond, 3, 20*time.Millisecond), true)

	for _, body := range []string{failoverChatBody, failoverStreamBody} {
		resp, _ := h.do("/ai/primary/v1/chat/completions", body)
		require.Equal(t, http.StatusOK, resp.StatusCode, body)
		assert.Empty(t, resp.Header.Get(internalTimingHeader))
		assert.Empty(t, resp.Trailer.Get(internalTimingHeader))

		st := resp.Trailer.Get("Server-Timing")
		if st == "" {
			st = resp.Header.Get("Server-Timing")
		}
		m := parseServerTiming(st)
		assert.GreaterOrEqual(t, durOf(t, m, "upstream-ttfb"), 60.0, body)
		// gw-pre spans the outer handler, the loopback and the inner hop's
		// pre-upstream work, so it is positive but well under the vendor wait.
		pre := durOf(t, m, "gw-pre")
		assert.Greater(t, pre, 0.0)
		assert.Less(t, pre, 60.0)
	}
}

func TestServerTiming_OffByDefault(t *testing.T) {
	h := newTimingHarness(t, delayedOpenAI(0, 2, 0), false)

	resp, _ := h.do("/llm/rest/primary/v1/chat/completions", failoverChatBody)
	assert.Empty(t, resp.Header.Get("Server-Timing"))
	resp, _ = h.do("/llm/stream/primary/v1/chat/completions", failoverStreamBody)
	assert.Empty(t, resp.Header.Get("Server-Timing"))
	assert.Empty(t, resp.Trailer.Get("Server-Timing"))
}

func TestInnerTimingRoundTrip(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	rt := &requestTiming{start: base}
	rt.beginUpstream(base.Add(2 * time.Millisecond))
	rt.setConn(true)
	rt.setUpstreamHeaders(base.Add(50 * time.Millisecond))
	rt.setUpstreamFirstBody(base.Add(51 * time.Millisecond))
	rt.setUpstreamEnd(base.Add(90 * time.Millisecond))

	got, ok := parseInnerTiming(rt.innerValue())
	require.True(t, ok)
	assert.True(t, got.start.Equal(rt.upstreamStart))
	assert.True(t, got.headers.Equal(rt.upstreamHeaders))
	assert.True(t, got.firstBody.Equal(rt.upstreamFirstBody))
	assert.True(t, got.end.Equal(rt.upstreamEnd))
	assert.True(t, got.connKnown && got.connReused)

	outer := &requestTiming{start: base}
	outer.beginUpstream(base.Add(1 * time.Millisecond))
	outer.adoptInner(got)
	m := parseServerTiming(outer.trailerValue(base.Add(95 * time.Millisecond)))
	assert.InDelta(t, 2.0, durOf(t, m, "gw-pre"), 0.001)
	assert.InDelta(t, 48.0, durOf(t, m, "upstream-ttfb"), 0.001)
	assert.InDelta(t, 88.0, durOf(t, m, "upstream"), 0.001)
	assert.InDelta(t, 7.0, durOf(t, m, "gw"), 0.001)
	assert.Equal(t, "reused", m["conn"])
}

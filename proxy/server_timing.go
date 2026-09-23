package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Server-Timing instrumentation (opt-in, Config.ServerTiming).
//
// When enabled, every LLM response carries a Server-Timing header that splits
// the request into the gateway's own time and the upstream vendor's time, so a
// benchmark can measure gateway overhead per request instead of inferring it
// from noisy end-to-end differences.
//
// Two places carry the values:
//
//   - The Server-Timing header, written with the response headers. It holds
//     what is known at that moment: gw-pre, upstream-ttfb, conn, and, when the
//     response was buffered, upstream and elapsed.
//   - A Server-Timing trailer, written after the body on chunked responses
//     (streaming, and REST without a Content-Length). It holds the complete
//     picture, including total, gw and gw-ttfb.
//
// Metric names, all in milliseconds:
//
//	gw-pre         request received -> upstream request started
//	upstream-ttfb  upstream request started -> upstream response headers
//	upstream       upstream request started -> upstream body fully read
//	elapsed        request received -> response headers written (header only)
//	gw-ttfb        gateway share of time-to-first-body-byte: client first body
//	               write minus the upstream's own first-body-byte time
//	gw             gateway time outside the upstream window: total - upstream
//	total          request received -> handler returned
//	conn           desc="reused" or "new" upstream connection
//	attempts       upstream round trips when more than one (failover)
//
// The /ai/ and unified-router endpoints reach the vendor through a loopback
// call to /llm/call/. The inner hop reports its absolute upstream timestamps
// to the outer hop in an internal header (and trailer), so the outer hop's
// "upstream" is the vendor, not the loopback, and its gw-pre includes the
// loopback cost. The internal header never reaches the client.

const (
	serverTimingHeader = "Server-Timing"
	// internalTimingHeader carries the inner hop's absolute upstream
	// timestamps back across the loopback. Stripped before the client.
	internalTimingHeader = "X-Tyk-Internal-Timing"
)

type timingCtxKey struct{}

// requestTiming collects timestamps for one gateway request. The trace hooks
// and the response body wrapper can run on transport goroutines, hence the
// mutex.
type requestTiming struct {
	mu sync.Mutex

	start time.Time

	upstreamStart     time.Time
	upstreamHeaders   time.Time
	upstreamFirstBody time.Time
	upstreamEnd       time.Time
	connKnown         bool
	connReused        bool
	attempts          int

	clientFirstBody time.Time
}

func withRequestTiming(ctx context.Context, rt *requestTiming) context.Context {
	return context.WithValue(ctx, timingCtxKey{}, rt)
}

func timingFrom(ctx context.Context) *requestTiming {
	rt, _ := ctx.Value(timingCtxKey{}).(*requestTiming)
	return rt
}

func (rt *requestTiming) beginUpstream(t time.Time) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.attempts++
	rt.upstreamStart = t
	rt.upstreamHeaders = time.Time{}
	rt.upstreamFirstBody = time.Time{}
	rt.upstreamEnd = time.Time{}
	rt.connKnown = false
}

func (rt *requestTiming) setConn(reused bool) {
	rt.mu.Lock()
	rt.connKnown, rt.connReused = true, reused
	rt.mu.Unlock()
}

func (rt *requestTiming) setUpstreamHeaders(t time.Time) {
	rt.mu.Lock()
	if rt.upstreamHeaders.IsZero() {
		rt.upstreamHeaders = t
	}
	rt.mu.Unlock()
}

func (rt *requestTiming) setUpstreamFirstBody(t time.Time) {
	rt.mu.Lock()
	if rt.upstreamFirstBody.IsZero() {
		rt.upstreamFirstBody = t
	}
	rt.mu.Unlock()
}

func (rt *requestTiming) setUpstreamEnd(t time.Time) {
	rt.mu.Lock()
	if rt.upstreamEnd.IsZero() {
		rt.upstreamEnd = t
	}
	rt.mu.Unlock()
}

func (rt *requestTiming) setClientFirstBody(t time.Time) {
	rt.mu.Lock()
	if rt.clientFirstBody.IsZero() {
		rt.clientFirstBody = t
	}
	rt.mu.Unlock()
}

// adoptInner replaces the loopback's timestamps with the inner hop's vendor
// timestamps. Zero values in inner leave the existing value in place.
func (rt *requestTiming) adoptInner(inner innerTiming) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if !inner.start.IsZero() {
		rt.upstreamStart = inner.start
	}
	if !inner.headers.IsZero() {
		rt.upstreamHeaders = inner.headers
	}
	if !inner.firstBody.IsZero() {
		rt.upstreamFirstBody = inner.firstBody
	}
	if !inner.end.IsZero() {
		rt.upstreamEnd = inner.end
	}
	if inner.connKnown {
		rt.connKnown, rt.connReused = true, inner.connReused
	}
	if inner.attempts > 1 {
		rt.attempts = inner.attempts
	}
}

func msSince(from, to time.Time) float64 {
	return float64(to.Sub(from).Microseconds()) / 1000
}

func appendDur(parts []string, name string, ms float64) []string {
	return append(parts, name+";dur="+strconv.FormatFloat(ms, 'f', 3, 64))
}

// headerValue renders what is known when the response headers are written.
func (rt *requestTiming) headerValue(now time.Time) string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	var parts []string
	if !rt.upstreamStart.IsZero() {
		parts = appendDur(parts, "gw-pre", msSince(rt.start, rt.upstreamStart))
		if !rt.upstreamHeaders.IsZero() {
			parts = appendDur(parts, "upstream-ttfb", msSince(rt.upstreamStart, rt.upstreamHeaders))
		}
		if !rt.upstreamEnd.IsZero() {
			parts = appendDur(parts, "upstream", msSince(rt.upstreamStart, rt.upstreamEnd))
		}
	}
	parts = appendDur(parts, "elapsed", msSince(rt.start, now))
	return rt.appendConnLocked(parts)
}

// trailerValue renders the complete breakdown once the handler has returned.
func (rt *requestTiming) trailerValue(end time.Time) string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	var parts []string
	total := msSince(rt.start, end)
	if !rt.upstreamStart.IsZero() {
		parts = appendDur(parts, "gw-pre", msSince(rt.start, rt.upstreamStart))
		if !rt.upstreamHeaders.IsZero() {
			parts = appendDur(parts, "upstream-ttfb", msSince(rt.upstreamStart, rt.upstreamHeaders))
		}
		if !rt.upstreamEnd.IsZero() {
			upstream := msSince(rt.upstreamStart, rt.upstreamEnd)
			parts = appendDur(parts, "upstream", upstream)
			parts = appendDur(parts, "gw", total-upstream)
		}
		if !rt.upstreamFirstBody.IsZero() && !rt.clientFirstBody.IsZero() {
			clientTTFB := msSince(rt.start, rt.clientFirstBody)
			upstreamTTFB := msSince(rt.upstreamStart, rt.upstreamFirstBody)
			parts = appendDur(parts, "gw-ttfb", clientTTFB-upstreamTTFB)
		}
	}
	parts = appendDur(parts, "total", total)
	return rt.appendConnLocked(parts)
}

func (rt *requestTiming) appendConnLocked(parts []string) string {
	if rt.connKnown {
		desc := "new"
		if rt.connReused {
			desc = "reused"
		}
		parts = append(parts, `conn;desc="`+desc+`"`)
	}
	if rt.attempts > 1 {
		parts = append(parts, `attempts;desc="`+strconv.Itoa(rt.attempts)+`"`)
	}
	return strings.Join(parts, ", ")
}

// innerTiming is the loopback wire form: absolute UnixNano timestamps, which
// are comparable because both hops run in the same process.
type innerTiming struct {
	start, headers, firstBody, end time.Time
	connKnown, connReused          bool
	attempts                       int
}

func (rt *requestTiming) innerValue() string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	nano := func(t time.Time) string {
		if t.IsZero() {
			return "0"
		}
		return strconv.FormatInt(t.UnixNano(), 10)
	}
	conn := ""
	if rt.connKnown {
		conn = "new"
		if rt.connReused {
			conn = "reused"
		}
	}
	return fmt.Sprintf("start=%s,headers=%s,first=%s,end=%s,conn=%s,attempts=%d",
		nano(rt.upstreamStart), nano(rt.upstreamHeaders), nano(rt.upstreamFirstBody),
		nano(rt.upstreamEnd), conn, rt.attempts)
}

func parseInnerTiming(v string) (innerTiming, bool) {
	var it innerTiming
	if v == "" {
		return it, false
	}
	ts := func(s string) time.Time {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n == 0 {
			return time.Time{}
		}
		return time.Unix(0, n)
	}
	for _, kv := range strings.Split(v, ",") {
		k, val, _ := strings.Cut(kv, "=")
		switch k {
		case "start":
			it.start = ts(val)
		case "headers":
			it.headers = ts(val)
		case "first":
			it.firstBody = ts(val)
		case "end":
			it.end = ts(val)
		case "conn":
			if val != "" {
				it.connKnown, it.connReused = true, val == "reused"
			}
		case "attempts":
			it.attempts, _ = strconv.Atoi(val)
		}
	}
	return it, true
}

// serverTimingPath reports whether a request path is an LLM route that gets
// timing. Tool, MCP and datasource routes are left alone.
func serverTimingPath(path string) bool {
	return !strings.HasPrefix(path, "/tools/") &&
		!strings.HasPrefix(path, "/datasource/") &&
		!strings.HasPrefix(path, "/.well-known/")
}

// serverTimingMiddleware is the outermost proxy middleware when
// Config.ServerTiming is on. It starts the clock, carries the recorder on the
// request context for timedTransport to fill in, and writes the header and
// trailer.
func serverTimingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !serverTimingPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		rt := &requestTiming{start: time.Now()}
		tw := &timingWriter{
			ResponseWriter: w,
			rt:             rt,
			internal:       r.Header.Get(hdrInternalHop) != "",
		}
		next.ServeHTTP(tw, r.WithContext(withRequestTiming(r.Context(), rt)))
		if !tw.wroteHeader {
			return
		}
		// Keys set with TrailerPrefix after the body are sent as HTTP
		// trailers on chunked responses and dropped otherwise, which is when
		// the header already holds the buffered breakdown.
		h := w.Header()
		h.Set(http.TrailerPrefix+serverTimingHeader, rt.trailerValue(time.Now()))
		if tw.internal {
			h.Set(http.TrailerPrefix+internalTimingHeader, rt.innerValue())
		}
	})
}

// timingWriter stamps the Server-Timing header when the response headers go
// out and records when the first body byte is written.
type timingWriter struct {
	http.ResponseWriter
	rt          *requestTiming
	internal    bool
	wroteHeader bool
}

func (tw *timingWriter) WriteHeader(code int) {
	if !tw.wroteHeader {
		tw.wroteHeader = true
		h := tw.ResponseWriter.Header()
		h.Set(serverTimingHeader, tw.rt.headerValue(time.Now()))
		if tw.internal {
			h.Set(internalTimingHeader, tw.rt.innerValue())
		} else {
			// A handler that copied upstream headers (the /ai/ translator
			// relaying the loopback response) must not leak the internal one.
			h.Del(internalTimingHeader)
		}
	}
	tw.ResponseWriter.WriteHeader(code)
}

func (tw *timingWriter) Write(b []byte) (int, error) {
	if !tw.wroteHeader {
		tw.WriteHeader(http.StatusOK)
	}
	if len(b) > 0 {
		tw.rt.setClientFirstBody(time.Now())
	}
	return tw.ResponseWriter.Write(b)
}

// Flush keeps SSE working through the wrapper.
func (tw *timingWriter) Flush() {
	if !tw.wroteHeader {
		tw.WriteHeader(http.StatusOK)
	}
	if f, ok := tw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (tw *timingWriter) Unwrap() http.ResponseWriter {
	return tw.ResponseWriter
}

// timedTransport records upstream timings on the request's requestTiming.
// With no recorder on the context (timing off) it is a plain passthrough.
type timedTransport struct {
	base http.RoundTripper
}

func (t *timedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt := timingFrom(req.Context())
	if rt == nil {
		return t.base.RoundTrip(req)
	}

	trace := &httptrace.ClientTrace{
		GotConn:              func(info httptrace.GotConnInfo) { rt.setConn(info.Reused) },
		GotFirstResponseByte: func() { rt.setUpstreamHeaders(time.Now()) },
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	rt.beginUpstream(time.Now())
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		rt.setUpstreamEnd(time.Now())
		return nil, err
	}
	resp.Body = &timedBody{
		ReadCloser: resp.Body,
		onFirst:    func() { rt.setUpstreamFirstBody(time.Now()) },
		onDone:     func() { rt.setUpstreamEnd(time.Now()) },
	}
	return resp, nil
}

// loopbackRoundTrip performs the /ai/ -> /llm/call/ hop. When Server-Timing is
// on, the vendor timings come from the inner hop's internal header rather than
// from this round trip, which only measures the loopback.
func loopbackRoundTrip(base http.RoundTripper, req *http.Request) (*http.Response, error) {
	rt := timingFrom(req.Context())
	if rt == nil {
		return base.RoundTrip(req)
	}
	rt.beginUpstream(time.Now())
	resp, err := base.RoundTrip(req)
	if err != nil {
		rt.setUpstreamEnd(time.Now())
		return nil, err
	}
	if inner, ok := parseInnerTiming(resp.Header.Get(internalTimingHeader)); ok {
		rt.adoptInner(inner)
	}
	resp.Header.Del(internalTimingHeader)
	resp.Body = &timedBody{
		ReadCloser: resp.Body,
		onDone: func() {
			// The inner hop's trailer, present on chunked responses, has
			// the vendor's end time. Without it the loopback's own end is
			// the closest bound (slightly late, so gw is under-reported).
			if inner, ok := parseInnerTiming(resp.Trailer.Get(internalTimingHeader)); ok {
				rt.adoptInner(inner)
			}
			rt.setUpstreamEnd(time.Now())
		},
	}
	return resp, nil
}

// timedBody reports the first non-empty read and the end of the body (EOF,
// error or Close, whichever comes first).
type timedBody struct {
	io.ReadCloser
	onFirst func()
	onDone  func()
	first   sync.Once
	done    sync.Once
}

func (b *timedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 && b.onFirst != nil {
		b.first.Do(b.onFirst)
	}
	if err != nil {
		b.done.Do(b.onDone)
	}
	return n, err
}

func (b *timedBody) Close() error {
	b.done.Do(b.onDone)
	return b.ReadCloser.Close()
}

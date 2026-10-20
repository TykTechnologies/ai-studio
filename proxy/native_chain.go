package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
	"github.com/TykTechnologies/midsommar/v2/switches"
)

// Native chain hooks (nativeapi.ChainHook): synchronous calls that stay on the
// legacy native chain - model validation, filters, usage analysis - but hand
// out or reference vendor resource IDs, such as OpenAI's POST /v1/responses.
// The request's references (previous_response_id) are verified and unwrapped
// for the calling App and LLM before the filters run, and the IDs of the
// created object are wrapped in the response, JSON or server-sent events, so
// an App can only ever name its own resources. A created object the vendor's
// spec tracks as async (a background response) is recorded in the native
// resource ledger, and the reconciler accounts it once it ends.

// nativeChain is one synchronous-chain request a chain hook applies to.
type nativeChain struct {
	c    *nativeCall
	hook *nativeapi.ChainHook
	// created is the resource type of the created object (ResponseIDs[0]).
	created    nativeapi.ResourceType
	hasCreated bool

	mu sync.Mutex
	// recorded holds the vendor IDs this request recorded in the ledger, so
	// the events of one stream record a resource once.
	recorded map[string]bool
}

// nativeChainFor returns the chain hook that applies to r on llm, or nil when
// its vendor declares none for the request's method and canonical path.
func (p *Proxy) nativeChainFor(r *http.Request, llm *models.LLM, app *models.App) *nativeChain {
	mode, slug, rest, nv, canonical, hook := resolveChainHook(r, llm)
	if hook == nil {
		return nil
	}
	op := &nativeapi.Operation{
		Name: hook.Name, Method: hook.Method, Path: hook.Path,
		RequestRefs: hook.RequestRefs, ResponseIDs: hook.ResponseIDs, ErrorShape: hook.ErrorShape,
	}
	ch := &nativeChain{
		c: &nativeCall{
			p:     p,
			nr:    &nativeRoute{mode: mode, slug: slug, rest: rest, llm: llm, vendor: nv, canonical: canonical},
			op:    op,
			app:   app,
			store: nativeStoreOf(p.gatewayService),
			r:     r,
		},
		hook:     hook,
		recorded: map[string]bool{},
	}
	ch.created, ch.hasCreated = ch.c.createdResource()
	return ch
}

// nativeChainHookFor returns the chain hook that applies to r on llm, or nil.
func nativeChainHookFor(r *http.Request, llm *models.LLM) *nativeapi.ChainHook {
	_, _, _, _, _, hook := resolveChainHook(r, llm)
	return hook
}

// resolveChainHook matches r's method and canonical vendor path against
// llm's vendor's chain hooks.
func resolveChainHook(r *http.Request, llm *models.LLM) (mode, slug, rest string, nv *nativeVendor, canonical string, hook *nativeapi.ChainHook) {
	mode, slug, rest, ok := splitNativePath(r.URL.Path)
	if !ok {
		return
	}
	if nv = nativeVendorFor(llm.Vendor); nv == nil {
		return
	}
	hooks := nv.matcher.Spec().ChainHooks
	if hooks == nil {
		return
	}
	endpoint, err := url.Parse(switches.UpstreamEndpoint(llm))
	if err != nil {
		return
	}
	canonical = canonicalVendorPath(endpoint.Path, rest)
	hook, _ = hooks.Hook(r.Method, canonical)
	return
}

// fail writes a vendor-shaped error.
func (ch *nativeChain) fail(w http.ResponseWriter, status int, typ, msg string) {
	ch.c.fail(w, status, typ, msg)
}

// prepare sets up the ID codec and verifies and unwraps the request's
// resource references in r's body. On failure it has answered the client.
func (ch *nativeChain) prepare(w http.ResponseWriter, r *http.Request) bool {
	c := ch.c
	c.r = r
	for _, name := range ch.hook.DropHeaders {
		r.Header.Del(name)
	}
	if len(ch.hook.RequestRefs) > 0 || len(ch.hook.ResponseIDs) > 0 {
		codec, err := nativeapi.CodecForLLM(c.nr.llm, time.Now())
		if err != nil {
			ch.fail(w, http.StatusNotImplemented, nativeErrAPI, "this endpoint needs the LLM's signing secret or vendor credential")
			return false
		}
		c.codec = codec
		c.scope = nativeapi.Scope{LLMID: c.nr.llm.ID, AppID: c.app.ID}
	}
	if len(ch.hook.RequestRefs) == 0 && ch.hook.CheckBody == nil {
		return true
	}
	body, err := helpers.CopyRequestBody(r)
	if err != nil {
		ch.fail(w, http.StatusBadRequest, nativeErrInvalidRequest, "failed to read request body")
		return false
	}
	if ch.hook.CheckBody != nil {
		if err := ch.hook.CheckBody(body); err != nil {
			ch.fail(w, http.StatusBadRequest, nativeErrInvalidRequest, "invalid request body: "+err.Error())
			return false
		}
	}
	if len(ch.hook.RequestRefs) == 0 || len(bytes.TrimSpace(body)) == 0 {
		return true
	}
	obj, err := decodeNativeObject(body)
	if err != nil {
		ch.fail(w, http.StatusBadRequest, nativeErrInvalidRequest, "request body is not a JSON object")
		return false
	}
	if k, ok := refKeyVariant(obj, ch.hook.RequestRefs); ok {
		ch.fail(w, http.StatusBadRequest, nativeErrInvalidRequest, fmt.Sprintf("unsupported parameter %q", k))
		return false
	}
	spec := c.nr.vendor.matcher.Spec()
	changed := false
	for _, ref := range ch.hook.RequestRefs {
		rt, _ := spec.Resource(ref.Resource)
		if err := rewriteRef(obj, ref, func(wrapped string) (string, error) {
			if v, ok := c.unwrapID(rt, wrapped); ok {
				changed = true
				noteUnwrappedRef(r.Context(), wrapped, v)
				return v, nil
			}
			return "", errNativeRef
		}); err != nil {
			ch.fail(w, http.StatusNotFound, nativeErrNotFound, nativeNotFoundMsg)
			return false
		}
	}
	if !changed {
		return true
	}
	out, err := json.Marshal(obj)
	if err != nil {
		ch.fail(w, http.StatusInternalServerError, nativeErrAPI, "failed to encode request body")
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(out))
	r.ContentLength = int64(len(out))
	r.Header.Set("Content-Length", strconv.Itoa(len(out)))
	return true
}

// rewritesResponse reports whether the hook wraps IDs in responses. The
// upstream request then must not ask for a compressed response the gateway
// would have to rewrite (the transport asks for gzip itself and decompresses).
func (ch *nativeChain) rewritesResponse() bool {
	return ch != nil && len(ch.hook.ResponseIDs) > 0
}

// rewriteResponse wraps the IDs of a successful vendor response in place: a
// JSON object, or every server-sent event's resource object as it streams
// through. A response the gateway cannot rewrite, or a tracked resource it
// cannot record, is replaced by a vendor-shaped error: no vendor ID reaches
// the client unwrapped.
func (ch *nativeChain) rewriteResponse(resp *http.Response) {
	if !ch.rewritesResponse() || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" && enc != "identity" {
		ch.replace(resp, http.StatusBadGateway, nativeErrAPI, "unsupported upstream content encoding")
		return
	}
	if strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		if ch.hook.EventObject == "" {
			return
		}
		rw := newSSEEventRewriter(resp.Body, nativeMaxObjectBody, ch.rewriteEvent)
		rw.errorEvent = func(err error) []byte {
			var nre *nativeRecordError
			if errors.As(err, &nre) {
				return ch.c.nr.errorShape()(nre.status, nativeErrAPI, nre.msg)
			}
			return ch.c.nr.errorShape()(http.StatusBadGateway, nativeErrAPI, "unexpected upstream response")
		}
		resp.Body = rw
		resp.Header.Del("Content-Length")
		resp.ContentLength = -1
		return
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, nativeMaxObjectBody+1))
	resp.Body.Close()
	if err != nil {
		ch.replace(resp, upstreamTransportStatus(err), nativeErrAPI, "failed to read upstream response")
		return
	}
	if len(raw) > nativeMaxObjectBody {
		ch.replace(resp, http.StatusBadGateway, nativeErrAPI, "upstream response too large")
		return
	}
	out, err := ch.rewriteObject(raw)
	if err != nil {
		var nre *nativeRecordError
		if errors.As(err, &nre) {
			ch.replace(resp, nre.status, nativeErrAPI, nre.msg)
		} else {
			ch.replace(resp, http.StatusBadGateway, nativeErrAPI, "unexpected upstream response")
		}
		return
	}
	setResponseBody(resp, out)
}

// replace turns resp into a vendor-shaped error.
func (ch *nativeChain) replace(resp *http.Response, status int, typ, msg string) {
	resp.StatusCode = status
	resp.Status = fmt.Sprintf("%d %s", status, http.StatusText(status))
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Del("Content-Encoding")
	setResponseBody(resp, ch.c.nr.errorShape()(status, typ, msg))
}

func setResponseBody(resp *http.Response, body []byte) {
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
}

// nativeRecordError is a created resource the gateway could not record.
type nativeRecordError struct {
	status int
	msg    string
}

func (e *nativeRecordError) Error() string { return e.msg }

// rewriteObject wraps the IDs of a resource object and records it when it is
// tracked.
func (ch *nativeChain) rewriteObject(raw []byte) ([]byte, error) {
	obj, err := decodeNativeObject(raw)
	if err != nil {
		return nil, err
	}
	if !ch.c.wrapResponse(obj) {
		return nil, errors.New("response IDs not wrapped")
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	if err := ch.track(raw, out); err != nil {
		return nil, err
	}
	return out, nil
}

// rewriteEvent rewrites the JSON data of one server-sent event: the IDs of
// its resource object are wrapped, and a tracked object is recorded. Data
// without the resource object is returned unchanged.
func (ch *nativeChain) rewriteEvent(data []byte) ([]byte, error) {
	ev, err := decodeNativeObject(data)
	if err != nil {
		return data, nil
	}
	parent := ev
	segs := strings.Split(ch.hook.EventObject, ".")
	for _, seg := range segs[:len(segs)-1] {
		next, ok := parent[seg].(map[string]any)
		if !ok {
			return data, nil
		}
		parent = next
	}
	obj, ok := parent[segs[len(segs)-1]].(map[string]any)
	if !ok {
		return data, nil
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	if !ch.c.wrapResponse(obj) {
		return nil, errors.New("event response IDs not wrapped")
	}
	wrapped, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	if err := ch.track(raw, wrapped); err != nil {
		return nil, err
	}
	return json.Marshal(ev)
}

// track records a created object in the ledger when its type tracks it (a
// background response), once per request. Without a ledger the resource is
// cancelled upstream: it would never be accounted.
func (ch *nativeChain) track(raw, wrapped []byte) error {
	if !ch.hasCreated || !ch.created.Async.IsTracked(raw) {
		return nil
	}
	vendorID := createdVendorID(ch.c.op, raw)
	if vendorID == "" {
		return errors.New("created resource has no id")
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.recorded[vendorID] {
		return nil
	}
	c := ch.c
	ctx := context.WithoutCancel(c.r.Context())
	if c.store == nil {
		slog.Error("native chain: tracked resource created on a gateway without a resource ledger; cancelling it",
			"llm", c.nr.llm.Name, "type", ch.created.Name, "vendor_id", vendorID)
		c.cancelUpstream(ctx, ch.created, vendorID)
		return &nativeRecordError{status: http.StatusNotImplemented,
			msg: "this kind of request is not available on this gateway and was canceled"}
	}
	if err := c.ledgerCreated(ch.created, raw, wrapped); err != nil {
		return &nativeRecordError{status: http.StatusServiceUnavailable,
			msg: fmt.Sprintf("the %s could not be recorded by the gateway and was canceled; retry shortly", nativeNoun(ch.created.Name))}
	}
	ch.recorded[vendorID] = true
	return nil
}

// createdVendorID reads the created object's vendor ID from raw at op's first
// response ID path.
func createdVendorID(op *nativeapi.Operation, raw []byte) string {
	if len(op.ResponseIDs) == 0 {
		return ""
	}
	obj, err := decodeNativeObject(raw)
	if err != nil {
		return ""
	}
	var vendorID string
	_ = rewriteBodyRef(obj, op.ResponseIDs[0].Path, func(s string) (string, error) { vendorID = s; return s, nil })
	return vendorID
}

// nativeNoun names a resource type for messages: "openai.response" is a
// "response", "anthropic.message_batch" a "message batch".
func nativeNoun(resourceType string) string {
	_, name, ok := strings.Cut(resourceType, ".")
	if !ok {
		name = resourceType
	}
	return strings.ReplaceAll(name, "_", " ")
}

// sseEventRewriter rewrites a server-sent event stream one event at a time:
// each event's data lines (joined) are handed to rewrite, and a changed
// payload replaces them as a single data line. It never holds more than one
// event, so a stream keeps flowing event by event; an event larger than max
// bytes ends the stream with an error rather than pass through unrewritten.
type sseEventRewriter struct {
	src     io.ReadCloser
	rewrite func(data []byte) ([]byte, error)
	// errorEvent, when set, renders the data of a last "error" event sent
	// when rewrite fails, so the client is told why the stream ends.
	errorEvent func(err error) []byte
	max        int
	// failed is set once rewrite failed: nothing more is read or sent.
	failed  bool
	in, out []byte
	err     error
	buf     []byte
}

func newSSEEventRewriter(src io.ReadCloser, max int, rewrite func([]byte) ([]byte, error)) *sseEventRewriter {
	return &sseEventRewriter{src: src, rewrite: rewrite, max: max, buf: make([]byte, 32<<10)}
}

var errSSEEventTooLarge = errors.New("server-sent event too large to rewrite")

func (s *sseEventRewriter) Read(p []byte) (int, error) {
	for len(s.out) == 0 {
		if s.failed {
			return 0, s.err
		}
		if end := sseEventEnd(s.in); end >= 0 {
			if end > s.max {
				s.err, s.failed, s.in = errSSEEventTooLarge, true, nil
				return 0, s.err
			}
			if err := s.emit(s.in[:end]); err != nil {
				return 0, err
			}
			s.in = s.in[end:]
			continue
		}
		if s.err != nil {
			if len(s.in) > 0 {
				// A last event without its blank line.
				ev := s.in
				s.in = nil
				if err := s.emit(ev); err != nil {
					return 0, err
				}
				continue
			}
			return 0, s.err
		}
		if len(s.in) > s.max {
			s.err, s.failed, s.in = errSSEEventTooLarge, true, nil
			return 0, s.err
		}
		n, err := s.src.Read(s.buf)
		s.in = append(s.in, s.buf[:n]...)
		if err != nil {
			s.err = err
		}
	}
	n := copy(p, s.out)
	s.out = s.out[n:]
	return n, nil
}

func (s *sseEventRewriter) Close() error { return s.src.Close() }

// emit rewrites one event (with its terminating blank line) into s.out. A
// rewrite error ends the stream.
func (s *sseEventRewriter) emit(ev []byte) error {
	out, err := rewriteSSEEvent(ev, s.rewrite)
	if err != nil {
		s.err, s.failed = err, true
		if s.errorEvent != nil {
			// Delivered before the error ends the stream.
			s.out = append(s.out, "event: error\ndata: "...)
			s.out = append(s.out, s.errorEvent(err)...)
			s.out = append(s.out, "\n\n"...)
			return nil
		}
		return err
	}
	s.out = append(s.out, out...)
	return nil
}

// sseEventEnd is the length of the first complete event in b, including the
// blank line ("\n\n" or "\r\n\r\n") that ends it, or -1. It walks the
// newlines once and stops at the first event: searching the whole buffer for
// each terminator made framing a long buffered stream quadratic.
func sseEventEnd(b []byte) int {
	for i := 0; i < len(b); {
		j := bytes.IndexByte(b[i:], '\n')
		if j < 0 {
			return -1
		}
		k := i + j
		if k+1 < len(b) && b[k+1] == '\n' {
			return k + 2
		}
		if k > 0 && b[k-1] == '\r' && k+2 < len(b) && b[k+1] == '\r' && b[k+2] == '\n' {
			return k + 3
		}
		i = k + 1
	}
	return -1
}

// rewriteSSEEvent applies rewrite to the data of one event. The event's other
// fields and its terminator are kept; data lines are replaced by one data
// line only when rewrite changed the payload.
func rewriteSSEEvent(ev []byte, rewrite func([]byte) ([]byte, error)) ([]byte, error) {
	body := bytes.TrimRight(ev, "\r\n")
	term := ev[len(body):]
	lines := bytes.Split(body, []byte("\n"))
	var data [][]byte
	first := -1
	for i, line := range lines {
		line = bytes.TrimSuffix(line, []byte("\r"))
		v, ok := bytes.CutPrefix(line, []byte("data:"))
		if !ok {
			continue
		}
		if first < 0 {
			first = i
		}
		data = append(data, bytes.TrimPrefix(v, []byte(" ")))
	}
	if first < 0 {
		return ev, nil
	}
	payload := bytes.Join(data, []byte("\n"))
	out, err := rewrite(payload)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(out, payload) {
		return ev, nil
	}
	kept := make([][]byte, 0, len(lines))
	for i, line := range lines {
		trimmed := bytes.TrimSuffix(line, []byte("\r"))
		if bytes.HasPrefix(trimmed, []byte("data:")) {
			if i == first {
				kept = append(kept, append([]byte("data: "), out...))
			}
			continue
		}
		kept = append(kept, trimmed)
	}
	b := bytes.Join(kept, []byte("\n"))
	if len(term) > 0 {
		b = append(b, "\n\n"...) // the end of the last field and the blank line
	}
	return b, nil
}

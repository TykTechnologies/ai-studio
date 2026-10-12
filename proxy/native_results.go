package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
)

// Results downloads (an Anthropic batch's JSONL results): streamed line by
// line, never buffered whole - a results file can be hundreds of megabytes.
// Each line passes the LLM's response filters (a blocked line is replaced by
// the vendor's errored result for the same key) and, on the node that
// created the resource, its usage is aggregated; at the end of a complete
// download the ledger row is settled and the usage recorded, once.

var (
	// nativeResultsIdleTimeout ends a download whose upstream sends nothing
	// for that long. The request timeout does not apply: a large file takes
	// as long as it takes.
	nativeResultsIdleTimeout = 2 * time.Minute
	// nativeResultsWriteTimeout is the write deadline extended before every
	// line to the client.
	nativeResultsWriteTimeout = 2 * time.Minute
)

// llmScopedResponseHook is an optional interface of a ResponseHook: whether
// it acts on an LLM's responses at all. A hook without it is assumed to.
type llmScopedResponseHook interface {
	HasHooksForLLM(llmID uint) bool
}

// responseHooksApply reports whether a response hook may rewrite llm's
// response bodies. A streamed results file cannot be handed to a body hook,
// so such an LLM refuses the download rather than skip its plugins.
func (p *Proxy) responseHooksApply(llmID uint) bool {
	if p.responseHookManager == nil {
		return false
	}
	m, ok := p.responseHookManager.(*DefaultResponseHookManager)
	if !ok {
		return true
	}
	for _, h := range m.hooks {
		s, ok := h.(llmScopedResponseHook)
		if !ok || s.HasHooksForLLM(llmID) {
			return true
		}
	}
	return false
}

// idleTimeoutReader cancels the download when a Read waits longer than the
// idle timeout.
type idleTimeoutReader struct {
	r      io.Reader
	timer  *time.Timer
	idle   time.Duration
	mu     sync.Mutex
	closed bool
}

func newIdleTimeoutReader(r io.Reader, idle time.Duration, onIdle func()) *idleTimeoutReader {
	return &idleTimeoutReader{r: r, idle: idle, timer: time.AfterFunc(idle, onIdle)}
}

func (ir *idleTimeoutReader) Read(b []byte) (int, error) {
	n, err := ir.r.Read(b)
	ir.mu.Lock()
	if !ir.closed {
		ir.timer.Reset(ir.idle)
	}
	ir.mu.Unlock()
	return n, err
}

func (ir *idleTimeoutReader) stop() {
	ir.mu.Lock()
	ir.closed = true
	ir.timer.Stop()
	ir.mu.Unlock()
}

// serveResults streams a results download.
func (c *nativeCall) serveResults(w http.ResponseWriter, path string) {
	llm := c.nr.llm
	if c.p.responseHooksApply(llm.ID) {
		c.fail(w, http.StatusNotImplemented, nativeErrAPI,
			"results downloads are not available on this LLM: its response plugins cannot run on a streamed results file")
		return
	}
	rt, _ := c.nr.vendor.matcher.Spec().Resource(c.op.PathResource)

	// Only the node that created the resource accounts for it; its ledger
	// holds the row. Elsewhere the download is a plain pass-through.
	var row *models.NativeResource
	if rt.Async != nil {
		if got, err := c.store.GetByOwner(c.r.Context(), llm.ID, c.app.ID, rt.Name, c.vendorID); err == nil &&
			!models.NativeAccountingTerminal(got.AccountingState) {
			row = got
		}
	}

	ctx, cancel := context.WithCancel(c.r.Context())
	defer cancel()
	req, err := c.upstreamRequest(ctx, http.MethodGet, path, c.r.URL.Query(), nil)
	if err != nil {
		slog.Error("native upstream request", "llm", llm.Name, "op", c.op.Name, "error", err)
		c.fail(w, http.StatusBadGateway, nativeErrAPI, "upstream endpoint not permitted")
		return
	}
	// The response header must arrive within the request timeout; the body
	// then only has to keep moving.
	headerTimer := time.AfterFunc(c.p.config.llmTimeout(), cancel)
	resp, err := upstreamTransport.RoundTrip(req)
	headerTimer.Stop()
	if err != nil {
		c.fail(w, upstreamTransportStatus(err), nativeErrAPI, "failed to make upstream request")
		return
	}
	defer resp.Body.Close()
	if enc := resp.Header.Get("Content-Encoding"); enc != "" && enc != "identity" {
		c.fail(w, http.StatusBadGateway, nativeErrAPI, "unsupported upstream content encoding")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, nativeMaxObjectBody))
		copyNativeResponseHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(raw)
		c.status, c.respBody = resp.StatusCode, raw
		return
	}

	body := newIdleTimeoutReader(resp.Body, nativeResultsIdleTimeout, cancel)
	defer body.stop()
	copyNativeResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	c.status = resp.StatusCode
	rc := http.NewResponseController(w)

	filtered := c.p.hasResponseFilters(llm)
	usage := newNativeUsage()
	lines, blocked := 0, 0
	var sent int64
	reader := nativeapi.NewLineReader(body, nativeapi.DefaultMaxLineSize)
	complete := false
	for {
		line, err := reader.Next()
		if errors.Is(err, io.EOF) {
			complete = true
			break
		}
		if err != nil {
			slog.Warn("native results download ended early", "llm", llm.Name, "op", c.op.Name, "line", reader.Line(), "error", err)
			break
		}
		out, billed, isBlocked := c.resultLine(line, filtered)
		if out == nil {
			continue
		}
		if row != nil && billed.Billed {
			usage.add(billed)
		}
		if isBlocked {
			blocked++
		}
		_ = rc.SetWriteDeadline(time.Now().Add(nativeResultsWriteTimeout))
		n, err := w.Write(append(out, '\n'))
		sent += int64(n)
		if err != nil {
			slog.Info("native results client went away", "llm", llm.Name, "line", reader.Line(), "error", err)
			break
		}
		_ = rc.Flush()
		lines++
	}
	_ = rc.SetWriteDeadline(time.Time{})
	metrics.RecordNativeResultsBytes(c.r.Context(), string(llm.Vendor), c.op.Name, sent)
	c.respBody = []byte(fmt.Sprintf(`{"results_lines":%d,"blocked_lines":%d,"complete":%t}`, lines, blocked, complete))

	// A partial download proves nothing about the usage; the row stays
	// claimable for the next download or the reconciler.
	if !complete || row == nil {
		return
	}
	actx := context.WithoutCancel(c.r.Context())
	won, err := settleNativeAccounting(actx, c.store, row)
	if err != nil {
		slog.Error("native accounting claim failed", "row", row.ID, "error", err)
		return
	}
	if won {
		c.p.recordAsyncUsage(row, llm, c.app, usage, time.Now())
	}
}

// resultLine filters one results line. It returns the line to send (nil to
// drop it), the line's billed usage and whether a filter blocked it. Usage
// is billed whether or not a filter blocks the line: the vendor charged it.
func (c *nativeCall) resultLine(line []byte, filtered bool) ([]byte, nativeapi.Usage, bool) {
	view, err := c.op.Results.View(line)
	if err != nil {
		if filtered {
			// A line the gateway cannot read cannot be screened.
			slog.Warn("native results line not understood; withheld from the client", "op", c.op.Name, "error", err)
			return nil, nativeapi.Usage{}, false
		}
		return line, nativeapi.Usage{}, false
	}
	if !filtered || view.Message == nil {
		return line, view.Usage, false
	}
	isBlocked, msg, ferr := ExecuteResponseFilters(c.nr.llm, c.p.gatewayService, view.Message, http.StatusOK, false, false, 0, "", c.r)
	if ferr != nil {
		// Response filters fail open on an error, as on the synchronous chain.
		slog.Error("response filter failed on a results line", "op", c.op.Name, "key", view.Key, "error", ferr)
		return line, view.Usage, false
	}
	if !isBlocked {
		return line, view.Usage, false
	}
	replaced, err := c.op.Results.Blocked(line, msg)
	if err != nil {
		return nil, view.Usage, true
	}
	return replaced, view.Usage, true
}

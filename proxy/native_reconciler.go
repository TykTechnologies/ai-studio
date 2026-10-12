package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gosimple/slug"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// The native resource reconciler accounts for async native resources (an
// Anthropic batch) whose results nobody downloads. On the node that created
// them - their ledger rows only exist there - it polls each pending row until
// the vendor reports it ended, then claims the row, streams the results
// through the extension's LineUsage, settles the row and records the usage.
// A results download (native_results.go) settles through the same ledger
// CAS, so whichever finishes first accounts and the other records nothing.
//
// It never follows a vendor-supplied URL: every request is built from the
// extension's canonical paths on the LLM's configured, SSRF-guarded endpoint.

// The reconciler's default timings; proxy.Config overrides each when set.
var (
	// nativeReconcileInterval is how often the reconciler looks for due rows.
	nativeReconcileInterval = 30 * time.Second
	// nativeReconcileBatch caps the rows one pass looks at.
	nativeReconcileBatch = 50
	// nativeReconcileWorkers bounds the rows reconciled concurrently.
	nativeReconcileWorkers = 4
	// nativePollInterval is the wait before polling an unfinished resource
	// again.
	nativePollInterval = 2 * time.Minute
	// nativePollMaxBackoff caps the wait after consecutive failed polls.
	nativePollMaxBackoff = 30 * time.Minute
)

// nativeAccountOutcome is what accountNativeRow did with a row.
type nativeAccountOutcome int

const (
	// nativeAccountFailed: the call failed (its error says why); nothing
	// was settled. It is the zero value, so an error return never reads as
	// a settled outcome.
	nativeAccountFailed nativeAccountOutcome = iota
	// nativeAccountAccounted: this call won the claim and recorded the usage.
	nativeAccountAccounted
	// nativeAccountSettled: the row was already terminal, or another settler
	// holds a live claim (or took it over).
	nativeAccountSettled
	// nativeAccountNotEnded: the vendor still processes the resource; its
	// ledger snapshot was refreshed.
	nativeAccountNotEnded
	// nativeAccountUnaccountable: the results expired (or the vendor no
	// longer serves them) before they were read; the row is settled
	// unaccountable.
	nativeAccountUnaccountable
	// nativeAccountLost: the vendor no longer knows the resource; the row is
	// settled lost.
	nativeAccountLost
)

func (o nativeAccountOutcome) String() string {
	switch o {
	case nativeAccountFailed:
		return "failed"
	case nativeAccountAccounted:
		return "accounted"
	case nativeAccountSettled:
		return "settled"
	case nativeAccountNotEnded:
		return "not_ended"
	case nativeAccountUnaccountable:
		return "unaccountable"
	case nativeAccountLost:
		return "lost"
	}
	return fmt.Sprintf("nativeAccountOutcome(%d)", int(o))
}

// errNativeRowUnresolvable: the row's LLM is not loaded on this node (deleted
// or deactivated) or its vendor no longer declares the resource type. The row
// is retried with backoff until its results expire.
var errNativeRowUnresolvable = errors.New("native resource: LLM or resource type not available")

// StartNativeWorkers starts the native resource reconciler. gate, when not
// nil, is asked before every pass and a pass runs only when it returns true
// (Studio: only the cluster leader reconciles); the ledger CAS keeps
// concurrent settlers correct either way. stop cancels the workers and waits
// for them; usage they recorded is still being analysed, so a host stops the
// workers before it drains the analytics (WaitForAnalytics). It is a no-op
// without a native resource ledger.
func (p *Proxy) StartNativeWorkers(ctx context.Context, gate func() bool) (stop func()) {
	store := nativeStoreOf(p.gatewayService)
	if store == nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(p.config.nativeReconcileInterval())
		defer t.Stop()
		for {
			if gate == nil || gate() {
				p.reconcileNative(ctx, store, time.Now())
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

// reconcileNative runs one reconciler pass: sweep settled rows past their
// results expiry, then reconcile the due rows.
func (p *Proxy) reconcileNative(ctx context.Context, store services.NativeResourceStore, now time.Time) {
	if n, err := store.SweepExpired(ctx, now); err != nil {
		slog.Warn("native ledger sweep failed", "error", err)
	} else if n > 0 {
		slog.Debug("native ledger swept expired rows", "rows", n)
	}
	rows, err := store.Due(ctx, now, nativeClaimLease, p.config.nativeReconcileBatch())
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("native reconciler could not list due rows", "error", err)
		}
		return
	}
	sem := make(chan struct{}, p.config.nativeReconcileWorkers())
	var wg sync.WaitGroup
	for i := range rows {
		if ctx.Err() != nil {
			break
		}
		row := rows[i]
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-sem; wg.Done() }()
			p.reconcileNativeRow(ctx, store, &row, now)
		}()
	}
	wg.Wait()
}

// reconcileNativeRow accounts one row and schedules its next poll.
func (p *Proxy) reconcileNativeRow(ctx context.Context, store services.NativeResourceStore, row *models.NativeResource, now time.Time) {
	outcome, err := p.accountNativeRow(ctx, row)
	if ctx.Err() != nil {
		return // shutting down; the row stays due
	}
	metrics.RecordNativeReconcile(ctx, row.Vendor, row.ResourceType, outcome.String())
	sctx := context.WithoutCancel(ctx)
	switch {
	case err != nil:
		failures := row.PollFailures + 1
		next := now.Add(p.config.nativePollBackoff(failures))
		slog.Warn("native reconciler could not account resource; retrying", "row", row.ID, "type", row.ResourceType,
			"llm_id", row.LLMID, "failures", failures, "next_poll", next, "error", err)
		if serr := store.SchedulePoll(sctx, row.ID, next, failures); serr != nil && !errors.Is(serr, services.ErrNativeResourceNotFound) {
			slog.Warn("native reconciler could not schedule poll", "row", row.ID, "error", serr)
		}
	case outcome == nativeAccountNotEnded:
		if serr := store.SchedulePoll(sctx, row.ID, now.Add(p.config.nativePollInterval()), 0); serr != nil && !errors.Is(serr, services.ErrNativeResourceNotFound) {
			slog.Warn("native reconciler could not schedule poll", "row", row.ID, "error", serr)
		}
	default:
		slog.Debug("native resource reconciled", "row", row.ID, "type", row.ResourceType, "outcome", outcome.String())
	}
}

func (c *Config) nativeReconcileInterval() time.Duration {
	if c == nil || c.NativeReconcileInterval <= 0 {
		return nativeReconcileInterval
	}
	return c.NativeReconcileInterval
}

func (c *Config) nativeReconcileBatch() int {
	if c == nil || c.NativeReconcileBatch <= 0 {
		return nativeReconcileBatch
	}
	return c.NativeReconcileBatch
}

func (c *Config) nativeReconcileWorkers() int {
	if c == nil || c.NativeReconcileWorkers <= 0 {
		return nativeReconcileWorkers
	}
	return c.NativeReconcileWorkers
}

func (c *Config) nativePollInterval() time.Duration {
	if c == nil || c.NativePollInterval <= 0 {
		return nativePollInterval
	}
	return c.NativePollInterval
}

// nativePollMaxBackoff is never below the poll interval.
func (c *Config) nativePollMaxBackoff() time.Duration {
	d := nativePollMaxBackoff
	if c != nil && c.NativePollMaxBackoff > 0 {
		d = c.NativePollMaxBackoff
	}
	return max(d, c.nativePollInterval())
}

// nativePollBackoff is the wait after failures consecutive failed polls: the
// poll interval, doubled per further failure, capped at the max backoff.
func (c *Config) nativePollBackoff(failures int) time.Duration {
	d, ceiling := c.nativePollInterval(), c.nativePollMaxBackoff()
	for i := 1; i < failures && d < ceiling; i++ {
		d *= 2
	}
	return min(d, ceiling)
}

// accountNativeRow brings an async native resource's accounting to an end if
// it can: it polls the vendor object, and once the resource has ended claims
// the row, reads the whole results file for its usage, settles the row and
// records the usage - only when the settle won, so a download or another
// settler racing it never doubles the records. A non-nil error is
// transient (network, vendor 5xx, store): the row stays claimable and the
// caller decides whether to retry.
//
// The delete stage (native list/delete) calls it to account a batch before
// deleting it.
func (p *Proxy) accountNativeRow(ctx context.Context, row *models.NativeResource) (nativeAccountOutcome, error) {
	if models.NativeAccountingTerminal(row.AccountingState) {
		return nativeAccountSettled, nil
	}
	store := nativeStoreOf(p.gatewayService)
	if store == nil {
		return nativeAccountFailed, errors.New("native resource: no ledger")
	}
	if row.ResultsExpireAt != nil && time.Now().After(*row.ResultsExpireAt) {
		slog.Warn("native resource results expired before they were accounted; usage not recorded",
			"row", row.ID, "type", row.ResourceType, "llm_id", row.LLMID, "app_id", row.AppID, "vendor_id", row.VendorResourceID)
		return p.settleNativeTerminal(ctx, store, row, models.NativeAccountingUnaccountable, nativeAccountUnaccountable)
	}

	llm, nv, rt, ok := p.resolveNativeRow(row)
	if !ok {
		return nativeAccountFailed, errNativeRowUnresolvable
	}
	vendorID := row.VendorResourceID

	status, obj, err := p.nativeFetch(ctx, llm, rt.Async.Poll(vendorID))
	if err != nil {
		return nativeAccountFailed, err
	}
	switch {
	case status == http.StatusNotFound:
		slog.Warn("native resource no longer known to the vendor; usage not recorded",
			"row", row.ID, "type", row.ResourceType, "llm_id", row.LLMID, "app_id", row.AppID, "vendor_id", vendorID)
		return p.settleNativeTerminal(ctx, store, row, models.NativeAccountingLost, nativeAccountLost)
	case status < 200 || status >= 300:
		return nativeAccountFailed, fmt.Errorf("vendor answered %d polling the resource", status)
	}
	info, err := rt.Async.State(obj)
	if err != nil {
		return nativeAccountFailed, fmt.Errorf("vendor object not understood: %w", err)
	}
	p.refreshNativeSnapshot(ctx, store, row, llm, nv, rt, obj, info)
	if info.State != nativeapi.StateEnded {
		return nativeAccountNotEnded, nil
	}
	rreq, hasResults := rt.Async.Results(vendorID, obj)
	if !hasResults {
		slog.Warn("native resource ended without results; usage not recorded",
			"row", row.ID, "type", row.ResourceType, "llm_id", row.LLMID, "vendor_id", vendorID)
		return p.settleNativeTerminal(ctx, store, row, models.NativeAccountingUnaccountable, nativeAccountUnaccountable)
	}

	token := newClaimToken()
	won, err := store.Claim(ctx, row.ID, token, time.Now().UTC(), nativeClaimLease)
	if err != nil {
		return nativeAccountFailed, err
	}
	if !won {
		return nativeAccountSettled, nil
	}
	sctx := context.WithoutCancel(ctx)
	usage, status, err := p.nativeResultsUsage(ctx, llm, rt, rreq)
	if err == nil && (status < 200 || status >= 300) && status != http.StatusNotFound && status != http.StatusGone {
		err = fmt.Errorf("vendor answered %d downloading the results", status)
	}
	if err != nil {
		if _, rerr := store.Release(sctx, row.ID, token); rerr != nil {
			slog.Warn("native accounting claim not released; it expires with its lease", "row", row.ID, "error", rerr)
		}
		return nativeAccountFailed, err
	}
	if status == http.StatusNotFound || status == http.StatusGone {
		slog.Warn("native resource results no longer available; usage not recorded",
			"row", row.ID, "type", row.ResourceType, "llm_id", row.LLMID, "vendor_id", vendorID, "status", status)
		if won, err := settleNativeWithRetry(sctx, store, row.ID, token, models.NativeAccountingUnaccountable); err != nil {
			return nativeAccountFailed, err
		} else if !won {
			return nativeAccountSettled, nil
		}
		return nativeAccountUnaccountable, nil
	}
	won, err = settleNativeWithRetry(sctx, store, row.ID, token, models.NativeAccountingAccounted)
	if err != nil {
		return nativeAccountFailed, err
	}
	if !won {
		// The claim outlived its lease and another settler took it over.
		return nativeAccountSettled, nil
	}
	p.recordAsyncUsage(row, llm, p.nativeRowApp(row), usage, time.Now())
	return nativeAccountAccounted, nil
}

// nativeSettleAttempts is how often a settle is tried on a store error
// before the claim is left to expire with its lease.
const nativeSettleAttempts = 4

// settleNativeWithRetry settles a row claimed by token, retrying briefly on a
// store error (a busy SQLite writer, a dropped connection): the claim is still
// ours, and giving up strands the row until the lease runs out. A settle that
// lost the claim (false, nil) is not retried.
func settleNativeWithRetry(ctx context.Context, store services.NativeResourceStore, id uint, token, state string) (bool, error) {
	var err error
	for attempt := 0; attempt < nativeSettleAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return false, err
			case <-time.After(time.Duration(25<<attempt) * time.Millisecond):
			}
		}
		var won bool
		if won, err = store.Settle(ctx, id, token, state, time.Now().UTC()); err == nil {
			return won, nil
		}
	}
	return false, err
}

// settleNativeTerminal claims row and settles it in state, reporting outcome
// when this call won.
func (p *Proxy) settleNativeTerminal(ctx context.Context, store services.NativeResourceStore, row *models.NativeResource, state string, outcome nativeAccountOutcome) (nativeAccountOutcome, error) {
	sctx := context.WithoutCancel(ctx)
	token, now := newClaimToken(), time.Now().UTC()
	won, err := store.Claim(sctx, row.ID, token, now, nativeClaimLease)
	if err != nil {
		return nativeAccountFailed, err
	}
	if !won {
		return nativeAccountSettled, nil
	}
	if won, err = settleNativeWithRetry(sctx, store, row.ID, token, state); err != nil {
		return nativeAccountFailed, err
	} else if !won {
		return nativeAccountSettled, nil
	}
	return outcome, nil
}

// resolveNativeRow finds the loaded LLM, the vendor spec and the async
// resource type of row.
func (p *Proxy) resolveNativeRow(row *models.NativeResource) (*models.LLM, *nativeVendor, nativeapi.ResourceType, bool) {
	llm, ok := p.GetLLMByID(row.LLMID)
	if !ok || llm == nil {
		return nil, nil, nativeapi.ResourceType{}, false
	}
	nv := nativeVendorFor(llm.Vendor)
	if nv == nil {
		return nil, nil, nativeapi.ResourceType{}, false
	}
	rt, ok := nv.matcher.Spec().Resource(row.ResourceType)
	if !ok || rt.Async == nil || rt.Async.Poll == nil || rt.Async.State == nil ||
		rt.Async.Results == nil || rt.Async.LineUsage == nil {
		return nil, nil, nativeapi.ResourceType{}, false
	}
	return llm, nv, rt, true
}

// nativeRowApp is the App a row's usage is recorded against: the stored App,
// or (if it was deleted since) one carrying the row's IDs.
func (p *Proxy) nativeRowApp(row *models.NativeResource) *models.App {
	if app, err := p.gatewayService.GetAppByID(row.AppID); err == nil && app != nil {
		return app
	}
	return &models.App{ID: row.AppID, UserID: row.UserID}
}

// nativeFetch sends a gateway-originated vendor request and reads its (bounded)
// response body.
func (p *Proxy) nativeFetch(ctx context.Context, llm *models.LLM, ur nativeapi.UpstreamReq) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.llmTimeout())
	defer cancel()
	req, err := p.nativeUpstreamRequest(ctx, llm, ur.Method, ur.Path, ur.Query, nil, nil, ur.Header)
	if err != nil {
		return 0, nil, err
	}
	resp, err := upstreamTransport.RoundTrip(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, nativeMaxObjectBody+1))
	if err != nil {
		return 0, nil, err
	}
	if len(raw) > nativeMaxObjectBody {
		return 0, nil, errors.New("vendor response too large")
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" && enc != "identity" {
		return 0, nil, fmt.Errorf("unsupported vendor content encoding %q", enc)
	}
	return resp.StatusCode, raw, nil
}

// nativeResultsUsage streams a results file and aggregates its billed usage.
// It returns the vendor's status; for a non-2xx status the usage is nil. A
// download that does not reach the end is an error: partial usage proves
// nothing.
func (p *Proxy) nativeResultsUsage(ctx context.Context, llm *models.LLM, rt nativeapi.ResourceType, ur nativeapi.UpstreamReq) (*nativeUsage, int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := p.nativeUpstreamRequest(ctx, llm, ur.Method, ur.Path, ur.Query, nil, nil, ur.Header)
	if err != nil {
		return nil, 0, err
	}
	headerTimer := time.AfterFunc(p.config.llmTimeout(), cancel)
	resp, err := upstreamTransport.RoundTrip(req)
	headerTimer.Stop()
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, nativeMaxObjectBody))
		return nil, resp.StatusCode, nil
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" && enc != "identity" {
		return nil, 0, fmt.Errorf("unsupported vendor content encoding %q", enc)
	}
	body := newIdleTimeoutReader(resp.Body, nativeResultsIdleTimeout, cancel)
	defer body.stop()

	usage := newNativeUsage()
	reader := nativeapi.NewLineReader(body, nativeapi.DefaultMaxLineSize)
	unreadable, oversized := 0, 0
	for {
		line, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if errors.Is(err, nativeapi.ErrLineTooLong) {
			// One result the gateway cannot read must not hold the whole
			// batch's accounting back until its results expire: skip it
			// (unbuffered) and account the rest.
			oversized++
			slog.Warn("native results line over the size limit; its usage is not recorded",
				"llm", llm.Name, "type", rt.Name, "line", reader.Line(), "limit_bytes", nativeapi.DefaultMaxLineSize)
			if err := reader.SkipLine(); err != nil {
				return nil, 0, fmt.Errorf("results download ended at line %d: %w", reader.Line(), err)
			}
			continue
		}
		if err != nil {
			return nil, 0, fmt.Errorf("results download ended at line %d: %w", reader.Line(), err)
		}
		u, ok, err := rt.Async.LineUsage(line)
		if err != nil {
			unreadable++
			continue
		}
		if ok {
			usage.add(u)
		}
	}
	if unreadable > 0 || oversized > 0 {
		slog.Warn("native results lines not accounted; their usage is not recorded",
			"llm", llm.Name, "type", rt.Name, "unreadable", unreadable, "oversized", oversized)
	}
	return usage, resp.StatusCode, nil
}

// refreshNativeSnapshot stores the vendor's latest view of row, wrapped the
// way the vendor's retrieve operation hands it to the client, so ledger-served
// listings show the current state. A view that cannot be wrapped keeps the
// previous snapshot.
func (p *Proxy) refreshNativeSnapshot(ctx context.Context, store services.NativeResourceStore, row *models.NativeResource,
	llm *models.LLM, nv *nativeVendor, rt nativeapi.ResourceType, obj []byte, info nativeapi.StateInfo) {
	snapshot := row.Snapshot
	if op := nativeRetrieveOp(nv, rt.Name); op != nil {
		if codec, err := nativeapi.NewCodec(llm.APIKey); err == nil {
			c := &nativeCall{
				p:     p,
				nr:    &nativeRoute{mode: "call", slug: slug.Make(llm.Name), llm: llm, vendor: nv},
				op:    op,
				codec: codec,
				scope: nativeapi.Scope{LLMID: llm.ID, AppID: row.AppID},
			}
			if m, err := decodeNativeObject(obj); err == nil && c.wrapResponse(m) {
				if b, err := json.Marshal(m); err == nil {
					snapshot = string(b)
				}
			}
		}
	}
	endedAt, expireAt := info.EndedAt, info.ResultsExpireAt
	if endedAt == nil {
		endedAt = row.EndedAt
	}
	if expireAt == nil {
		expireAt = row.ResultsExpireAt
	}
	if err := store.UpdateSnapshot(context.WithoutCancel(ctx), row.ID, services.NativeResourceSnapshot{
		State: string(info.State), Snapshot: snapshot, EndedAt: endedAt, ResultsExpireAt: expireAt,
	}); err != nil && !errors.Is(err, services.ErrNativeResourceNotFound) {
		slog.Warn("native ledger snapshot not updated", "row", row.ID, "error", err)
	}
	row.State, row.Snapshot, row.EndedAt, row.ResultsExpireAt = string(info.State), snapshot, endedAt, expireAt
}

// nativeRetrieveOp is the vendor's plain GET of a resource of type name.
func nativeRetrieveOp(nv *nativeVendor, name string) *nativeapi.Operation {
	ops := nv.matcher.Spec().Operations
	for i := range ops {
		op := &ops[i]
		if op.Method == http.MethodGet && op.PathResource == name && op.Ledger == nativeapi.LedgerRead &&
			op.Results == nil && op.List == nil {
			return op
		}
	}
	return nil
}

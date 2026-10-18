package scripting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
)

// Scanning the selected segments with a provider, within the filter's
// budgets and from the verdict cache where it can.
//
// A request-side guardrail screens text whose size and shape the sender
// controls: a tool result can be padded past any limit, or a request filled
// with thousands of them. Two rules keep that from becoming a way past the
// guardrail:
//
//   - Text that is not scanned in full is never treated as clean. A segment
//     over the size cap or the piece cap, or one whose provider call failed
//     in a run that is not small (the sender decides how large a run is, and
//     so how slow its calls are and how many it needs), is "unscanned": it
//     gets a guardrail.unscanned finding and the filter's action applies to
//     it as to any finding, whatever the fail mode. So does text the
//     provider refused (guardrails.ErrInputRejected), in any run.
//   - The fail mode keeps its meaning for what it was made for: a small run
//     (one provider call per trust class, or one parallel wave for a
//     provider that makes a request per segment, see callsWithinSmallRun,
//     and at most budget.single_call_chars of text) that fails for
//     operational reasons (timeout, 5xx, 429, network) passes or blocks the
//     whole request. An ordinary prompt cannot be made slow by padding
//     without growing past that bound.
//
// Local providers (the built-in library) scan every segment in full up to
// localMaxScanBytes() and are exempt from the remote budgets; a larger
// segment is unscanned (a tool result is quarantined, other text blocked
// under block). The response side keeps its single-pass behaviour: no
// cache, no budgets, and every failure follows the fail mode, since the
// model's own output is not padded by the sender. On every path a call that
// does not return after its timeout is given up on, and a remote provider
// that keeps doing so is suspended (guardrail_breaker.go).

// Environment variables (or AppConf.FilterLimits) that bound in-process
// scans. The built-in library scans about 70-330 ms per MiB with every
// category on an M-series laptop (4uc.58); small edges and loaded hosts can
// be several times slower.
const (
	// EnvLocalMaxScanBytes bounds what an in-process provider scans of one
	// segment: 8 MiB by default (about 2M tokens), 64 KiB to 64 MiB.
	EnvLocalMaxScanBytes = "GUARDRAIL_LOCAL_MAX_SCAN_BYTES"
	// EnvLocalScanMsPerMiB is the time per MiB of a call's text an
	// in-process provider is given beyond timeout_ms before the call is
	// cancelled and abandoned: 2000 by default, 100 to 60000. It leaves room
	// for slow hosts while bounding what one call can cost; text the scan
	// does not finish in time is unscanned.
	EnvLocalScanMsPerMiB = "GUARDRAIL_LOCAL_SCAN_MS_PER_MIB"

	defaultLocalMaxScanBytes = 8 << 20
	defaultLocalScanMsPerMiB = 2000
)

// The local scan bounds are read once per process.
var (
	localMaxScanBytes = sync.OnceValue(func() int {
		return boundedTunable(EnvLocalMaxScanBytes, defaultLocalMaxScanBytes, 64<<10, 64<<20)
	})
	localScanAllowance = sync.OnceValue(func() time.Duration {
		return time.Duration(boundedTunable(EnvLocalScanMsPerMiB, defaultLocalScanMsPerMiB, 100, 60000)) * time.Millisecond
	})
)

// boundedTunable is tunableInt brought within [lo, hi].
func boundedTunable(name string, def, lo, hi int) int {
	return min(max(tunableInt(name, def), lo), hi)
}

const (
	// scanParallel bounds the provider calls of one run in flight at once.
	scanParallel = 4
	// maxSeamRunes bounds the overlap scanned across each cut of a split
	// segment, so text placed to straddle a cut is still seen whole.
	maxSeamRunes = 512
	// callGrace is how long a run waits for a call in flight once the
	// call's timeout, or the run's budget, has run out.
	callGrace = 100 * time.Millisecond
	// smallRunWave is the most calls per trust class a small run of a
	// SegmentPerCall provider makes: one parallel wave. It is its own
	// constant, not scanParallel, so more parallelism would never widen the
	// fail mode's exemption; a sender can provoke at most this many calls
	// per class to throttle.
	smallRunWave = 4
)

// Reasons a segment is unscanned, as recorded with its finding.
const (
	unscannedSize          = "size"
	unscannedPieces        = "pieces"
	unscannedTimeBudget    = "time_budget"
	unscannedInputRejected = "input_rejected"
	unscannedProviderPanic = "provider_panic"
	unscannedProviderError = "provider_error"
	// unscannedBusy: an in-process provider's call got no slot in the local
	// scan limiter (guardrail_local.go) within the time budget.
	unscannedBusy = "busy"
	// unscannedSeam: a redacting run found text across a cut that its
	// rewrite did not remove.
	unscannedSeam = "seam"
)

// unscannedDetector is the detector of the finding an unscanned segment
// gets; its compliance event is guardrail.unscanned.
const unscannedDetector = "unscanned"

// scanResult is a run's outcome. Indices are positions in the segments the
// run was given.
type scanResult struct {
	findings  []guardrails.Finding
	rewritten map[int]string
	// unscanned maps a segment to why it was not scanned in full.
	unscanned map[int]string
	// latency is the provider latency summed over every call, parallel,
	// re-sent and seam calls alike, not the run's wall time.
	latency time.Duration
	// err is the operational failure of a small run, on which the caller
	// applies the fail mode, or guardrails.ErrCallBudget when the request's
	// call budget ran out, which refuses the request.
	err error
	// failedTools are the tool results of a small run under fail closed
	// whose own call failed for operational reasons while the rest of the
	// run was scanned: they are quarantined, whatever the filter's action,
	// rather than the whole request blocked. toolErr is that failure.
	failedTools map[int]bool
	toolErr     error
}

// piece is a slice of one segment sent to the provider.
type piece struct {
	parent int // position of the segment in the run's segments
	text   string
	// ordinal is the piece's place in its segment; -1 for a seam, an
	// overlap across a cut whose findings count but whose rewrite is never
	// used.
	ordinal int
}

// call is one provider call's pieces.
type call []piece

// callResult is a provider call's outcome.
type callResult struct {
	verdict guardrails.Verdict
	err     error
	// started is false for a call not made because the time budget ran
	// out. A call refused for another reason (a suspended provider) counts
	// as started, so its failure is not put down to the budget.
	started bool
}

// scanJob is one run.
type scanJob struct {
	ctx       context.Context
	cfg       guardrails.Config
	provider  guardrails.Provider
	caps      guardrails.Capabilities
	segments  []guardrails.Segment
	direction guardrails.Direction
	redact    *guardrails.RedactionSpec
	meta      map[string]any
	cache     *verdictKeyer // nil: no cache for this run
	breaker   *callBreaker  // nil: calls are never suspended
	// tenant is the request's tenantScope, which the breaker and the
	// local scan limiter count by.
	tenant string
	// small is set while the calls being run make a small run: only their
	// abandons count towards the breaker, since a sender can make a large
	// run slow.
	small bool

	res scanResult
	// pieces holds each segment's pieces; rewrites the rewritten text of
	// rewritten pieces, by segment and ordinal.
	pieces   map[int][]piece
	rewrites map[int]map[int]string
	// seamFindings wait until every call is in, so a seam finding already
	// made by a whole piece is not counted twice.
	seamFindings []guardrails.Finding
	anyFailed    bool
	// unpinned marks segments that took a finding the provider did not pin
	// on them; their verdict is not stored, so a clean text sent alongside
	// flagged text is not remembered as flagged.
	unpinned map[int]bool
	// request is set for the request side, where the budgets apply.
	request bool
	// piecesUsed counts the pieces sent, and reserved for seams, against
	// the piece budget.
	piecesUsed int
}

// scan runs the provider over the segments.
func (j *scanJob) scan() scanResult {
	j.res = scanResult{rewritten: map[int]string{}, unscanned: map[int]string{}, failedTools: map[int]bool{}}
	j.pieces = map[int][]piece{}
	j.rewrites = map[int]map[int]string{}
	j.unpinned = map[int]bool{}
	j.request = j.direction == guardrails.DirectionRequest
	request := j.request

	todo := j.fromCache()
	// The caps apply before any call, so what padding achieves does not
	// depend on the provider's latency.
	if request {
		todo = j.capSize(todo)
	}

	callChars := j.callChars()
	pieceMax := j.caps.MaxChars
	if callChars > 0 && (pieceMax <= 0 || callChars < pieceMax) {
		pieceMax = callChars
	}
	// A run that only detects scans the seams across cuts with the pieces.
	// A redacting run scans them afterwards, on the redacted text
	// (scanSeams), since overlapping rewrites cannot be merged.
	withSeams := j.redact == nil
	for _, i := range todo {
		j.pieces[i] = splitSegment(i, j.segments[i].Text, pieceMax, withSeams)
	}
	if request && !j.caps.Local {
		todo = j.capPieces(todo, withSeams)
	}

	var all []piece
	seamsLater := false
	for _, i := range todo {
		all = append(all, j.pieces[i]...)
		seamsLater = seamsLater || (!withSeams && len(j.pieces[i]) > 1)
	}
	calls := j.packByTrust(all, callChars)
	// Small: within single_call_chars and few calls per trust class (see
	// callsWithinSmallRun), so neither the split by trust nor a provider's
	// request per segment makes an ordinary request lose the fail mode.
	small := !seamsLater && runeTotal(all) <= j.cfg.Budget.SingleCallChars && j.callsWithinSmallRun(calls)

	ctx, cancel := j.budgetContext()
	defer cancel()
	j.small = small
	if !j.collect(ctx, calls, j.run(ctx, calls, j.redact), small) {
		return j.res
	}
	j.mergeRewrites(todo)
	if seamsLater {
		if j.scanSeams(ctx, todo, pieceMax); j.res.err != nil {
			return j.res
		}
	}

	for _, i := range todo {
		if _, skipped := j.res.unscanned[i]; skipped || j.res.failedTools[i] {
			continue
		}
		metrics.RecordGuardrailSegments(j.ctx, j.cfg.Provider, segmentKind(j.segments[i]), "scanned", 1)
		// A run that failed anywhere is not trusted to have seen everything.
		if !j.anyFailed && !j.unpinned[i] {
			j.cache.store(j.segments[i], i, j.res)
		}
	}
	for i := range j.res.unscanned {
		metrics.RecordGuardrailSegments(j.ctx, j.cfg.Provider, segmentKind(j.segments[i]), "unscanned", 1)
	}
	for i := range j.res.failedTools {
		metrics.RecordGuardrailSegments(j.ctx, j.cfg.Provider, segmentKind(j.segments[i]), "unscanned", 1)
	}
	return j.res
}

// fromCache answers the segments the cache knows and returns the others
// that have any text.
func (j *scanJob) fromCache() []int {
	var todo []int
	for i, s := range j.segments {
		if strings.TrimSpace(s.Text) == "" {
			continue
		}
		v, ok := j.cache.lookup(i, s)
		if !ok {
			todo = append(todo, i)
			continue
		}
		metrics.RecordGuardrailSegments(j.ctx, j.cfg.Provider, segmentKind(s), "cached", 1)
		for _, f := range v.findings {
			f.SegmentIndex = i
			j.res.findings = append(j.res.findings, f)
		}
		if v.rewritten {
			j.res.rewritten[i] = v.rewrite
		}
	}
	return todo
}

// capSize marks segments too large to scan and returns the others. A remote
// provider scans at most the filter's max_scan_chars of a segment, a local
// one localMaxScanBytes().
func (j *scanJob) capSize(todo []int) []int {
	kept := make([]int, 0, len(todo))
	for _, i := range todo {
		text := j.segments[i].Text
		over := len(text) > localMaxScanBytes()
		if !j.caps.Local {
			over = utf8.RuneCountInString(text) > j.cfg.Budget.MaxScanChars
		}
		if over {
			j.res.unscanned[i] = unscannedSize
			continue
		}
		kept = append(kept, i)
	}
	return kept
}

// capPieces keeps whole segments, in order, while their pieces fit the
// filter's piece budget, and marks the rest unscanned. The seams a redacting
// run sends later count too.
func (j *scanJob) capPieces(todo []int, withSeams bool) []int {
	kept := make([]int, 0, len(todo))
	used := 0
	for _, i := range todo {
		n := len(j.pieces[i])
		if !withSeams && n > 1 {
			n += n - 1
		}
		if used+n > j.cfg.Budget.MaxPieces {
			j.res.unscanned[i] = unscannedPieces
			continue
		}
		used += n
		kept = append(kept, i)
	}
	j.piecesUsed = used
	return kept
}

// callChars is the most text one call carries: the provider's
// MaxCallChars, else guardrails.DefaultCallChars for a remote provider, so
// a large request becomes several calls (a failure then costs only that
// call's segments, and the time budget can run them in parallel); 0 (one
// call) for a local one.
func (j *scanJob) callChars() int {
	switch {
	case j.caps.MaxCallChars > 0:
		return j.caps.MaxCallChars
	case j.caps.Local:
		return 0
	}
	return guardrails.DefaultCallChars
}

// budgetContext bounds the run's provider calls by the filter's time
// budget. Local providers and the response side are bounded per call only.
func (j *scanJob) budgetContext() (context.Context, context.CancelFunc) {
	if !j.request || j.caps.Local {
		return context.WithCancel(j.ctx)
	}
	return context.WithTimeout(j.ctx, time.Duration(j.cfg.Budget.TimeMs)*time.Millisecond)
}

// run makes the calls, scanParallel at a time. A call whose turn comes
// after the budget has run out is not made. A provider that ignores its
// context does not hold the run: a call still out callGrace after its own
// deadline (callWait) is abandoned, on every path, and once the run's budget
// has run out the calls in flight are abandoned callGrace later. An
// abandoned call ends as failed and frees its slot; its goroutine finishes on
// its own, and the filter's breaker counts it (guardrail_breaker.go).
func (j *scanJob) run(ctx context.Context, calls []call, redact *guardrails.RedactionSpec) []callResult {
	type done struct {
		ci int
		r  callResult
		// begun reports that a local call got its slot and is starting.
		begun bool
	}
	results := make([]callResult, len(calls))
	finished := make([]bool, len(calls))
	probes := make([]bool, len(calls))
	// deadlines holds the time each call in flight is abandoned at.
	deadlines := make([]time.Time, len(calls))
	out := make(chan done, 2*len(calls)) // never blocks a late call
	inFlight := 0
	end := func(ci int, r callResult, abandoned bool) {
		results[ci], finished[ci] = r, true
		deadlines[ci] = time.Time{}
		inFlight--
		if abandoned {
			metrics.RecordGuardrailAbandoned(j.ctx, j.cfg.Provider)
		}
		switch {
		case probes[ci] && abandoned && !j.small:
			// A large run's probe says nothing about the provider.
			j.breaker.cancelProbe()
		default:
			j.breaker.done(probes[ci], abandoned && (probes[ci] || j.small), j.tenant)
		}
	}
	abandon := func(ci int, err error) {
		end(ci, callResult{err: err, started: true}, true)
	}

	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	expired := ctx.Done()
	var grace <-chan time.Time
	next := 0
	for {
		for ; next < len(calls) && inFlight < scanParallel; next++ {
			if r, ok := j.admit(ctx, next, probes); !ok {
				results[next], finished[next] = r, true
				continue
			}
			ci := next
			inFlight++
			if !j.caps.Local {
				deadlines[ci] = time.Now().Add(j.callWait(calls[ci]))
				go func() {
					v, err := j.classify(ctx, calls[ci], redact)
					out <- done{ci: ci, r: callResult{verdict: v, err: err, started: true}}
				}()
				continue
			}
			// A local call first waits for a slot, within the time budget;
			// its own deadline starts when it has one.
			queueWait := time.Duration(j.cfg.Budget.TimeMs) * time.Millisecond
			deadlines[ci] = time.Now().Add(queueWait + callGrace)
			go func() {
				waitCtx, cancel := context.WithTimeout(ctx, queueWait)
				small := callRunes(calls[ci]) <= j.cfg.Budget.SingleCallChars
				release, err := currentLocalLimiter(small).acquire(waitCtx, j.tenant, int64(callBytes(calls[ci])))
				cancel()
				if err != nil {
					out <- done{ci: ci, r: callResult{err: err}}
					return
				}
				defer release()
				out <- done{ci: ci, begun: true}
				v, err := j.classify(ctx, calls[ci], redact)
				out <- done{ci: ci, r: callResult{verdict: v, err: err, started: true}}
			}()
		}
		if inFlight == 0 {
			return results
		}
		var first time.Time
		for _, d := range deadlines {
			if !d.IsZero() && (first.IsZero() || d.Before(first)) {
				first = d
			}
		}
		timer.Reset(time.Until(first))

		select {
		case d := <-out:
			switch {
			case finished[d.ci]:
			case d.begun:
				deadlines[d.ci] = time.Now().Add(j.callWait(calls[d.ci]))
			default:
				end(d.ci, d.r, false)
			}
		case <-expired:
			expired, grace = nil, time.After(callGrace)
		case <-grace:
			for ci, d := range deadlines {
				if !d.IsZero() {
					abandon(ci, ctx.Err())
				}
			}
		case now := <-timer.C:
			for ci, d := range deadlines {
				if !d.IsZero() && !now.Before(d) {
					abandon(ci, context.DeadlineExceeded)
				}
			}
		}
	}
}

// admit decides whether call ci is made: not once the budget has run out,
// not beyond the request's call budget (remote providers), and not while the
// filter's calls are suspended. A refused call's result is returned.
func (j *scanJob) admit(ctx context.Context, ci int, probes []bool) (callResult, bool) {
	if err := ctx.Err(); err != nil {
		return callResult{err: err}, false
	}
	ok, probe := j.breaker.allow()
	if !ok {
		return callResult{err: guardrails.ErrProviderSuspended, started: true}, false
	}
	if !j.caps.Local {
		if err := guardrails.TakeCall(ctx); err != nil {
			if probe {
				j.breaker.cancelProbe()
			}
			return callResult{err: err}, false
		}
	}
	probes[ci] = probe
	return callResult{}, true
}

// callTimeout is the deadline of a call's context: timeout_ms, and for an
// in-process provider localScanAllowance() more per MiB of the call's text, so
// a full scan of a large segment is not cut short.
func (j *scanJob) callTimeout(c call) time.Duration {
	ms := j.cfg.TimeoutMs
	if ms <= 0 {
		ms = guardrails.DefaultTimeoutMs
	}
	timeout := time.Duration(ms) * time.Millisecond
	if j.caps.Local {
		timeout += time.Duration(callBytes(c)) * localScanAllowance() / (1 << 20)
	}
	return timeout
}

// callWait is how long a started call is waited for before it is abandoned.
func (j *scanJob) callWait(c call) time.Duration {
	return j.callTimeout(c) + callGrace
}

func callBytes(c call) int {
	n := 0
	for _, p := range c {
		n += len(p.text)
	}
	return n
}

func callRunes(c call) int {
	n := 0
	for _, p := range c {
		n += utf8.RuneCountInString(p.text)
	}
	return n
}

// classify sends one call and records its metrics.
func (j *scanJob) classify(ctx context.Context, c call, redact *guardrails.RedactionSpec) (guardrails.Verdict, error) {
	in := guardrails.Input{
		Segments:  make([]guardrails.Segment, len(c)),
		Direction: j.direction,
		Meta:      j.meta,
		Redact:    redact,
	}
	for k, p := range c {
		in.Segments[k] = j.segments[p.parent].ForProvider(k, p.text, j.caps)
	}
	// The call runs in a goroutine of its own, outside any request's
	// recovery: a provider panic recovered here fails the call instead of
	// ending the process.
	var v guardrails.Verdict
	var err error
	cfg := j.cfg
	cfg.TimeoutMs = int(j.callTimeout(c) / time.Millisecond)
	if safe.CallRedacted("guardrail_provider", func() { v, err = guardrails.Classify(ctx, cfg, j.provider, in) }) {
		v, err = guardrails.Verdict{}, guardrails.ErrProviderPanic
	}
	outcome := "clean"
	switch {
	case err != nil:
		outcome = "error"
	case v.Flagged:
		outcome = "flagged"
	}
	metrics.RecordGuardrail(j.ctx, j.cfg.Provider, outcome, j.cfg.OnDetect, j.callKind(c), v.Latency.Seconds())
	return v, err
}

// collect folds the calls' outcomes into the result. It returns false when
// the run has failed as a whole (res.err set). budget is the run's budget
// context.
func (j *scanJob) collect(budget context.Context, calls []call, results []callResult, small bool) bool {
	ok := j.eachResult(budget, calls, results, small, j.redact, func(c call, r callResult, reason string) bool {
		switch {
		case r.err == nil:
			j.addVerdict(c, r.verdict)
		case reason == "" && j.request && j.cfg.FailMode == guardrails.FailClosed && j.toolsOnly(c):
			// The call of the client's own text may still succeed; if it
			// fails too, the run fails as a whole.
			for _, parent := range parents(c) {
				j.res.failedTools[parent] = true
			}
			j.res.toolErr = &providerError{err: r.err}
		case reason == "":
			j.res.err = &providerError{err: r.err}
			return false
		default:
			for _, p := range c {
				if _, already := j.res.unscanned[p.parent]; !already {
					j.res.unscanned[p.parent] = reason
				}
			}
		}
		return true
	})
	if ok {
		j.addSeamFindings()
	}
	return ok
}

// eachResult hands each call's outcome to use, in order, after what every
// run does with it: adding up the latency, refusing the request when the
// caller's call budget ran out (res.err), classifying a failure (reason, ""
// when the fail mode decides; see recordFailure), and re-sending a refused call of
// several segments one segment per call. The re-sent calls join the end of
// the list (a run that needed them is no longer small), and the refused call
// itself is not handed on. eachResult stops, returning false, when the
// budget ran out or use returns false.
func (j *scanJob) eachResult(budget context.Context, calls []call, results []callResult, small bool, redact *guardrails.RedactionSpec, use func(c call, r callResult, reason string) bool) bool {
	for ci := 0; ci < len(results); ci++ {
		r := results[ci]
		j.res.latency += r.verdict.Latency
		reason := ""
		if r.err != nil {
			if errors.Is(r.err, guardrails.ErrCallBudget) {
				// The caller's budget, not the provider: the request is
				// refused.
				j.res.err = r.err
				return false
			}
			reason = j.recordFailure(budget, r, small)
			if reason == unscannedInputRejected && len(parents(calls[ci])) > 1 {
				more, moreResults := j.resplit(budget, calls[ci], redact)
				calls, results = append(calls, more...), append(results, moreResults...)
				small = false
				continue
			}
		}
		if !use(calls[ci], r, reason) {
			return false
		}
	}
	return true
}

// resplit re-sends the segments of a call the provider refused, one segment
// per call, so that only the text the provider refuses on its own is
// flagged: a tool result built to provoke a rejection must not take the
// user's prompt in the same call down with it. The re-sent pieces count
// against the piece budget and run within the time budget; a segment they do
// not fit is unscanned.
func (j *scanJob) resplit(ctx context.Context, c call, redact *guardrails.RedactionSpec) ([]call, []callResult) {
	j.small = false
	var calls []call
	for _, parent := range parents(c) {
		var group call
		for _, p := range c {
			if p.parent == parent {
				group = append(group, p)
			}
		}
		if j.request && !j.caps.Local && j.piecesUsed+len(group) > j.cfg.Budget.MaxPieces {
			j.markUnscanned(parent, unscannedPieces)
			continue
		}
		j.piecesUsed += len(group)
		calls = append(calls, group)
	}
	return calls, j.run(ctx, calls, redact)
}

// recordFailure records a failed call and classifies it: the reason its
// segments are unscanned, or "" when the fail mode decides the run. The
// error's text is logged at debug level only (see providerError).
func (j *scanJob) recordFailure(budget context.Context, r callResult, small bool) string {
	j.anyFailed = true
	reason := failureReason(budget, r, small, j.request, j.caps.Local)
	slog.Debug("guardrail provider call failed", "provider", j.cfg.Provider, "error", r.err)
	if reason != "" {
		metrics.RecordGuardrailError(j.ctx, j.cfg.Provider, "unscanned")
		slog.Warn("guardrail provider call failed; its segments are unscanned",
			"provider", j.cfg.Provider, "error_class", guardrails.ErrorClass(r.err), "reason", reason)
	}
	return reason
}

// failureReason classifies a failed call: the reason its segments are
// unscanned, or "" when the fail mode decides the run. budget is the run's
// budget context. A local provider's failure is never left to the fail mode:
// it has no outages, and a client can make it slow by loading the host.
func failureReason(budget context.Context, r callResult, small, request, local bool) string {
	switch {
	case !request:
		return ""
	case errors.Is(r.err, guardrails.ErrInputRejected):
		return unscannedInputRejected
	case errors.Is(r.err, guardrails.ErrProviderPanic):
		return unscannedProviderPanic
	case errors.Is(r.err, errLocalBusy):
		return unscannedBusy
	case local && (errors.Is(r.err, context.DeadlineExceeded) || errors.Is(r.err, context.Canceled)):
		return unscannedTimeBudget
	case local:
		return unscannedProviderError
	case small:
		return ""
	case !r.started || errors.Is(budget.Err(), context.DeadlineExceeded):
		return unscannedTimeBudget
	}
	return unscannedProviderError
}

// addVerdict maps one call's findings and rewrites onto the run's segments.
// A finding that names no piece of the call (a verdict on the call as a
// whole, or an index the provider got wrong) applies to every segment in the
// call: it cannot be pinned on one, and pinning it on the first would leave
// the text that caused it untouched.
func (j *scanJob) addVerdict(c call, v guardrails.Verdict) {
	findings := v.Findings
	if v.Flagged && len(findings) == 0 {
		findings = []guardrails.Finding{{Detector: "flagged", Category: guardrails.CategoryOther, Label: "Flagged by provider", Score: 1, Severity: "warning", SegmentIndex: -1}}
	}
	for _, f := range findings {
		f.Span = nil // piece-relative; meaningless on the whole segment
		if f.SegmentIndex < 0 || f.SegmentIndex >= len(c) {
			for _, parent := range parents(c) {
				g := f
				g.SegmentIndex = parent
				j.res.findings = append(j.res.findings, g)
				j.unpinned[parent] = true
			}
			continue
		}
		p := c[f.SegmentIndex]
		f.SegmentIndex = p.parent
		if p.ordinal < 0 {
			j.seamFindings = append(j.seamFindings, f)
			continue
		}
		j.res.findings = append(j.res.findings, f)
	}
	for k, text := range v.Rewritten {
		if k < 0 || k >= len(c) || c[k].ordinal < 0 {
			continue
		}
		p := c[k]
		if j.rewrites[p.parent] == nil {
			j.rewrites[p.parent] = map[int]string{}
		}
		j.rewrites[p.parent][p.ordinal] = text
	}
}

// addSeamFindings keeps the seam findings no whole piece of the same
// segment made with the same detector: those are text across a cut.
func (j *scanJob) addSeamFindings() {
	made := map[int]map[string]bool{}
	for _, f := range j.res.findings {
		if made[f.SegmentIndex] == nil {
			made[f.SegmentIndex] = map[string]bool{}
		}
		made[f.SegmentIndex][f.Detector] = true
	}
	for _, f := range j.seamFindings {
		if !made[f.SegmentIndex][f.Detector] {
			j.res.findings = append(j.res.findings, f)
		}
	}
	j.seamFindings = nil
}

// mergeRewrites reassembles each scanned segment's rewritten pieces into its
// rewrite. A segment left unscanned gets none: its action is not a redaction.
func (j *scanJob) mergeRewrites(todo []int) {
	for _, i := range todo {
		if _, skipped := j.res.unscanned[i]; skipped || len(j.rewrites[i]) == 0 {
			continue
		}
		text := strings.Join(j.pieceTexts(i), "")
		if text != j.segments[i].Text {
			j.res.rewritten[i] = text
		}
	}
}

// pieceTexts is segment i's whole pieces as rewritten.
func (j *scanJob) pieceTexts(i int) []string {
	var out []string
	for _, p := range j.pieces[i] {
		if p.ordinal < 0 {
			continue
		}
		if r, ok := j.rewrites[i][p.ordinal]; ok {
			out = append(out, r)
		} else {
			out = append(out, p.text)
		}
	}
	return out
}

// scanSeams checks a redacting run's work across the cuts: each cut's seam
// is taken from the rewritten text, and a seam the provider still flags
// holds text across the cut that no piece's rewrite removed. That segment
// is unscanned, since what was found cannot be redacted.
func (j *scanJob) scanSeams(ctx context.Context, todo []int, pieceMax int) {
	k := seamRunes(pieceMax)
	var seams []piece
	for _, i := range todo {
		if _, skipped := j.res.unscanned[i]; skipped {
			continue
		}
		texts := j.pieceTexts(i)
		for n := 0; n+1 < len(texts); n++ {
			seams = append(seams, piece{parent: i, text: tailRunes(texts[n], k) + headRunes(texts[n+1], k), ordinal: -1})
		}
	}
	calls := j.packByTrust(seams, j.callChars())
	// Seams are scanned only for a redacting request run that is not small,
	// so a failed seam call always leaves its segments unscanned.
	j.eachResult(ctx, calls, j.run(ctx, calls, nil), false, nil, func(c call, r callResult, reason string) bool {
		if r.err == nil {
			if len(r.verdict.Findings) == 0 && !r.verdict.Flagged {
				return true
			}
			reason = unscannedSeam
		}
		flagged := map[int]bool{}
		for _, f := range r.verdict.Findings {
			if f.SegmentIndex >= 0 && f.SegmentIndex < len(c) {
				flagged[c[f.SegmentIndex].parent] = true
				f.Span = nil
				f.SegmentIndex = c[f.SegmentIndex].parent
				j.res.findings = append(j.res.findings, f)
			}
		}
		for _, parent := range parents(c) {
			if r.err != nil || len(flagged) == 0 || flagged[parent] {
				j.markUnscanned(parent, reason)
			}
		}
		return true
	})
}

// markUnscanned records a segment as unscanned, dropping a rewrite it had.
func (j *scanJob) markUnscanned(i int, reason string) {
	if _, already := j.res.unscanned[i]; !already {
		j.res.unscanned[i] = reason
	}
	delete(j.res.rewritten, i)
}

// unscannedFinding is the finding that puts segment i on record as not
// scanned.
func unscannedFinding(i int, label string) guardrails.Finding {
	return guardrails.Finding{
		Detector: unscannedDetector, Category: guardrails.CategoryOther, Label: label,
		Score: 1, Severity: "warning", SegmentIndex: i,
	}
}

// unscannedFindings are the findings that put unscanned segments on record,
// in segment order.
func (r scanResult) unscannedFindings() []guardrails.Finding {
	out := make([]guardrails.Finding, 0, len(r.unscanned))
	for _, i := range slices.Sorted(maps.Keys(r.unscanned)) {
		out = append(out, unscannedFinding(i, "Not scanned in full ("+r.unscanned[i]+")"))
	}
	return out
}

// unscannedEvents records unscanned segments: one event per reason, with
// the roles and kinds of the segments, how many there were, the largest
// one's size and the limit that applied. Like every guardrail event it
// carries no text.
func unscannedEvents(filterName string, cfg guardrails.Config, segs []guardrails.Segment, unscanned map[int]string, action string) []ComplianceEventOutput {
	type group struct {
		count    int
		maxChars int
		roles    map[string]bool
		kinds    map[string]bool
	}
	groups := map[string]*group{}
	for i, reason := range unscanned {
		g := groups[reason]
		if g == nil {
			g = &group{roles: map[string]bool{}, kinds: map[string]bool{}}
			groups[reason] = g
		}
		g.count++
		g.maxChars = max(g.maxChars, utf8.RuneCountInString(segs[i].Text))
		g.roles[segs[i].Role] = true
		g.kinds[segmentKind(segs[i])] = true
	}
	reasons := make([]string, 0, len(groups))
	for r := range groups {
		reasons = append(reasons, r)
	}
	sort.Strings(reasons)

	severity := "warning"
	if action == guardrails.ActionBlock {
		severity = "critical"
	}
	events := make([]ComplianceEventOutput, 0, len(reasons))
	for _, reason := range reasons {
		g := groups[reason]
		roles, kinds := sortedKeys(g.roles), sortedKeys(g.kinds)
		meta := map[string]interface{}{
			"provider":  cfg.Provider,
			"detector":  unscannedDetector,
			"reason":    reason,
			"count":     g.count,
			"max_chars": g.maxChars,
			"action":    action,
			"roles":     roles,
			"kinds":     kinds,
		}
		switch reason {
		case unscannedSize:
			meta["limit"] = cfg.Budget.MaxScanChars
		case unscannedPieces:
			meta["limit"] = cfg.Budget.MaxPieces
		case unscannedTimeBudget:
			meta["limit"] = cfg.Budget.TimeMs
		}
		events = append(events, ComplianceEventOutput{
			EventType:   "guardrail." + unscannedDetector,
			Severity:    severity,
			Description: fmt.Sprintf("guardrail %q (%s): %d segment(s) in %s not scanned in full (%s); action %s", filterName, cfg.Provider, g.count, strings.Join(roles, ","), reason, action),
			Metadata:    meta,
		})
	}
	return events
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// providerError is a failed provider call. Its text names the error's class
// only: a provider's error may echo the text it was sent, which must not
// reach logs or compliance events.
type providerError struct {
	err error
}

func (e *providerError) Error() string {
	return "provider call failed: " + guardrails.ErrorClass(e.err)
}

func (e *providerError) Unwrap() error { return e.err }

// splitSegment cuts a segment's text into pieces of at most pieceMax runes
// (one piece when pieceMax is 0 or the text fits), with a seam across each
// cut when withSeams is set.
func splitSegment(parent int, text string, pieceMax int, withSeams bool) []piece {
	if pieceMax <= 0 || utf8.RuneCountInString(text) <= pieceMax {
		return []piece{{parent: parent, text: text}}
	}
	runes := []rune(text)
	k := seamRunes(pieceMax)
	var out []piece
	for start, n := 0, 0; start < len(runes); start, n = start+pieceMax, n+1 {
		end := min(start+pieceMax, len(runes))
		if withSeams && start > 0 {
			out = append(out, piece{parent: parent, text: string(runes[max(start-k, 0):min(start+k, len(runes))]), ordinal: -1})
		}
		out = append(out, piece{parent: parent, text: string(runes[start:end]), ordinal: n})
	}
	return out
}

// seamRunes is the overlap on each side of a cut.
func seamRunes(pieceMax int) int {
	return max(1, min(maxSeamRunes, pieceMax/4))
}

// packCalls groups pieces, in order, into calls of at most maxCallChars
// runes; with no limit, into one call.
func packCalls(pieces []piece, maxCallChars int) []call {
	if len(pieces) == 0 {
		return nil
	}
	if maxCallChars <= 0 {
		return []call{pieces}
	}
	var calls []call
	var cur call
	size := 0
	for _, p := range pieces {
		n := utf8.RuneCountInString(p.text)
		if len(cur) > 0 && size+n > maxCallChars {
			calls = append(calls, cur)
			cur, size = nil, 0
		}
		cur = append(cur, p)
		size += n
	}
	return append(calls, cur)
}

// packByTrust packs the pieces of tool results and those of the other text
// into separate calls. A call's failure costs every segment in it, and a
// tool result can be built to make its call fail (slow, throttled,
// refused); kept apart, it can only cost other tool results, never the
// user's prompt or the system prompt.
//
// Tool calls, when the filter scans them, are a class of their own: the
// model wrote them, possibly from what a tool result told it, so they are
// not the client's text, and keeping them out of the tool results' calls
// leaves those calls (and the quarantine of their tool results when a call
// fails under fail closed) exactly as they are without tool-call scanning.
func (j *scanJob) packByTrust(pieces []piece, maxCallChars int) []call {
	var classes [trustClasses][]piece
	for _, p := range pieces {
		c := trustClass(j.segments[p.parent])
		classes[c] = append(classes[c], p)
	}
	var calls []call
	for _, class := range classes {
		if j.caps.SegmentPerCall {
			calls = append(calls, j.packPerSegment(class, maxCallChars)...)
		} else {
			calls = append(calls, packCalls(class, maxCallChars)...)
		}
	}
	return calls
}

// Trust classes of segments, which never share a provider call.
const (
	trustOwn = iota
	trustToolResult
	trustToolCall
	trustClasses
)

func trustClass(s guardrails.Segment) int {
	switch {
	case s.IsToolResult():
		return trustToolResult
	case s.IsToolCall():
		return trustToolCall
	}
	return trustOwn
}

// packPerSegment packs each segment's pieces into calls of their own, for a
// provider that makes one request per segment (Capabilities.SegmentPerCall):
// its calls then run in parallel, each under its own timeout, and a failure
// costs only its segment.
func (j *scanJob) packPerSegment(pieces []piece, maxCallChars int) []call {
	var calls []call
	for start := 0; start < len(pieces); {
		end := start
		for end < len(pieces) && pieces[end].parent == pieces[start].parent {
			end++
		}
		calls = append(calls, packCalls(pieces[start:end], maxCallChars)...)
		start = end
	}
	return calls
}

// callsWithinSmallRun reports whether the calls are few enough for a small
// run: one per trust class, or for a provider that makes a request per
// segment (SegmentPerCall) one parallel wave of smallRunWave per class, so
// that an ordinary request (a system prompt and a user message) on such a
// provider still has its outages decided by the fail mode.
func (j *scanJob) callsWithinSmallRun(calls []call) bool {
	limit := 1
	if j.caps.SegmentPerCall {
		limit = smallRunWave
	}
	var perClass [trustClasses]int
	for _, c := range calls {
		class := trustClass(j.segments[c[0].parent])
		if perClass[class]++; perClass[class] > limit {
			return false
		}
	}
	return true
}

// toolsOnly reports a call that holds tool results only.
func (j *scanJob) toolsOnly(c call) bool {
	for _, p := range c {
		if !j.segments[p.parent].IsToolResult() {
			return false
		}
	}
	return len(c) > 0
}

// failedToolFindings put the failed tool results of a fail-closed run on
// record, so the quarantine placeholder and the outcome name them.
func (r scanResult) failedToolFindings() []guardrails.Finding {
	out := make([]guardrails.Finding, 0, len(r.failedTools))
	for _, i := range slices.Sorted(maps.Keys(r.failedTools)) {
		out = append(out, unscannedFinding(i, "Not scanned (provider failure)"))
	}
	return out
}

// failedToolsEvent records tool results quarantined because their call
// failed under fail closed: the error's class only, never its text.
func failedToolsEvent(filterName string, cfg guardrails.Config, segs []guardrails.Segment, failed map[int]bool, err error) ComplianceEventOutput {
	tools := map[string]bool{}
	for si := range failed {
		if name := safeLabel(segs[si].ToolName); name != "" {
			tools[name] = true
		}
	}
	class := guardrails.ErrorClass(err)
	return ComplianceEventOutput{
		EventType:   "guardrail.error",
		Severity:    "warning",
		Description: fmt.Sprintf("guardrail %q (%s) could not evaluate %d tool result(s): %s; fail mode %s; the tool results were quarantined", filterName, cfg.Provider, len(failed), class, cfg.FailMode),
		Metadata: map[string]interface{}{
			"provider":    cfg.Provider,
			"fail_mode":   cfg.FailMode,
			"blocked":     false,
			"action":      guardrails.ActionQuarantine,
			"error_class": class,
			"count":       len(failed),
			"tool_names":  sortedKeys(tools),
		},
	}
}

// runeTotal is the characters of text in pieces.
func runeTotal(pieces []piece) int {
	n := 0
	for _, p := range pieces {
		n += utf8.RuneCountInString(p.text)
	}
	return n
}

// parents lists the segments a call has pieces of, in order.
func parents(c call) []int {
	var out []int
	seen := map[int]bool{}
	for _, p := range c {
		if !seen[p.parent] {
			seen[p.parent] = true
			out = append(out, p.parent)
		}
	}
	return out
}

func headRunes(s string, n int) string {
	r := []rune(s)
	return string(r[:min(n, len(r))])
}

func tailRunes(s string, n int) string {
	r := []rune(s)
	return string(r[max(len(r)-n, 0):])
}

// segmentKind is a segment's kind for metrics.
func segmentKind(s guardrails.Segment) string {
	if s.Kind == "" {
		return guardrails.KindText
	}
	return s.Kind
}

// callKind is the kind of text in a call for metrics: one kind, or mixed.
func (j *scanJob) callKind(c call) string {
	kind := ""
	for _, p := range c {
		k := segmentKind(j.segments[p.parent])
		if kind != "" && k != kind {
			return "mixed"
		}
		kind = k
	}
	return kind
}

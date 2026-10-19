package scripting

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
)

// Shadow guardrails (guardrails.Config.Shadow) never hold up traffic: the
// runner returns pass-through at once and hands the classification to this
// pool, which runs it with a context detached from the request and records
// its compliance events through the input's RecordEvents when it finishes.
//
// The pool is bounded by the bytes of text waiting, with a per-tenant share
// so one app's flood cannot crowd out the others. A job that does not fit is
// dropped, never waited for: aistudio_guardrail_shadow_dropped_total counts
// it and one guardrail.unscanned event (reason shadow_dropped) records it.

// Environment variables (or AppConf.FilterLimits) that size the pool.
const (
	// EnvShadowWorkers is how many shadow evaluations run at once per
	// process (default 4 per CPU Go uses, at least 8).
	EnvShadowWorkers = "GUARDRAIL_SHADOW_WORKERS"
	// EnvShadowQueueBytes bounds the text waiting for a worker (default
	// 64 MiB); one tenant may hold at most half of it.
	EnvShadowQueueBytes = "GUARDRAIL_SHADOW_QUEUE_BYTES"

	defaultShadowQueueBytes = 64 << 20
	// shadowJobsMax bounds the queued jobs whatever their size.
	shadowJobsMax = 10000
)

// Reasons a shadow evaluation is dropped.
const (
	shadowDropQueueFull   = "queue_full"
	shadowDropTenantShare = "tenant_share"
	shadowDropShutdown    = "shutdown"
)

// unscannedShadowDropped is the unscanned reason of a dropped evaluation.
const unscannedShadowDropped = "shadow_dropped"

type shadowJob struct {
	run    func()
	bytes  int64
	tenant string
}

type shadowPool struct {
	jobs     chan *shadowJob
	maxBytes int64

	mu        sync.Mutex
	queued    int64
	perTenant map[string]int64
	closed    bool
	// abandon makes the workers drop what is still queued (shutdown past
	// its deadline).
	abandon bool
	wg      sync.WaitGroup
}

func newShadowPool(workers int, maxBytes int64) *shadowPool {
	p := &shadowPool{jobs: make(chan *shadowJob, shadowJobsMax), maxBytes: maxBytes, perTenant: map[string]int64{}}
	for range workers {
		p.wg.Add(1)
		go p.work()
	}
	return p
}

var (
	shadowPoolMu     sync.Mutex
	sharedShadowPool *shadowPool
)

// currentShadowPool returns the process's pool, sized from the environment
// on first use.
func currentShadowPool() *shadowPool {
	shadowPoolMu.Lock()
	defer shadowPoolMu.Unlock()
	if sharedShadowPool == nil {
		sharedShadowPool = newShadowPool(
			tunableInt(EnvShadowWorkers, max(8, 4*runtime.GOMAXPROCS(0))),
			int64(tunableInt(EnvShadowQueueBytes, defaultShadowQueueBytes)),
		)
	}
	return sharedShadowPool
}

// enqueue queues job, or returns why it was dropped.
func (p *shadowPool) enqueue(job *shadowJob) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch {
	case p.closed:
		return shadowDropShutdown
	case p.queued+job.bytes > p.maxBytes:
		return shadowDropQueueFull
	case p.perTenant[job.tenant]+job.bytes > p.maxBytes/2:
		return shadowDropTenantShare
	}
	select {
	case p.jobs <- job:
	default:
		return shadowDropQueueFull
	}
	p.queued += job.bytes
	p.perTenant[job.tenant] += job.bytes
	return ""
}

func (p *shadowPool) work() {
	defer p.wg.Done()
	for job := range p.jobs {
		p.mu.Lock()
		p.queued -= job.bytes
		if p.perTenant[job.tenant] -= job.bytes; p.perTenant[job.tenant] <= 0 {
			delete(p.perTenant, job.tenant)
		}
		abandon := p.abandon
		p.mu.Unlock()
		if abandon {
			continue
		}
		safe.CallRedacted("guardrail_shadow", job.run)
	}
}

// drain stops the pool taking jobs and waits for the queued ones until ctx
// ends; what is still queued then is abandoned. It reports whether every
// job finished.
func (p *shadowPool) drain(ctx context.Context) bool {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.jobs)
	}
	p.mu.Unlock()
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		p.mu.Lock()
		p.abandon = true
		p.mu.Unlock()
		return false
	}
}

// DrainShadowGuardrails stops the pool taking new evaluations and waits for
// the queued ones until ctx ends (graceful shutdown); what is still queued
// then is abandoned. An evaluation queued after it starts a new pool, so a
// process that embeds and restarts AI Studio keeps shadow filters working.
func DrainShadowGuardrails(ctx context.Context) {
	shadowPoolMu.Lock()
	p := sharedShadowPool
	sharedShadowPool = nil
	shadowPoolMu.Unlock()
	if p == nil {
		return
	}
	if !p.drain(ctx) {
		slog.Warn("shadow guardrail evaluations abandoned at shutdown")
	}
}

// shadowInputBytes approximates the memory a queued evaluation holds.
func shadowInputBytes(in *ScriptInput) int64 {
	n := len(in.RawInput) + len(in.CurrentBuffer) + len(in.ResponseBody) + len(in.RequestBody) + 512
	for _, m := range in.Messages {
		n += len(messageText(m))
	}
	return int64(n)
}

// shadow queues the guardrail's evaluation of input and returns pass-through
// at once. The evaluation runs on a copy of the input with a context
// detached from the request, bounded by the filter's own time budget, and
// records its compliance events through input.RecordEvents. Its outcome
// never changes the traffic, whatever it finds or however it fails.
func (g *guardrailRunner) shadow(input *ScriptInput, p *preparedGuardrail) *ScriptOutput {
	cfg := p.cfg
	record := input.RecordEvents
	parent := input.Ctx
	if parent == nil {
		parent = context.Background()
	}
	detached := context.WithoutCancel(parent)
	snapshot := *input
	snapshot.RecordEvents = nil
	limit := time.Duration(max(cfg.Budget.TimeMs, cfg.TimeoutMs))*time.Millisecond + 2*callGrace

	job := &shadowJob{bytes: shadowInputBytes(input), tenant: tenantScope(input.Context)}
	job.run = func() {
		ctx, cancel := context.WithTimeout(detached, limit)
		defer cancel()
		in := snapshot
		in.Ctx = ctx
		out, err := g.evaluateRecovered(&in, p)
		if err != nil {
			slog.Warn("shadow guardrail evaluation failed", "filter_name", g.filter.Name, "error_class", guardrails.ErrorClass(err))
			return
		}
		if out != nil && len(out.ComplianceEvents) > 0 {
			record(ctx, g.filter.Name, out)
		}
	}
	if reason := currentShadowPool().enqueue(job); reason != "" {
		metrics.RecordGuardrailShadowDropped(parent, cfg.Provider, reason)
		record(detached, g.filter.Name, &ScriptOutput{ComplianceEvents: []ComplianceEventOutput{shadowDroppedEvent(g.filter.Name, cfg, input, reason)}})
	}
	return passThrough(input)
}

// evaluateRecovered is evaluate with a panic turned into its recorded
// outcome, as RunScript does.
func (g *guardrailRunner) evaluateRecovered(input *ScriptInput, p *preparedGuardrail) (out *ScriptOutput, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = g.panicked(input, r), nil
		}
	}()
	return g.evaluate(input, p)
}

// shadowDroppedEvent records a shadow evaluation that was dropped.
func shadowDroppedEvent(filterName string, cfg guardrails.Config, input *ScriptInput, reason string) ComplianceEventOutput {
	return ComplianceEventOutput{
		EventType:   "guardrail." + unscannedDetector,
		Severity:    "warning",
		Description: fmt.Sprintf("guardrail %q (%s): shadow evaluation dropped (%s); the traffic went on unscreened, as shadow traffic always does", filterName, cfg.Provider, reason),
		Metadata: map[string]interface{}{
			"provider":  cfg.Provider,
			"detector":  unscannedDetector,
			"reason":    unscannedShadowDropped,
			"drop":      reason,
			"action":    guardrails.ActionLog,
			"shadow":    true,
			"response":  input.IsResponse,
			"max_chars": len(input.RawInput),
		},
	}
}

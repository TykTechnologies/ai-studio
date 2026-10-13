package scripting

import (
	"context"
	"errors"
	"runtime"
	"sync"
)

// In-process providers (the built-in library) scan on the gateway's own CPU,
// and a client decides how much text there is to scan. The local scan
// limiter bounds that work per process, in two pools: calls of at most
// budget.single_call_chars (an ordinary prompt, which scans in milliseconds)
// and larger ones, so an ordinary request never queues behind near-cap
// scans, however many tenants a flood comes from. The large pool has half
// the CPUs Go uses by default and the small one a quarter (at least one
// each), so a flood of either cannot keep every core busy. In each pool a
// tenant (tenantScope: the app, else the user, else the chat session) holds
// at most half the slots, freed slots go round-robin to the tenants
// waiting, and a tenant may have at most localScanQueueBytes of text
// waiting. A call that cannot get a slot within the filter's time budget, or
// whose tenant's queue is full, is not made: on the request side its text is
// unscanned, never decided by the fail mode, since the client can cause the
// contention.

// Environment variables (or AppConf.FilterLimits) that size the limiter.
const (
	// EnvLocalScanSlots is how many large in-process provider calls run at
	// once; the default is half of GOMAXPROCS.
	EnvLocalScanSlots = "GUARDRAIL_LOCAL_SCAN_SLOTS"
	// EnvLocalScanSmallSlots is how many small ones (at most
	// budget.single_call_chars of text) run at once, besides those; the
	// default is a quarter of GOMAXPROCS.
	EnvLocalScanSmallSlots = "GUARDRAIL_LOCAL_SCAN_SMALL_SLOTS"
	// EnvLocalScanQueueBytes is how much text one tenant may have waiting
	// for a slot.
	EnvLocalScanQueueBytes = "GUARDRAIL_LOCAL_SCAN_QUEUE_BYTES"

	defaultLocalScanQueueBytes = 64 << 20
)

// errLocalBusy is a local call that was not made: no slot came free within
// the time budget, or its tenant had too much text waiting.
var errLocalBusy = errors.New("guardrail local scan capacity exhausted")

// maxLocalScanSlots bounds either pool's configured size.
const maxLocalScanSlots = 1024

// localPools are the process's two limiters.
type localPools struct {
	large, small *localLimiter
}

var (
	localScanMu      sync.Mutex
	sharedLocalScans *localPools
)

// newLocalPools sizes the pools for procs CPUs and the configured limits.
func newLocalPools(procs int) *localPools {
	queued := int64(tunableInt(EnvLocalScanQueueBytes, defaultLocalScanQueueBytes))
	return &localPools{
		large: newLocalLimiter(boundedTunable(EnvLocalScanSlots, max(1, procs/2), 1, maxLocalScanSlots), queued),
		small: newLocalLimiter(boundedTunable(EnvLocalScanSmallSlots, max(1, procs/4), 1, maxLocalScanSlots), queued),
	}
}

// currentLocalLimiter returns the process's limiter for a call that is
// small (at most budget.single_call_chars of text) or not, sizing the pools
// on first use.
func currentLocalLimiter(small bool) *localLimiter {
	localScanMu.Lock()
	defer localScanMu.Unlock()
	if sharedLocalScans == nil {
		sharedLocalScans = newLocalPools(runtime.GOMAXPROCS(0))
	}
	if small {
		return sharedLocalScans.small
	}
	return sharedLocalScans.large
}

type localLimiter struct {
	perTenant int
	maxQueued int64

	mu   sync.Mutex
	free int
	held map[string]int
	// queued is each tenant's text waiting, waiting its calls in arrival
	// order; turns lists the tenants with calls waiting, served round-robin
	// from next.
	queued  map[string]int64
	waiting map[string][]*localWaiter
	turns   []string
	next    int
}

type localWaiter struct {
	ready   chan struct{}
	bytes   int64
	granted bool
}

func newLocalLimiter(slots int, maxQueued int64) *localLimiter {
	slots = max(slots, 1)
	return &localLimiter{
		perTenant: max(1, slots/2),
		maxQueued: maxQueued,
		free:      slots,
		held:      map[string]int{},
		queued:    map[string]int64{},
		waiting:   map[string][]*localWaiter{},
	}
}

// acquire waits for a slot for tenant's call of size bytes until ctx ends.
// The caller must call release once the call has returned (not when it
// stopped waiting for it), so the slots bound the work actually running.
func (l *localLimiter) acquire(ctx context.Context, tenant string, bytes int64) (release func(), err error) {
	release = func() { l.release(tenant) }
	l.mu.Lock()
	if l.free > 0 && l.held[tenant] < l.perTenant {
		l.free--
		l.held[tenant]++
		l.mu.Unlock()
		return release, nil
	}
	if l.queued[tenant]+bytes > l.maxQueued {
		l.mu.Unlock()
		return nil, errLocalBusy
	}
	w := &localWaiter{ready: make(chan struct{}), bytes: bytes}
	if len(l.waiting[tenant]) == 0 {
		l.turns = append(l.turns, tenant)
	}
	l.waiting[tenant] = append(l.waiting[tenant], w)
	l.queued[tenant] += bytes
	l.mu.Unlock()

	select {
	case <-w.ready:
		return release, nil
	case <-ctx.Done():
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if w.granted {
		// Granted as the wait ended: hand the slot on.
		l.releaseLocked(tenant)
	} else {
		l.dropWaiter(tenant, w)
	}
	return nil, errLocalBusy
}

func (l *localLimiter) release(tenant string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.releaseLocked(tenant)
}

func (l *localLimiter) releaseLocked(tenant string) {
	l.free++
	if l.held[tenant]--; l.held[tenant] <= 0 {
		delete(l.held, tenant)
	}
	l.dispatch()
}

// dispatch hands free slots to waiting calls, taking the tenants in turn and
// skipping those that already hold their share.
func (l *localLimiter) dispatch() {
	for l.free > 0 && len(l.turns) > 0 {
		granted := false
		for range l.turns {
			if l.next >= len(l.turns) {
				l.next = 0
			}
			tenant := l.turns[l.next]
			if l.held[tenant] >= l.perTenant {
				l.next++
				continue
			}
			w := l.waiting[tenant][0]
			l.dropWaiter(tenant, w)
			// dropWaiter removed tenant from turns if it has nothing left
			// waiting; otherwise move past it.
			if len(l.waiting[tenant]) > 0 {
				l.next++
			}
			l.free--
			l.held[tenant]++
			w.granted = true
			close(w.ready)
			granted = true
			break
		}
		if !granted {
			return
		}
	}
}

// dropWaiter takes w off tenant's queue.
func (l *localLimiter) dropWaiter(tenant string, w *localWaiter) {
	q := l.waiting[tenant]
	for i, x := range q {
		if x == w {
			q = append(q[:i], q[i+1:]...)
			break
		}
	}
	if l.queued[tenant] -= w.bytes; l.queued[tenant] <= 0 {
		delete(l.queued, tenant)
	}
	if len(q) > 0 {
		l.waiting[tenant] = q
		return
	}
	delete(l.waiting, tenant)
	for i, t := range l.turns {
		if t == tenant {
			l.turns = append(l.turns[:i], l.turns[i+1:]...)
			if i < l.next {
				l.next--
			}
			break
		}
	}
}

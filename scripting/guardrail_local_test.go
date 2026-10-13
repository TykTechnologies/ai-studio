package scripting

import (
	"context"
	"errors"
	"testing"
	"time"
)

// useLocalLimiter gives the test its own local scan limiter, one pool for
// small and large calls alike.
func useLocalLimiter(t *testing.T, l *localLimiter) {
	t.Helper()
	useLocalPools(t, &localPools{large: l, small: l})
}

// useLocalPools gives the test its own pools.
func useLocalPools(t *testing.T, p *localPools) {
	t.Helper()
	localScanMu.Lock()
	prev := sharedLocalScans
	sharedLocalScans = p
	localScanMu.Unlock()
	t.Cleanup(func() {
		localScanMu.Lock()
		sharedLocalScans = prev
		localScanMu.Unlock()
	})
}

func mustAcquire(t *testing.T, l *localLimiter, tenant string) func() {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	release, err := l.acquire(ctx, tenant, 1)
	if err != nil {
		t.Fatalf("acquire(%s): %v", tenant, err)
	}
	return release
}

// waitFor starts an acquire for tenant and returns a channel that receives
// its release func once granted (nil if it failed).
func waitFor(l *localLimiter, ctx context.Context, tenant string, bytes int64) <-chan func() {
	ch := make(chan func(), 1)
	go func() {
		release, err := l.acquire(ctx, tenant, bytes)
		if err != nil {
			release = nil
		}
		ch <- release
	}()
	return ch
}

func granted(ch <-chan func()) (func(), bool) {
	select {
	case r := <-ch:
		return r, r != nil
	case <-time.After(50 * time.Millisecond):
		return nil, false
	}
}

// A tenant holds at most half the slots, so another tenant always finds one.
func TestLocalLimiter_TenantShare(t *testing.T) {
	l := newLocalLimiter(4, 1<<20)
	a1, a2 := mustAcquire(t, l, "a"), mustAcquire(t, l, "a")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a3 := waitFor(l, ctx, "a", 1)
	if _, ok := granted(a3); ok {
		t.Fatal("tenant a got more than half the slots")
	}
	b1 := mustAcquire(t, l, "b")
	a1()
	release, ok := granted(a3)
	if !ok {
		t.Fatal("tenant a's waiting call did not get the slot a freed")
	}
	release()
	a2()
	b1()
	if l.free != 4 || len(l.held) != 0 || len(l.waiting) != 0 || len(l.queued) != 0 {
		t.Errorf("limiter not back to idle: free=%d held=%v waiting=%d queued=%v", l.free, l.held, len(l.waiting), l.queued)
	}
}

// Freed slots go to the waiting tenants in turn, not in arrival order: a
// tenant that queued many calls first does not take every slot that frees.
func TestLocalLimiter_RoundRobin(t *testing.T) {
	l := newLocalLimiter(4, 1<<20) // two slots per tenant
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	x := mustAcquire(t, l, "x")
	y := mustAcquire(t, l, "y")
	defer mustAcquire(t, l, "w")()
	defer mustAcquire(t, l, "v")()
	var floods []<-chan func()
	for range 3 {
		floods = append(floods, waitFor(l, ctx, "a", 1))
	}
	time.Sleep(20 * time.Millisecond)
	victim := waitFor(l, ctx, "b", 1)
	time.Sleep(20 * time.Millisecond)

	x()
	var first func()
	for _, f := range floods {
		if r, ok := granted(f); ok {
			first = r
			break
		}
	}
	if first == nil {
		t.Fatal("no waiting call of tenant a was served")
	}
	y()
	release, ok := granted(victim)
	if !ok {
		t.Fatal("tenant b waited behind tenant a's whole queue")
	}
	release()
	first()
}

// A tenant with too much text waiting is refused at once; other tenants are
// not.
func TestLocalLimiter_QueuedBytesCap(t *testing.T) {
	l := newLocalLimiter(1, 100)
	held := mustAcquire(t, l, "z")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = waitFor(l, ctx, "a", 80)
	time.Sleep(20 * time.Millisecond)
	if _, err := l.acquire(ctx, "a", 30); !errors.Is(err, errLocalBusy) {
		t.Errorf("acquire over tenant a's queue cap = %v, want errLocalBusy", err)
	}
	b := waitFor(l, ctx, "b", 80)
	if _, ok := granted(b); ok {
		t.Fatal("b was granted while the only slot was held")
	}
	held()
}

// A call that stops waiting leaves the queue, and a slot granted just as it
// stopped is handed on.
func TestLocalLimiter_WaitExpires(t *testing.T) {
	l := newLocalLimiter(1, 1<<20)
	held := mustAcquire(t, l, "z")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := l.acquire(ctx, "a", 10); !errors.Is(err, errLocalBusy) {
		t.Fatalf("acquire past its wait = %v, want errLocalBusy", err)
	}
	if len(l.waiting) != 0 || len(l.queued) != 0 || len(l.turns) != 0 {
		t.Errorf("an expired waiter stayed queued: waiting=%v queued=%v turns=%v", l.waiting, l.queued, l.turns)
	}
	held()
	if l.free != 1 {
		t.Errorf("free = %d after release, want 1", l.free)
	}
}

// The pools default to half and a quarter of the CPUs, at least one slot
// each, and their configured sizes are bounded.
func TestLocalPools_Sizes(t *testing.T) {
	for procs, want := range map[int][2]int{1: {1, 1}, 2: {1, 1}, 4: {2, 1}, 16: {8, 4}} {
		p := newLocalPools(procs)
		if got := [2]int{p.large.free, p.small.free}; got != want {
			t.Errorf("%d CPUs: slots %v, want %v", procs, got, want)
		}
	}
	t.Setenv(EnvLocalScanSlots, "3")
	t.Setenv(EnvLocalScanSmallSlots, "99999")
	p := newLocalPools(16)
	if p.large.free != 3 || p.small.free != maxLocalScanSlots {
		t.Errorf("configured slots: large %d, small %d; want 3 and %d", p.large.free, p.small.free, maxLocalScanSlots)
	}
}

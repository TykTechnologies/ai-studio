// Package replicas is how code anywhere in Studio (core or Enterprise)
// cooperates with the other control-plane replicas sharing its database,
// without being handed the cluster machinery:
//
//   - IsLeader: whether this replica should run singleton background work
//     (aggregations, alerts, syncs, reports, cleanups). One replica holds the
//     leader lease at a time (pkg/cluster.Leadership).
//   - OnLeading: run something when this replica becomes the leader, such as
//     start-up work that was skipped because another replica (or this one's
//     dead predecessor) still held the lease.
//   - Signal / OnSignal: tell every other replica that some in-memory state
//     derived from the database is stale ("budgets", "governed_metadata",
//     ...). The replica that changed the data refreshes its own state
//     itself; the signal reaches the others through the cluster event log,
//     at least once, within about a second.
//
// Studio runs one instance per process, so this is process-wide. Until
// pkg/studio connects a backend (and in tests), this process is the only
// replica: IsLeader is true and signals go nowhere.
package replicas

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/TykTechnologies/midsommar/v2/logger"
)

// Signal names shared across packages.
const (
	// SignalBudgets: budget limits, periods or the team-budget switch
	// changed; every replica clears its budget caches.
	SignalBudgets = "budgets"
	// SignalGovernedMetadata: metadata schemas or vocabularies changed;
	// every replica drops its resolved-schema cache (Enterprise).
	SignalGovernedMetadata = "governed_metadata"
)

// Backend is the cluster behind the package (pkg/studio provides it).
type Backend interface {
	IsLeader() bool
	// Signal publishes name to the other replicas.
	Signal(ctx context.Context, name string) error
}

var (
	backend atomic.Pointer[Backend]

	mu       sync.Mutex
	handlers = map[string]map[int]func(){}
	leading  = map[int]func(){}
	nextID   int
)

// SetBackend connects the cluster; nil disconnects it.
func SetBackend(b Backend) {
	if b == nil {
		backend.Store(nil)
		return
	}
	backend.Store(&b)
}

// IsLeader reports whether this replica runs singleton background work.
func IsLeader() bool {
	if b := backend.Load(); b != nil {
		return (*b).IsLeader()
	}
	return true
}

// Signal tells the other replicas that the state called name is stale. It
// does not call this replica's own handlers: the caller has refreshed its
// state already. Failures are logged; the other replicas then refresh on
// their own schedule (TTL) if they have one, or serve stale state.
func Signal(ctx context.Context, name string) {
	b := backend.Load()
	if b == nil {
		return
	}
	if err := (*b).Signal(ctx, name); err != nil {
		logger.Errorf("Could not tell the other replicas that %q changed; they may serve stale data: %v", name, err)
	}
}

// OnSignal registers fn to run when another replica signals name. fn runs
// on the cluster log's reader goroutine: it must be quick (clear a cache,
// trigger a background reload). The returned function removes it.
func OnSignal(name string, fn func()) (remove func()) {
	mu.Lock()
	defer mu.Unlock()
	id := nextID
	nextID++
	if handlers[name] == nil {
		handlers[name] = map[int]func(){}
	}
	handlers[name][id] = fn
	return func() {
		mu.Lock()
		defer mu.Unlock()
		delete(handlers[name], id)
	}
}

// Deliver runs the handlers for a signal received from another replica
// (called by the backend).
func Deliver(name string) {
	mu.Lock()
	fns := make([]func(), 0, len(handlers[name]))
	for _, fn := range handlers[name] {
		fns = append(fns, fn)
	}
	mu.Unlock()
	run(fns, fmt.Sprintf("replica signal %q", name))
}

// OnLeading registers fn to run each time this replica becomes the leader.
// Leader-only work that checks IsLeader on a timer uses it to catch up at
// once instead of waiting a whole interval: a replica restarted after a
// crash may only get the lease a moment after its start-up run was skipped.
// Register before checking IsLeader, so no change falls between the two.
// fn runs on the lease's goroutine: it must be quick (wake a loop). The
// returned function removes it.
func OnLeading(fn func()) (remove func()) {
	mu.Lock()
	defer mu.Unlock()
	id := nextID
	nextID++
	leading[id] = fn
	return func() {
		mu.Lock()
		defer mu.Unlock()
		delete(leading, id)
	}
}

// BecameLeader runs the OnLeading handlers (called by the backend when this
// replica gains the leader lease).
func BecameLeader() {
	mu.Lock()
	fns := make([]func(), 0, len(leading))
	for _, fn := range leading {
		fns = append(fns, fn)
	}
	mu.Unlock()
	run(fns, "leadership")
}

func run(fns []func(), what string) {
	for _, fn := range fns {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Handler for %s panicked: %v", what, r)
				}
			}()
			fn()
		}()
	}
}

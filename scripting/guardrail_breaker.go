package scripting

import (
	"log/slog"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/lrucache"
)

// A provider that ignores its context is given up on (scanJob.run), but its
// call's goroutine runs on: the runner cannot stop it. A provider that hangs
// on every call would leave a few goroutines behind per request until the
// process runs out of memory. The call breaker bounds that per prepared
// filter: after breakerAbandons abandoned calls within breakerWindow, from
// at least breakerTenants tenants, the filter's calls are suspended, failing
// at once with guardrails.ErrProviderSuspended as an operational failure (on
// the request side a small run follows the fail mode, other text is
// unscanned; the response side follows the fail mode). After breakerCooldown
// one call is let through as a probe; if it answers in time the calls
// resume, if it is abandoned too they stay suspended for another cool-down.
//
// A breaker is shared by every tenant on the filter, and suspending it moves
// their small requests to the fail mode, so no one sender may trip it: only
// abandons in small runs count (a sender can make a large run slow), and
// they must come from several tenants (a real outage hits them all). Local
// providers have no breaker: their calls are abandoned only when the host is
// overloaded, which a sender can cause, and their failures are never left to
// the fail mode (failureReason).

const (
	breakerAbandons = 8
	breakerTenants  = 2
	breakerWindow   = time.Minute
	breakerCooldown = 10 * time.Second
)

// breakerNow is the breakers' clock; tests move it.
var breakerNow = time.Now

// breakers holds a breaker per prepared filter key, bounded like the
// prepared entries; a breaker outlives the entry's TTL so that re-preparing
// a filter does not reset it.
var breakers = lrucache.New[*callBreaker](preparedMax)

// breakerFor returns the breaker of a prepared filter, nil for a local
// provider and for a filter that is not a saved version (the test endpoint's
// temporary filter).
func breakerFor(p *preparedGuardrail) *callBreaker {
	if p.key == "" || p.caps.Local {
		return nil
	}
	b, _ := breakers.Do(p.key, func() (*callBreaker, error) { return &callBreaker{provider: p.cfg.Provider}, nil })
	return b
}

// callBreaker counts a filter's abandoned calls; nil never suspends.
type callBreaker struct {
	provider string

	mu sync.Mutex
	// abandons are the abandoned calls within the window, while closed.
	abandons []abandon
	// openedAt is when the calls were suspended, zero while they are not;
	// probing is set while a probe is out.
	openedAt time.Time
	probing  bool
}

// allow reports whether a call may be made. While suspended, one call at a
// time is let through as a probe once the cool-down has passed; probe is
// set for it, and the caller must end it with done or cancelProbe.
func (b *callBreaker) allow() (ok, probe bool) {
	if b == nil {
		return true, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.openedAt.IsZero() {
		return true, false
	}
	if b.probing || breakerNow().Sub(b.openedAt) < breakerCooldown {
		return false, false
	}
	b.probing = true
	return true, true
}

// cancelProbe ends a probe that was never sent.
func (b *callBreaker) cancelProbe() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.probing = false
	b.mu.Unlock()
}

type abandon struct {
	at     time.Time
	tenant string
}

// done records the end of a call that was made: abandoned when the run gave
// up waiting for it and the abandon counts. tenant is the request's
// tenantScope.
func (b *callBreaker) done(probe, abandoned bool, tenant string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := breakerNow()
	if probe {
		b.probing = false
		if abandoned {
			b.openedAt = now
		} else {
			b.openedAt, b.abandons = time.Time{}, nil
			slog.Info("guardrail provider answered a probe; calls resumed", "provider", b.provider)
		}
		return
	}
	if !abandoned || !b.openedAt.IsZero() {
		return
	}
	keep := b.abandons[:0]
	tenants := map[string]bool{tenant: true}
	for _, a := range b.abandons {
		if now.Sub(a.at) < breakerWindow {
			keep = append(keep, a)
			tenants[a.tenant] = true
		}
	}
	// A tenant counts for at most its share of breakerAbandons (its most
	// recent abandons), so the breaker needs breakerTenants tenants' worth.
	b.abandons = append(keep, abandon{at: now, tenant: tenant})
	if countTenant(b.abandons, tenant) > breakerAbandons/breakerTenants {
		b.abandons = dropOldest(b.abandons, tenant)
	}
	if len(b.abandons) >= breakerAbandons && len(tenants) >= breakerTenants {
		b.openedAt, b.abandons = now, nil
		slog.Warn("guardrail provider repeatedly did not return after its timeout; calls suspended",
			"provider", b.provider, "abandoned", breakerAbandons, "window", breakerWindow, "cooldown", breakerCooldown)
	}
}

func countTenant(as []abandon, tenant string) int {
	n := 0
	for _, a := range as {
		if a.tenant == tenant {
			n++
		}
	}
	return n
}

// dropOldest removes tenant's oldest abandon.
func dropOldest(as []abandon, tenant string) []abandon {
	for i, a := range as {
		if a.tenant == tenant {
			return append(as[:i], as[i+1:]...)
		}
	}
	return as
}

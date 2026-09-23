// Package sched drives load at a cell's arms and streams every Result to a
// sink. Three modes:
//
//   - open: requests are released on an arrival schedule (constant spacing or
//     Poisson) that does not wait for earlier responses. Latency is measured
//     from each request's intended release time, so a slow system is charged
//     for the queueing it causes (no coordinated omission).
//   - closed: N workers each send back to back. Only suitable for the
//     unloaded overhead floor, where there is no queueing to hide.
//   - paired: ABBA blocks, one request at a time per worker, for real
//     upstreams. Both arms see the same vendor conditions within a block.
package sched

import (
	"context"
	"math/rand/v2"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/probe"
)

// Arm is one side of a comparison (e.g. "gateway" or "direct").
type Arm struct {
	Name   string
	Target probe.Target
	// Weight is this arm's share of an open-loop schedule's slots.
	Weight float64
	Client *http.Client
	// Request, when set, replaces the cell's request for this arm (e.g. a
	// different model string on the unified endpoint).
	Request *probe.Request
}

// Cell is one measured configuration.
type Cell struct {
	Name    string
	Arms    []Arm
	Request probe.Request
}

// Sink receives every result, from many goroutines.
type Sink func(probe.Result)

// NewClient returns a client with a connection pool big enough that the load
// generator itself never limits concurrency. coldConn disables keep-alive.
func NewClient(coldConn bool) *http.Client {
	tr := &http.Transport{
		Proxy:               nil,
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:        0,
		MaxIdleConnsPerHost: 100000,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		DisableKeepAlives:   coldConn,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	return &http.Client{Transport: tr}
}

// --- open loop ---------------------------------------------------------------

// RateFunc gives the target arrival rate (requests/s) at elapsed time t, and
// false once the run is over.
type RateFunc func(t time.Duration) (float64, bool)

// Constant runs at rps for d.
func Constant(rps float64, d time.Duration) RateFunc {
	return func(t time.Duration) (float64, bool) { return rps, t < d }
}

// Steps starts at start rps and adds step every hold, up to max (inclusive),
// holding each level for hold.
func Steps(start, step, max float64, hold time.Duration) RateFunc {
	return func(t time.Duration) (float64, bool) {
		level := start + step*float64(int(t/hold))
		return level, level <= max
	}
}

// Spike runs at base, jumps to peak during [at, at+width), then returns to
// base until total.
func Spike(base, peak float64, at, width, total time.Duration) RateFunc {
	return func(t time.Duration) (float64, bool) {
		if t >= at && t < at+width {
			return peak, true
		}
		return base, t < total
	}
}

// OpenOptions configures an open-loop run.
type OpenOptions struct {
	Rate    RateFunc
	Poisson bool
	Warmup  time.Duration
	Timeout time.Duration
	// MaxInFlight bounds outstanding requests so a collapsed target cannot
	// exhaust the load generator. Slots beyond it are recorded as
	// "loadgen_overflow" errors rather than silently skipped.
	MaxInFlight int
	// Stop, if set, is polled once a second; returning true ends the run
	// early (ramp stop conditions).
	Stop func() bool
}

// RunOpen releases requests on the schedule until the rate function ends.
func RunOpen(ctx context.Context, cell Cell, o OpenOptions, sink Sink) {
	if o.MaxInFlight <= 0 {
		o.MaxInFlight = 20000
	}
	picker := newArmPicker(cell.Arms)
	var inflight atomic.Int64
	var wg sync.WaitGroup
	var seq int64

	start := time.Now()
	next := start
	stopCheck := start.Add(time.Second)

	for {
		elapsed := next.Sub(start)
		rate, ok := o.Rate(elapsed)
		if !ok || ctx.Err() != nil {
			break
		}
		if o.Stop != nil && time.Now().After(stopCheck) {
			stopCheck = time.Now().Add(time.Second)
			if o.Stop() {
				break
			}
		}
		intended := next
		if d := time.Until(intended); d > 0 {
			sleepCtx(ctx, d)
		}

		arm := picker.pick()
		seq++
		warm := elapsed < o.Warmup
		if inflight.Load() >= int64(o.MaxInFlight) {
			sink(probe.Result{Cell: cell.Name, Arm: arm.Name, Seq: seq, Warmup: warm,
				Intended: intended.UnixNano(), Err: "loadgen_overflow"})
		} else {
			inflight.Add(1)
			wg.Add(1)
			go func(arm Arm, seq int64, intended time.Time, warm bool) {
				defer wg.Done()
				defer inflight.Add(-1)
				sink(send(ctx, cell, arm, seq, intended, warm, o.Timeout))
			}(arm, seq, intended, warm)
		}

		if rate <= 0 {
			rate = 1
		}
		gap := float64(time.Second) / rate
		if o.Poisson {
			gap *= rand.ExpFloat64()
		}
		next = next.Add(time.Duration(gap))
	}
	wg.Wait()
}

// --- closed loop -------------------------------------------------------------

// ClosedOptions configures a closed-loop run.
type ClosedOptions struct {
	Concurrency    int
	Requests       int // measured requests per arm
	WarmupRequests int // per arm, excluded from stats
	Timeout        time.Duration
}

// RunClosed has Concurrency workers per arm send back to back. Arms take
// turns request by request, so both see the same conditions over time.
func RunClosed(ctx context.Context, cell Cell, o ClosedOptions, sink Sink) {
	if o.Concurrency < 1 {
		o.Concurrency = 1
	}
	total := (o.Requests + o.WarmupRequests) * len(cell.Arms)
	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < o.Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				i := next.Add(1) - 1
				if i >= int64(total) {
					return
				}
				arm := cell.Arms[i%int64(len(cell.Arms))]
				round := i / int64(len(cell.Arms))
				sink(send(ctx, cell, arm, i, time.Now(), round < int64(o.WarmupRequests), o.Timeout))
			}
		}()
	}
	wg.Wait()
}

// --- paired (real upstreams) -------------------------------------------------

// PairedOptions configures an ABBA run.
type PairedOptions struct {
	Blocks       int // measured blocks; each block sends A,B,B,A (or B,A,A,B)
	WarmupBlocks int
	Workers      int
	Timeout      time.Duration
	// Gap is a pause between requests, to stay inside vendor rate limits.
	Gap time.Duration
	// Backoff after a 429, unless the response carries Retry-After.
	Backoff time.Duration
}

// RunPaired needs exactly two arms. Each block's four results share a Pair id
// (carried in Seq / 4) so the report can difference them within the block.
func RunPaired(ctx context.Context, cell Cell, o PairedOptions, sink Sink) {
	if len(cell.Arms) != 2 {
		panic("paired run needs exactly two arms")
	}
	if o.Workers < 1 {
		o.Workers = 1
	}
	if o.Backoff <= 0 {
		o.Backoff = 10 * time.Second
	}
	total := o.Blocks + o.WarmupBlocks
	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < o.Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				block := next.Add(1) - 1
				if block >= int64(total) {
					return
				}
				a, b := cell.Arms[0], cell.Arms[1]
				if rand.IntN(2) == 1 {
					a, b = b, a
				}
				warm := block < int64(o.WarmupBlocks)
				for i, arm := range []Arm{a, b, b, a} {
					for attempt := 0; attempt < 5 && ctx.Err() == nil; attempt++ {
						res := send(ctx, cell, arm, block*4+int64(i), time.Now(), warm, o.Timeout)
						sink(res)
						if res.Status != http.StatusTooManyRequests {
							break
						}
						sleepCtx(ctx, o.Backoff)
					}
					if o.Gap > 0 {
						sleepCtx(ctx, o.Gap)
					}
				}
			}
		}()
	}
	wg.Wait()
}

// --- shared ------------------------------------------------------------------

func send(ctx context.Context, cell Cell, arm Arm, seq int64, intended time.Time, warm bool, timeout time.Duration) probe.Result {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	// The deadline runs from the intended start: a request the load generator
	// released late has already used part of its budget.
	rctx, cancel := context.WithDeadline(ctx, intended.Add(timeout))
	defer cancel()
	req := cell.Request
	if arm.Request != nil {
		req = *arm.Request
	}
	res := probe.Do(rctx, arm.Client, arm.Target, req, intended)
	res.Cell, res.Arm, res.Seq, res.Warmup = cell.Name, arm.Name, seq, warm
	return res
}

type armPicker struct {
	arms []Arm
	cum  []float64
}

func newArmPicker(arms []Arm) *armPicker {
	p := &armPicker{arms: arms}
	var sum float64
	for _, a := range arms {
		w := a.Weight
		if w <= 0 {
			w = 1
		}
		sum += w
		p.cum = append(p.cum, sum)
	}
	for i := range p.cum {
		p.cum[i] /= sum
	}
	return p
}

func (p *armPicker) pick() Arm {
	if len(p.arms) == 1 {
		return p.arms[0]
	}
	r := rand.Float64()
	for i, c := range p.cum {
		if r < c {
			return p.arms[i]
		}
	}
	return p.arms[len(p.arms)-1]
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

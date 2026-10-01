// Package safe keeps a panic in Studio's background work from taking the
// whole process down. Go's runtime ends the process on any goroutine's
// unrecovered panic; embedded in a host (the Dashboard, MDCB), that would
// take the host down with it, and every edge that depends on it.
//
// A recovered panic is logged with its stack through the logger package
// (logger.Current, so it shows even where Studio's logger was never set up)
// and counted (Panics, and aistudio_goroutine_panics_total when metrics are
// on).
//
//   - Recover, deferred at the top of a goroutine, ends that goroutine
//     quietly instead of the process; RecoverWith also tells its owner.
//   - Go starts a one-shot goroutine that does so.
//   - Call runs a function in the current goroutine and reports whether it
//     panicked, for per-item work (a handler, one event) whose loop should
//     carry on with the next item.
//   - Loop supervises a long-lived loop: it runs the loop again, after a
//     backoff, when it panics.
package safe

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/metrics"
)

var panics atomic.Uint64

// Panics returns how many panics this process has recovered through the
// package.
func Panics() uint64 { return panics.Load() }

// Recover recovers a panic in the goroutine it is deferred in and reports
// it under name. It must be deferred directly: defer safe.Recover("relay").
func Recover(name string) {
	if r := recover(); r != nil {
		report(name, r)
	}
}

// RecoverWith is Recover that also calls then after a panic, for a
// goroutine whose owner must learn that it ended (to end a stream, say).
// It must be deferred directly: defer safe.RecoverWith("recv", cancel).
func RecoverWith(name string, then func()) {
	if r := recover(); r != nil {
		report(name, r)
		then()
	}
}

// Go runs fn in a new goroutine; a panic in fn is recovered and reported
// under name.
func Go(name string, fn func()) {
	go func() {
		defer Recover(name)
		fn()
	}()
}

// Call runs fn and reports whether it panicked; the panic is recovered and
// reported under name.
func Call(name string, fn func()) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			report(name, r)
			panicked = true
		}
	}()
	fn()
	return false
}

// LoopOption tunes Loop.
type LoopOption func(*loopConfig)

type loopConfig struct {
	afterPanic func()
}

// AfterPanic has Loop call fn after each recovered panic, before it waits
// to run the loop again: to drop state the panic may have left half done
// (a connection, a lease the replica can no longer vouch for). A panic in
// fn is recovered too.
func AfterPanic(fn func()) LoopOption {
	return func(c *loopConfig) { c.afterPanic = fn }
}

// Backoff bounds for Loop: the first restart waits loopBackoffMin, each
// further one twice as long up to loopBackoffMax. A run that lasted
// loopBackoffReset before panicking starts again from the minimum.
var (
	loopBackoffMin   = time.Second
	loopBackoffMax   = 30 * time.Second
	loopBackoffReset = time.Minute
)

// Loop runs fn, a long-lived loop, in the calling goroutine until it
// returns. When fn panics, Loop recovers, reports it under name, and runs
// fn again after a backoff (1 s, doubling to 30 s), unless stop is closed
// first. Because it runs in the caller's goroutine, the caller's own
// shutdown signalling (defer close(done), a WaitGroup) keeps its meaning:
// it happens once, when Loop returns.
//
// fn starts from the top after a panic, so whatever it sets up for itself
// (tickers, local state) is set up again; state it shares with the rest of
// its type must survive a panic midway, or be reset with AfterPanic.
func Loop(name string, stop <-chan struct{}, fn func(), opts ...LoopOption) {
	var cfg loopConfig
	for _, o := range opts {
		o(&cfg)
	}
	backoff := loopBackoffMin
	for {
		started := time.Now()
		if !Call(name, fn) {
			return
		}
		if cfg.afterPanic != nil {
			Call(name+" (after panic)", cfg.afterPanic)
		}
		if time.Since(started) >= loopBackoffReset {
			backoff = loopBackoffMin
		}
		logger.Current().Warn().Str("goroutine", name).Dur("backoff", backoff).Msg("Restarting background work after a panic")
		t := time.NewTimer(backoff)
		select {
		case <-stop:
			t.Stop()
			return
		case <-t.C:
		}
		if backoff *= 2; backoff > loopBackoffMax {
			backoff = loopBackoffMax
		}
	}
}

func report(name string, r interface{}) {
	panics.Add(1)
	logger.Current().Error().
		Str("goroutine", name).
		Str("panic", fmt.Sprint(r)).
		Str("stack", string(debug.Stack())).
		Msg("Recovered from a panic; the process keeps running")
	metrics.RecordGoroutinePanic(context.Background(), name)
}

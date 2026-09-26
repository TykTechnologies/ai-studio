package overload

import (
	"math"
	"os"
	"runtime/debug"
	"runtime/metrics"
	"sync/atomic"
	"time"
)

// DefaultGCPercent is the highest GOGC the gateway sets when GOGC is not set.
//
// With Go's default of 100 the gateway's small live heap (tens of MB) gives a
// heap goal of ~100–170 MB, so under load the collector ran ~20 times a
// second, marked about 30% of the time and at times drafted every allocating
// request into mark assist for ~100 ms. Measured on a 4-vCPU edge (AWS gwbench
// S5, full policy), GOGC=400 cut GC to ~4–6 cycles a second, raised the
// sustained rate from ~7,100 to ~8,100 req/s, cut p50 overhead at 7,500 req/s
// from +2.8 to +0.6 ms and removed the one-window p99 spikes, for +100–200 MB
// of memory.
//
// A fixed 400 was too much once the live heap is large: with thousands of
// streams in flight (live heap ~0.4 GB) it let the heap grow to ~5x and
// roughly doubled memory. So GOGC is adapted to the live heap instead; see
// adaptiveGCPercent.
const DefaultGCPercent = 400

const (
	// minGCPercent is the lowest GOGC the adaptation sets (Go's default).
	minGCPercent = 100
	// gcHeadroom is how much garbage the adaptation lets build up between
	// collections: GOGC is chosen so the heap goal is about the live heap
	// plus this, within [minGCPercent, DefaultGCPercent].
	gcHeadroom = 256 << 20
	// gcAdaptInterval is how often the live heap is read to adapt GOGC.
	gcAdaptInterval = time.Second
)

// softLimitFraction is the share of the memory limit given to GOMEMLIMIT when
// the gateway sets it: close enough to the limit that the larger heap goal is
// used, far enough that shedding (at Threshold, 85% by default) comes first.
const softLimitFraction = 0.9

// currentGCPercent is the GOGC in effect, for metrics.
var currentGCPercent atomic.Int64

// CurrentGCPercent returns the GOGC in effect.
func CurrentGCPercent() int { return int(currentGCPercent.Load()) }

// RuntimeTuning reports what TuneRuntime applied.
type RuntimeTuning struct {
	// GCPercent is the GOGC in effect at start-up; GCPercentSet says the
	// gateway set it, and GCPercentAdaptive that it keeps adapting it.
	GCPercent         int
	GCPercentSet      bool
	GCPercentAdaptive bool
	// MemoryLimit is the limit overload shedding should judge against (0:
	// none known); SoftLimit is the GOMEMLIMIT the gateway set (0: none).
	MemoryLimit uint64
	SoftLimit   uint64
}

// TuneRuntime applies the gateway's garbage-collector defaults and returns the
// memory limit for overload shedding.
//
//   - GOGC unset: adapted to the live heap every second (adaptiveGCPercent),
//     starting at DefaultGCPercent.
//   - GOMEMLIMIT unset and a memory limit known (explicitLimit, else the
//     container's cgroup limit): GOMEMLIMIT at 90% of it, so the heap never
//     grows past the container.
//
// Operators keep full control: a GOGC or GOMEMLIMIT they set is left alone.
// The returned limit is the explicit one, else a GOMEMLIMIT the operator set,
// else the cgroup limit, not the soft limit set here.
func TuneRuntime(explicitLimit uint64) RuntimeTuning {
	t := tuneRuntime(explicitLimit, os.Getenv, func() uint64 { return cgroupMemoryLimit("/sys/fs/cgroup") })
	if t.GCPercentAdaptive {
		go adaptGC(nil)
	}
	return t
}

func tuneRuntime(explicitLimit uint64, getenv func(string) string, cgroupLimit func() uint64) RuntimeTuning {
	var t RuntimeTuning

	if getenv("GOGC") == "" {
		debug.SetGCPercent(DefaultGCPercent)
		t.GCPercent, t.GCPercentSet, t.GCPercentAdaptive = DefaultGCPercent, true, true
	} else {
		t.GCPercent = debug.SetGCPercent(-1)
		debug.SetGCPercent(t.GCPercent)
	}
	currentGCPercent.Store(int64(t.GCPercent))

	operatorLimit := getenv("GOMEMLIMIT") != ""
	switch {
	case explicitLimit > 0:
		t.MemoryLimit = explicitLimit
	case operatorLimit:
		if l := debug.SetMemoryLimit(-1); l > 0 && l < math.MaxInt64 {
			t.MemoryLimit = uint64(l)
		}
	default:
		t.MemoryLimit = cgroupLimit()
	}

	if !operatorLimit && t.MemoryLimit > 0 {
		t.SoftLimit = uint64(float64(t.MemoryLimit) * softLimitFraction)
		debug.SetMemoryLimit(int64(t.SoftLimit))
	}
	return t
}

// adaptiveGCPercent returns the GOGC for a live heap: the heap goal is about
// the live heap plus gcHeadroom, within [minGCPercent, DefaultGCPercent], in
// steps of 25 so small changes in the live heap do not change it every second.
//
// A small live heap (REST traffic, tens of MB) gets 400: few collections,
// which is where the throughput gain was measured. A large one (thousands of
// streams in flight) tapers towards Go's default of 100, so the collector's
// extra memory stays near gcHeadroom instead of growing with the heap.
func adaptiveGCPercent(live uint64) int {
	if live == 0 {
		return DefaultGCPercent
	}
	p := int(uint64(gcHeadroom) * 100 / live)
	if p > DefaultGCPercent {
		p = DefaultGCPercent
	}
	if p < minGCPercent {
		p = minGCPercent
	}
	return p / 25 * 25
}

// adaptGC sets GOGC from the live heap every gcAdaptInterval until stop is
// closed (nil: for the life of the process).
func adaptGC(stop <-chan struct{}) {
	sample := []metrics.Sample{{Name: "/gc/heap/live:bytes"}}
	ticker := time.NewTicker(gcAdaptInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			metrics.Read(sample)
			if sample[0].Value.Kind() != metrics.KindUint64 {
				continue
			}
			p := adaptiveGCPercent(sample[0].Value.Uint64())
			if int64(p) != currentGCPercent.Load() {
				debug.SetGCPercent(p)
				currentGCPercent.Store(int64(p))
			}
		}
	}
}

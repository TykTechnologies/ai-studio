package overload

import (
	"math"
	"os"
	"runtime/debug"
)

// DefaultGCPercent is the GOGC the gateway runs with when GOGC is not set.
//
// With Go's default of 100 the gateway's small live heap (tens of MB) gives a
// heap goal of ~100–170 MB, so under load the collector ran ~20 times a
// second, marked about 30% of the time and at times drafted every allocating
// request into mark assist for ~100 ms. Measured on a 4-vCPU edge (AWS gwbench
// S5, full policy), GOGC=400 cut GC to ~4–6 cycles a second, raised the
// sustained rate from ~7,100 to ~8,100 req/s, cut p50 overhead at 7,500 req/s
// from +2.8 to +0.6 ms and removed the one-window p99 spikes, for +100–200 MB
// of memory.
const DefaultGCPercent = 400

// softLimitFraction is the share of the memory limit given to GOMEMLIMIT when
// the gateway sets it: close enough to the limit that the larger heap goal is
// used, far enough that shedding (at Threshold, 85% by default) comes first.
const softLimitFraction = 0.9

// RuntimeTuning reports what TuneRuntime applied.
type RuntimeTuning struct {
	// GCPercent is the GOGC in effect; GCPercentSet says the gateway set it.
	GCPercent    int
	GCPercentSet bool
	// MemoryLimit is the limit overload shedding should judge against (0:
	// none known); SoftLimit is the GOMEMLIMIT the gateway set (0: none).
	MemoryLimit uint64
	SoftLimit   uint64
}

// TuneRuntime applies the gateway's garbage-collector defaults and returns the
// memory limit for overload shedding.
//
//   - GOGC unset: DefaultGCPercent.
//   - GOMEMLIMIT unset and a memory limit known (explicitLimit, else the
//     container's cgroup limit): GOMEMLIMIT at 90% of it, so the larger heap
//     never grows past the container.
//
// Operators keep full control: a GOGC or GOMEMLIMIT they set is left alone.
// The returned limit is the explicit one, else a GOMEMLIMIT the operator set,
// else the cgroup limit, not the soft limit set here.
func TuneRuntime(explicitLimit uint64) RuntimeTuning {
	return tuneRuntime(explicitLimit, os.Getenv, func() uint64 { return cgroupMemoryLimit("/sys/fs/cgroup") })
}

func tuneRuntime(explicitLimit uint64, getenv func(string) string, cgroupLimit func() uint64) RuntimeTuning {
	var t RuntimeTuning

	if getenv("GOGC") == "" {
		debug.SetGCPercent(DefaultGCPercent)
		t.GCPercent, t.GCPercentSet = DefaultGCPercent, true
	} else {
		t.GCPercent = debug.SetGCPercent(-1)
		debug.SetGCPercent(t.GCPercent)
	}

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

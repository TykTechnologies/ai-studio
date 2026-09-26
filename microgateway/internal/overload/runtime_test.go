package overload

import (
	"math"
	"runtime/debug"
	"testing"
)

// preserveRuntime restores GOGC and GOMEMLIMIT, which are process-wide, after
// a test changes them.
func preserveRuntime(t *testing.T) {
	t.Helper()
	gc := debug.SetGCPercent(-1)
	debug.SetGCPercent(gc)
	limit := debug.SetMemoryLimit(-1)
	t.Cleanup(func() {
		debug.SetGCPercent(gc)
		debug.SetMemoryLimit(limit)
	})
}

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

// soft is the GOMEMLIMIT tuneRuntime sets for a limit.
func soft(limit uint64) uint64 { return uint64(float64(limit) * softLimitFraction) }

func currentGCPercent() int {
	gc := debug.SetGCPercent(-1)
	debug.SetGCPercent(gc)
	return gc
}

func TestTuneRuntime(t *testing.T) {
	const gib = 1 << 30

	t.Run("defaults with a container limit", func(t *testing.T) {
		preserveRuntime(t)
		debug.SetMemoryLimit(math.MaxInt64)
		got := tuneRuntime(0, env(nil), func() uint64 { return 2 * gib })
		if !got.GCPercentSet || got.GCPercent != DefaultGCPercent || currentGCPercent() != DefaultGCPercent {
			t.Fatalf("GOGC not defaulted: %+v, runtime %d", got, currentGCPercent())
		}
		if got.MemoryLimit != 2*gib {
			t.Fatalf("memory limit = %d, want the container's", got.MemoryLimit)
		}
		want := soft(2 * gib)
		if got.SoftLimit != want || debug.SetMemoryLimit(-1) != int64(want) {
			t.Fatalf("soft limit = %d (runtime %d), want %d", got.SoftLimit, debug.SetMemoryLimit(-1), want)
		}
	})

	t.Run("operator GOGC is left alone", func(t *testing.T) {
		preserveRuntime(t)
		debug.SetGCPercent(150)
		got := tuneRuntime(0, env(map[string]string{"GOGC": "150"}), func() uint64 { return 0 })
		if got.GCPercentSet || got.GCPercent != 150 || currentGCPercent() != 150 {
			t.Fatalf("operator GOGC overridden: %+v, runtime %d", got, currentGCPercent())
		}
	})

	t.Run("operator GOMEMLIMIT is left alone and used for shedding", func(t *testing.T) {
		preserveRuntime(t)
		debug.SetMemoryLimit(gib)
		got := tuneRuntime(0, env(map[string]string{"GOMEMLIMIT": "1GiB"}), func() uint64 { return 4 * gib })
		if got.MemoryLimit != gib || got.SoftLimit != 0 || debug.SetMemoryLimit(-1) != gib {
			t.Fatalf("operator GOMEMLIMIT not respected: %+v, runtime %d", got, debug.SetMemoryLimit(-1))
		}
	})

	t.Run("explicit limit wins over the container's", func(t *testing.T) {
		preserveRuntime(t)
		debug.SetMemoryLimit(math.MaxInt64)
		got := tuneRuntime(gib, env(nil), func() uint64 { return 4 * gib })
		if got.MemoryLimit != gib || got.SoftLimit != soft(gib) {
			t.Fatalf("explicit limit not used: %+v", got)
		}
	})

	t.Run("no limit known sets no GOMEMLIMIT", func(t *testing.T) {
		preserveRuntime(t)
		debug.SetMemoryLimit(math.MaxInt64)
		got := tuneRuntime(0, env(nil), func() uint64 { return 0 })
		if got.MemoryLimit != 0 || got.SoftLimit != 0 || debug.SetMemoryLimit(-1) != math.MaxInt64 {
			t.Fatalf("GOMEMLIMIT set without a limit: %+v, runtime %d", got, debug.SetMemoryLimit(-1))
		}
		if currentGCPercent() != DefaultGCPercent {
			t.Fatalf("GOGC not defaulted without a limit: %d", currentGCPercent())
		}
	})
}

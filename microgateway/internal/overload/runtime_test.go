package overload

import (
	"math"
	"runtime/debug"
	"testing"
	"time"
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

func runtimeGCPercent() int {
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
		if !got.GCPercentSet || !got.GCPercentAdaptive || got.GCPercent != DefaultGCPercent || runtimeGCPercent() != DefaultGCPercent {
			t.Fatalf("GOGC not defaulted: %+v, runtime %d", got, runtimeGCPercent())
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
		if got.GCPercentSet || got.GCPercentAdaptive || got.GCPercent != 150 || runtimeGCPercent() != 150 {
			t.Fatalf("operator GOGC overridden: %+v, runtime %d", got, runtimeGCPercent())
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
		if runtimeGCPercent() != DefaultGCPercent {
			t.Fatalf("GOGC not defaulted without a limit: %d", runtimeGCPercent())
		}
	})
}

// GOGC follows the live heap: 400 while it is small (the measured REST win),
// tapering to Go's default of 100 once the extra ~256 MB of garbage would be a
// small share of it, so a heap of thousands of streams no longer grows ~5x.
func TestAdaptiveGCPercent(t *testing.T) {
	const mib = 1 << 20
	for _, c := range []struct {
		live uint64
		want int
	}{
		{0, DefaultGCPercent},
		{20 * mib, 400},  // REST: tens of MB live
		{64 * mib, 400},  // 256/64 = 400%
		{100 * mib, 250}, // 256%, in steps of 25
		{128 * mib, 200},
		{256 * mib, 100},
		{400 * mib, 100}, // streaming at the knee: Go's default
		{4096 * mib, 100},
	} {
		if got := adaptiveGCPercent(c.live); got != c.want {
			t.Errorf("adaptiveGCPercent(%d MiB) = %d, want %d", c.live/mib, got, c.want)
		}
	}
}

// The adaptation loop sets a GOGC in range and stops when told to.
func TestAdaptGCLoop(t *testing.T) {
	preserveRuntime(t)
	currentGCPercent.Store(-1)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { adaptGC(stop); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for currentGCPercent.Load() == -1 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	close(stop)
	<-done
	got := CurrentGCPercent()
	if got < minGCPercent || got > DefaultGCPercent || runtimeGCPercent() != got {
		t.Fatalf("adapted GOGC = %d (runtime %d), want within [%d, %d]", got, runtimeGCPercent(), minGCPercent, DefaultGCPercent)
	}
}

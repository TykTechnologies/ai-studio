package run

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/probe"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/scenario"
)

// realisticTTFT draws TTFTs shaped like the mock's realistic profile: 300ms
// median with a tail to about 900ms at p99.
func realisticTTFT(r *rand.Rand) float64 { return 300 * math.Exp(0.47*r.NormFloat64()) }

// feed adds 10 s of traffic at rps to l, ending ago before now, the baseline
// arm at 10% weight.
func feed(l *live, r *rand.Rand, rps int, shift float64, ago time.Duration) {
	now := time.Now().Add(-ago)
	for i := 0; i < rps*10; i++ {
		at := now.Add(-time.Duration(i) * (10 * time.Second) / time.Duration(rps*10)).UnixNano()
		l.add(probe.Result{Arm: "gateway", Intended: at, Status: 200, TTFT: realisticTTFT(r) + shift})
		if i%10 == 0 {
			l.add(probe.Result{Arm: "direct", Intended: at, Status: 200, TTFT: realisticTTFT(r)})
		}
	}
}

// At the bottom of a ramp the baseline has ~20 requests in the stop window,
// whose p99 is little more than their maximum. Identical distributions must
// not stop the ramp (it stopped S4 at 21 req/s on a gateway adding <1ms).
func TestStopIgnoresThinBaselineNoise(t *testing.T) {
	stop := scenario.Stop{MaxErrorRate: 0.02, MaxP99OverheadMS: 500, Metric: "ttft"}
	for seed := uint64(1); seed <= 200; seed++ {
		l := newLive("direct")
		var reason string
		check := l.stopper(stop, func(s string) { reason = s })
		r := rand.New(rand.NewPCG(seed, 3))
		feed(l, r, 21, 0, 0)
		if check() {
			t.Fatalf("seed %d: identical distributions stopped the ramp: %s", seed, reason)
		}
	}
}

// A real collapse still stops it, once there are enough samples to tell.
func TestStopCatchesRealOverhead(t *testing.T) {
	stop := scenario.Stop{MaxErrorRate: 0.02, MaxP99OverheadMS: 500, Metric: "ttft"}
	l := newLive("direct")
	var reason string
	check := l.stopper(stop, func(s string) { reason = s })
	r := rand.New(rand.NewPCG(7, 3))
	// A minute of normal traffic earlier in the ramp, then the collapse.
	for ago := 60 * time.Second; ago > 0; ago -= 10 * time.Second {
		feed(l, r, 100, 0, ago)
	}
	feed(l, r, 100, 800, 0)
	if !check() {
		t.Fatal("an 800ms shift on 1,000 requests did not stop the ramp")
	}
	if reason == "" {
		t.Fatal("no stop reason")
	}
}

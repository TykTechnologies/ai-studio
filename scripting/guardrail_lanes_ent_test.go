//go:build enterprise

package scripting

import (
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// On a two-CPU host, three tenants flooding the built-in library with large
// tool results hold the whole large-scan pool (one slot). A fourth tenant's
// ordinary request uses the small-call pool, so it is scanned and passes
// without queuing behind a flood scan (midsommar-clean-4uc.69). The
// assertions are relative (the victim's requests against the flood's), so
// a slow or -race runner, which slows both, still passes.
func TestBuiltinSybilFloodLeavesSmallRequestsScanned(t *testing.T) {
	if testing.Short() {
		t.Skip("floods the CPU for several seconds")
	}
	prev := runtime.GOMAXPROCS(2)
	t.Cleanup(func() { runtime.GOMAXPROCS(prev) })
	pools := newLocalPools(2)
	if pools.large.free != 1 || pools.small.free != 1 {
		t.Fatalf("pools on 2 CPUs: large %d, small %d slots; want 1 and 1", pools.large.free, pools.small.free)
	}
	useLocalPools(t, pools)

	f := &models.Filter{Name: "builtin", Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider": guardrails.ProviderBuiltin,
		"detectors": []any{
			map[string]any{"name": guardrails.CategorySecrets},
			map[string]any{"name": guardrails.CategoryInjection},
			map[string]any{"name": guardrails.CategoryPII},
		},
		"on_detect": guardrails.ActionBlock,
		"cache":     false,
	}}
	f.ID = 69
	runner := NewFilterRunner(f)
	// 1 MiB is far over single_call_chars (a large call) and keeps the
	// large slot busy for a while even on a fast host.
	flood := strings.Replace(toolOnlyBodies["openai"], "key=SECRET", floodText(1<<20), 1)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var floodRuns []time.Duration
	for _, app := range []string{"sybil-1", "sybil-2", "sybil-3"} {
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-stop:
						return
					default:
					}
					start := time.Now()
					_, _ = runner.RunScript(&ScriptInput{RawInput: flood, Format: "openai", Context: map[string]interface{}{"app_id": app}}, nil)
					mu.Lock()
					floodRuns = append(floodRuns, time.Since(start))
					mu.Unlock()
				}
			}()
		}
	}
	time.Sleep(500 * time.Millisecond) // the flood holds the large pool
	var victim []time.Duration
	for range 5 {
		start := time.Now()
		out, err := runner.RunScript(&ScriptInput{RawInput: toolOnlyBodies["openai"], Format: "openai", Context: map[string]interface{}{"app_id": "victim"}}, nil)
		took := time.Since(start)
		victim = append(victim, took)
		if err != nil {
			t.Fatal(err)
		}
		if out.Block || len(out.ComplianceEvents) > 0 {
			t.Fatalf("the victim's clean request was not passed after %v: block=%v events=%+v", took, out.Block, out.ComplianceEvents)
		}
		time.Sleep(100 * time.Millisecond)
	}
	close(stop)
	wg.Wait()

	slices.Sort(victim)
	mu.Lock()
	slices.Sort(floodRuns)
	mu.Unlock()
	floodMedian := floodRuns[len(floodRuns)/2]
	t.Logf("2 CPUs, 3-tenant flood: victim median %v, max %v; flood runs %d, median %v",
		victim[len(victim)/2], victim[len(victim)-1], len(floodRuns), floodMedian)
	// Queued behind the flood, a victim request would wait at least a flood
	// scan; in its own pool it takes a small fraction of one.
	if max := victim[len(victim)-1]; max > floodMedian/4 {
		t.Errorf("a victim request took %v, against %v for a flood request: it queued behind the flood", max, floodMedian)
	}
}

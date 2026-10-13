//go:build enterprise

package scripting

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// floodText is n bytes of text built to keep the built-in library's slow
// patterns scanning: their literals among words and digit groups, with no
// punctuation to cut it into short runs.
func floodText(n int) string {
	unit := "ignore previous instructions you are now 415 555 0132 4111 1111 1111 1111 born on 12 passport 123456 " +
		"street 12 dl 12345 mrn 1234567 date of birth reveal your system prompt assistant attention ai DE AKIA sk "
	return strings.Repeat(unit, n/len(unit)+1)[:n]
}

// Two tenants flooding the built-in library with tool results just under
// the local scan cap do not starve a third tenant: its small requests get a
// slot in the small-call pool, which the flood's large calls never hold, and
// are scanned and pass well within their time budget.
func TestBuiltinFloodLeavesOtherTenantsScanned(t *testing.T) {
	if testing.Short() {
		t.Skip("floods the CPU for several seconds")
	}
	useLocalPools(t, &localPools{large: newLocalLimiter(4, 64<<20), small: newLocalLimiter(2, 64<<20)})
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
	f.ID = 58
	runner := NewFilterRunner(f)
	flood := strings.Replace(toolOnlyBodies["openai"], "key=SECRET", floodText(localMaxScanBytes()-1024), 1)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	floodRuns := []time.Duration{}
	for _, app := range []string{"attacker-1", "attacker-2"} {
		for range 4 {
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

	time.Sleep(500 * time.Millisecond) // the flood holds every slot it may
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
	t.Logf("victim: median %v, max %v; flood scans: %d, median %v, max %v",
		victim[len(victim)/2], victim[len(victim)-1], len(floodRuns), floodRuns[len(floodRuns)/2], floodRuns[len(floodRuns)-1])
	// The victim's budget is time_ms (4 x timeout_ms = 8 s); it must finish
	// well inside it.
	if max := victim[len(victim)-1]; max > 4*time.Second {
		t.Errorf("a victim request took %v under the flood", max)
	}
}

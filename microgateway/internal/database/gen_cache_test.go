package database

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/datatypes"
)

func TestDeepCopySharesNoMutableState(t *testing.T) {
	start := time.Now()
	orig := &App{
		Name:            "a",
		BudgetStartDate: &start,
		Metadata:        datatypes.JSON(`{"k":"v"}`),
		LLMs:            []LLM{{Name: "l", GovernedMetadata: datatypes.JSON(`{"g":1}`)}},
		Tools:           []Tool{{Name: "t", Filters: []Filter{{Name: "f"}}}},
	}
	orig.ID = 7

	cp := DeepCopy(orig)
	if cp == orig || cp.ID != 7 || cp.Name != "a" || !cp.BudgetStartDate.Equal(start) {
		t.Fatalf("copy is not an equal, separate value: %+v", cp)
	}

	cp.Name = "changed"
	*cp.BudgetStartDate = start.Add(time.Hour)
	cp.Metadata[2] = 'X'
	cp.LLMs[0].Name = "changed"
	cp.LLMs[0].GovernedMetadata[2] = 'X'
	cp.Tools[0].Filters[0].Name = "changed"
	cp.LLMs = append(cp.LLMs, LLM{Name: "extra"})

	if orig.Name != "a" || !orig.BudgetStartDate.Equal(start) || string(orig.Metadata) != `{"k":"v"}` ||
		orig.LLMs[0].Name != "l" || string(orig.LLMs[0].GovernedMetadata) != `{"g":1}` ||
		orig.Tools[0].Filters[0].Name != "f" || len(orig.LLMs) != 1 {
		t.Fatalf("modifying the copy changed the original: %+v", orig)
	}

	var nilApp *App
	if DeepCopy(nilApp) != nil {
		t.Fatal("a nil pointer copies to nil")
	}
	m := map[string][]int{"a": {1}}
	mc := DeepCopy(m)
	mc["a"][0] = 2
	if m["a"][0] != 1 {
		t.Fatal("map values must be copied")
	}
}

// Concurrent misses for one key share a single load: after a configuration
// change, in-flight requests do not all go to the database.
func TestGenCacheCollapsesConcurrentLoads(t *testing.T) {
	c := NewGenCache[string, int]()
	var loads atomic.Int32
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.Load("k", func() (int, error) {
				loads.Add(1)
				<-release
				return 42, nil
			})
			if err != nil || v != 42 {
				t.Errorf("got %d %v", v, err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond) // let the goroutines pile up on the miss
	close(release)
	wg.Wait()
	if n := loads.Load(); n != 1 {
		t.Fatalf("expected one load for 50 concurrent misses, got %d", n)
	}
}

// When the cache is full, stale entries are swept rather than dropping every
// valid one.
func TestGenCacheOverflowSweepsInvalidEntriesFirst(t *testing.T) {
	c := NewGenCache[int, int]()
	for i := 0; i < genCacheMaxEntries-1; i++ {
		c.m[i] = genEntry[int]{v: i, gen: ConfigGeneration(), until: time.Now().Add(-time.Second)} // expired
	}
	if _, err := c.Load(-1, func() (int, error) { return 1, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Load(-2, func() (int, error) { return 2, nil }); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(-1); !ok {
		t.Fatal("a valid entry must survive the overflow sweep")
	}
	if len(c.m) != 2 {
		t.Fatalf("expired entries should have been swept, %d left", len(c.m))
	}
}

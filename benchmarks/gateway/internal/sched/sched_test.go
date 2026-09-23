package sched

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/probe"
)

// A target that serves one request at a time and stalls on the first one. An
// open-loop schedule keeps releasing requests during the stall, and each must
// be charged the time it spent queued behind it, measured from when it was
// meant to be sent. A closed-loop tool would report ~1ms for all but one.
func TestRunOpenChargesQueueingToEachRequest(t *testing.T) {
	var mu sync.Mutex
	first := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if first {
			first = false
			time.Sleep(300 * time.Millisecond)
		} else {
			time.Sleep(time.Millisecond)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cell := Cell{
		Name:    "stall",
		Arms:    []Arm{{Name: "target", Target: probe.Target{URL: srv.URL}, Client: NewClient(false)}},
		Request: probe.Request{Format: probe.FormatOpenAI, Body: []byte(`{}`)},
	}
	var rmu sync.Mutex
	var results []probe.Result
	RunOpen(context.Background(), cell, OpenOptions{Rate: Constant(100, 500*time.Millisecond), Timeout: 5 * time.Second},
		func(r probe.Result) { rmu.Lock(); results = append(results, r); rmu.Unlock() })

	if len(results) < 45 {
		t.Fatalf("expected ~50 requests, got %d", len(results))
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Intended < results[j].Intended })
	// Requests intended in the first 150ms waited for the 300ms stall to end.
	t0 := results[0].Intended
	var early []float64
	for _, r := range results {
		if !r.OK() {
			t.Fatalf("request failed: %+v", r)
		}
		if time.Duration(r.Intended-t0) < 150*time.Millisecond {
			early = append(early, r.Total)
		}
	}
	sort.Float64s(early)
	if med := early[len(early)/2]; med < 120 {
		t.Fatalf("median latency of requests queued behind the stall = %.1fms, want >= 120ms", med)
	}
	for _, r := range results {
		if r.Lag > 50 {
			t.Fatalf("load generator released a request %.1fms late", r.Lag)
		}
	}
}

func TestRunClosedAlternatesArmsAndMarksWarmup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer srv.Close()
	c := NewClient(false)
	cell := Cell{Name: "c", Arms: []Arm{
		{Name: "a", Target: probe.Target{URL: srv.URL}, Client: c},
		{Name: "b", Target: probe.Target{URL: srv.URL}, Client: c},
	}, Request: probe.Request{Body: []byte(`{}`)}}

	counts := map[string]int{}
	warm := 0
	var mu sync.Mutex
	RunClosed(context.Background(), cell, ClosedOptions{Concurrency: 1, Requests: 10, WarmupRequests: 3}, func(r probe.Result) {
		mu.Lock()
		defer mu.Unlock()
		counts[r.Arm]++
		if r.Warmup {
			warm++
		}
	})
	if counts["a"] != 13 || counts["b"] != 13 || warm != 6 {
		t.Fatalf("counts=%v warm=%d", counts, warm)
	}
}

func TestRunPairedSendsABBABlocks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer srv.Close()
	c := NewClient(false)
	cell := Cell{Name: "p", Arms: []Arm{
		{Name: "direct", Target: probe.Target{URL: srv.URL}, Client: c},
		{Name: "gateway", Target: probe.Target{URL: srv.URL}, Client: c},
	}, Request: probe.Request{Body: []byte(`{}`)}}

	blocks := map[int64][]string{}
	var mu sync.Mutex
	RunPaired(context.Background(), cell, PairedOptions{Blocks: 5}, func(r probe.Result) {
		mu.Lock()
		blocks[r.Seq/4] = append(blocks[r.Seq/4], r.Arm)
		mu.Unlock()
	})
	if len(blocks) != 5 {
		t.Fatalf("blocks=%d", len(blocks))
	}
	for id, arms := range blocks {
		if len(arms) != 4 || arms[0] != arms[3] || arms[1] != arms[2] || arms[0] == arms[1] {
			t.Fatalf("block %d is not ABBA: %v", id, arms)
		}
	}
}

func TestStepsAndSpikeProfiles(t *testing.T) {
	s := Steps(10, 10, 30, time.Minute)
	if r, ok := s(0); r != 10 || !ok {
		t.Fatal(r, ok)
	}
	if r, ok := s(2*time.Minute + time.Second); r != 30 || !ok {
		t.Fatal(r, ok)
	}
	if _, ok := s(3 * time.Minute); ok {
		t.Fatal("steps should end after max level")
	}
	sp := Spike(10, 30, time.Minute, 30*time.Second, 3*time.Minute)
	if r, _ := sp(70 * time.Second); r != 30 {
		t.Fatal(r)
	}
	if r, _ := sp(100 * time.Second); r != 10 {
		t.Fatal(r)
	}
}

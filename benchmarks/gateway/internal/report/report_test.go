package report

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/probe"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/run"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/sampler"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/scenario"
)

// writeRun fabricates a run directory: a gateway arm 2ms slower than direct.
func writeRun(t *testing.T, sc *scenario.Scenario, n int, gatewayErrs int) string {
	t.Helper()
	dir := t.TempDir()
	start := time.Now().Add(-time.Minute).UTC()
	r := rand.New(rand.NewPCG(1, 2))

	f, err := os.Create(filepath.Join(dir, "results.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for i := 0; i < n; i++ {
		for _, arm := range []string{"direct", "gateway"} {
			base := 10 + r.ExpFloat64()
			res := probe.Result{Cell: "c", Arm: arm, Seq: int64(i*2) + map[string]int64{"direct": 0, "gateway": 1}[arm],
				Intended: start.Add(time.Duration(i) * 10 * time.Millisecond).UnixNano(), Status: 200,
				TTFT: base, Total: base + 1, TTFB: base, Lag: 0.01}
			if arm == "gateway" {
				res.TTFT += 2
				res.Total += 2
				res.Timing = map[string]float64{"gw": 1.5, "gw-pre": 1.0}
				res.ConnGw = "reused"
				if i < gatewayErrs {
					res.Status, res.Err = 502, ""
				}
			}
			_ = enc.Encode(res)
		}
	}
	f.Close()

	sf, _ := os.Create(filepath.Join(dir, "samples.jsonl"))
	senc := json.NewEncoder(sf)
	for i := 0; i < 30; i++ {
		_ = senc.Encode(sampler.Sample{T: start.Add(time.Duration(i) * time.Second).UnixNano(),
			Gateway: map[string]float64{"process_resident_memory_bytes": 50e6, "go_goroutines": 40, "process_cpu_seconds_total": float64(i) * 0.5}})
	}
	sf.Close()

	m := run.Manifest{Scenario: sc, StartedAt: start, FinishedAt: start.Add(time.Minute),
		Cells:     []run.CellRecord{{Name: "c", Started: start, Finished: start.Add(time.Minute), Arms: []string{"gateway", "direct"}}},
		Analytics: &run.Analytics{Before: 10, After: 10 + n, Expected: n, Settled: true}}
	b, _ := json.Marshal(m)
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o644)
	return dir
}

func TestWriteProducesAllOutputsAndFindsOverhead(t *testing.T) {
	sc := &scenario.Scenario{Name: "t", Title: "Test", Mode: "closed", Baseline: "direct"}
	dir := writeRun(t, sc, 2000, 0)
	s, err := Write(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"summary.json", "report.md", "report.html"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatal(err)
		}
	}
	var ttft *Overhead
	for i, o := range s.Cells[0].Overhead {
		if o.Metric == "ttft" {
			ttft = &s.Cells[0].Overhead[i]
		}
	}
	if ttft == nil || !ttft.P50.Contains(2) || ttft.P50.Contains(0) {
		t.Fatalf("overhead %+v should be about 2ms", ttft)
	}
	if !s.Valid {
		t.Fatalf("expected valid run, checks: %+v", s.Checks)
	}
	if res := s.Cells[0].Resources; res == nil || res.CPUCoresAvg < 0.49 || res.CPUCoresAvg > 0.51 {
		t.Fatalf("resources %+v", res)
	}
	md, _ := os.ReadFile(filepath.Join(dir, "report.md"))
	if !strings.Contains(string(md), "VALID") || !strings.Contains(string(md), "Overhead summary") {
		t.Fatal("report.md missing sections")
	}
}

func TestCalibrationFailsWhenArmsDiffer(t *testing.T) {
	sc := &scenario.Scenario{Name: "aa", Mode: "closed", Baseline: "direct", Calibration: true}
	s, err := Write(writeRun(t, sc, 2000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if s.Valid {
		t.Fatal("an A/A run with a 2ms difference must be invalid")
	}
}

func TestErrorsAreCountedNotTimed(t *testing.T) {
	sc := &scenario.Scenario{Name: "e", Mode: "closed", Baseline: "direct"}
	s, err := Write(writeRun(t, sc, 500, 50))
	if err != nil {
		t.Fatal(err)
	}
	var gw ArmSummary
	for _, a := range s.Cells[0].Arms {
		if a.Arm == "gateway" {
			gw = a
		}
	}
	if gw.Errors != 50 || gw.TTFT.N != 450 || gw.ErrorKinds["HTTP 502"] != 50 {
		t.Fatalf("%+v", gw)
	}
	failedWarn := false
	for _, c := range s.Checks {
		if strings.HasPrefix(c.Name, "error rate (gateway)") && !c.Passed {
			failedWarn = true
		}
	}
	if !failedWarn {
		t.Fatal("10% errors should fail the error-rate check")
	}
}

func TestKneeFindsFirstBreach(t *testing.T) {
	ws := []Window{
		{BaselineCumN: 300, BaselineCumTTFTP99: 300, Arms: map[string]WinStat{"gateway": {N: 500, RPS: 100, TTFTP99: 310, GWTTFBP99: math.NaN()}}},
		{BaselineCumN: 300, BaselineCumTTFTP99: 300, Arms: map[string]WinStat{"gateway": {N: 500, RPS: 200, TTFTP99: 320, GWTTFBP99: math.NaN()}}},
		{BaselineCumN: 300, BaselineCumTTFTP99: 300, Arms: map[string]WinStat{"gateway": {N: 500, RPS: 300, TTFTP99: 400, GWTTFBP99: math.NaN()}}},
	}
	k := knee(ws, "direct", 0.001, 25)
	if k.SustainedRPS != 200 || k.BrokeAtRPS != 300 {
		t.Fatalf("%+v", k)
	}
}

// With Server-Timing, the gateway's own p99 decides even while the baseline
// is too small to compare against.
func TestKneeUsesServerTiming(t *testing.T) {
	ws := []Window{
		{BaselineCumN: 10, Arms: map[string]WinStat{"gateway": {N: 500, RPS: 100, TTFTP99: 900, GWTTFBP99: 3}}},
		{BaselineCumN: 20, Arms: map[string]WinStat{"gateway": {N: 500, RPS: 200, TTFTP99: 950, GWTTFBP99: 40}}},
	}
	k := knee(ws, "direct", 0.001, 25)
	if k.SustainedRPS != 100 || k.BrokeAtRPS != 200 {
		t.Fatalf("%+v", k)
	}
}

// A window with too few requests for a meaningful p99 makes no latency
// decision; errors still count.
func TestKneeIgnoresThinWindowsForLatency(t *testing.T) {
	ws := []Window{
		{BaselineCumN: 300, BaselineCumTTFTP99: 300, Arms: map[string]WinStat{"gateway": {N: 30, RPS: 50, TTFTP99: 900, GWTTFBP99: math.NaN()}}},
		{BaselineCumN: 300, BaselineCumTTFTP99: 300, Arms: map[string]WinStat{"gateway": {N: 500, RPS: 100, TTFTP99: 310, GWTTFBP99: math.NaN()}}},
		{BaselineCumN: 300, BaselineCumTTFTP99: 300, Arms: map[string]WinStat{"gateway": {N: 30, RPS: 150, ErrorRate: 0.1, GWTTFBP99: math.NaN()}}},
	}
	k := knee(ws, "direct", 0.001, 25)
	if k.SustainedRPS != 100 || k.BrokeAtRPS != 150 {
		t.Fatalf("%+v", k)
	}
}

// The baseline reference pools every window so far, so one noisy window of
// the small baseline sample cannot fake or hide a breach.
func TestWindowsPoolBaseline(t *testing.T) {
	start := time.Now()
	var recs []rec
	for i := 0; i < 100; i++ {
		recs = append(recs, rec{arm: "direct", ok: true, ttft: 100, intended: start.Add(time.Duration(i) * 10 * time.Millisecond).UnixNano()})
	}
	for i := 0; i < 1; i++ { // second window: one slow baseline sample
		recs = append(recs, rec{arm: "direct", ok: true, ttft: 1000, intended: start.Add(time.Second + time.Duration(i)*time.Millisecond).UnixNano()})
	}
	recs = append(recs, rec{arm: "direct", ok: true, ttft: 100, intended: start.Add(2 * time.Second).UnixNano()})
	ws := windows(recs, start, time.Second, "direct")
	if len(ws) < 2 || ws[1].BaselineCumTTFTP99 >= 1000 {
		t.Fatalf("pooled baseline p99 should be dominated by the earlier samples: %+v", ws)
	}
}

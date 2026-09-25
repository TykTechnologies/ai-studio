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
		{BaselineCumN: 300, BaselineCumTTFTP99: 300, Arms: map[string]WinStat{"gateway": {N: 500, RPS: 100, TTFTP99: 310, TTFTP99OverLo: 2, GWTTFBP99: math.NaN()}}},
		{BaselineCumN: 300, BaselineCumTTFTP99: 300, Arms: map[string]WinStat{"gateway": {N: 500, RPS: 200, TTFTP99: 320, TTFTP99OverLo: 12, GWTTFBP99: math.NaN()}}},
		{BaselineCumN: 300, BaselineCumTTFTP99: 300, Arms: map[string]WinStat{"gateway": {N: 500, RPS: 300, TTFTP99: 400, TTFTP99OverLo: 80, GWTTFBP99: math.NaN()}}},
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

// A point estimate of the p99 difference over the baseline's p99 is not
// enough: against an upstream with a heavy-tailed TTFT, both p99s move by tens
// of milliseconds between samples. A step breaches only when the difference's
// confidence interval lies above the SLO.
func TestKneeNeedsCIAboveSLO(t *testing.T) {
	ws := []Window{
		{BaselineCumN: 700, BaselineCumTTFTP99: 795, Arms: map[string]WinStat{"gateway": {N: 5000, RPS: 90, TTFTP99: 897, TTFTP99OverLo: -20, GWTTFBP99: 0.9}}},
		{BaselineCumN: 900, BaselineCumTTFTP99: 800, Arms: map[string]WinStat{"gateway": {N: 8000, RPS: 140, TTFTP99: 1100, TTFTP99OverLo: 180, GWTTFBP99: 0.9}}},
	}
	k := knee(ws, "direct", 0.001, 25)
	if k.SustainedRPS != 90 || k.BrokeAtRPS != 140 {
		t.Fatalf("%+v", k)
	}
}

// heavyTail draws TTFTs shaped like the mock's realistic profile: 300ms
// median with a long tail to about 900ms at p99.
func heavyTail(r *rand.Rand) float64 {
	return 300 * math.Exp(0.47*r.NormFloat64())
}

// Same upstream distribution on both arms, 10% baseline weight: the noise in
// the p99s alone must not produce a breach. With a real shift it must.
func TestWindowsKneeIgnoresTailNoise(t *testing.T) {
	build := func(shift float64) []rec {
		r := rand.New(rand.NewPCG(7, 11))
		start := time.Unix(0, 0)
		var recs []rec
		for w := 0; w < 5; w++ {
			for i := 0; i < 3000; i++ {
				at := start.Add(time.Duration(w)*time.Minute + time.Duration(i)*20*time.Millisecond).UnixNano()
				recs = append(recs, rec{arm: "gateway", ok: true, ttft: heavyTail(r) + shift, intended: at, gw: math.NaN(), gwTTFB: math.NaN()})
				if i%10 == 0 {
					recs = append(recs, rec{arm: "direct", ok: true, ttft: heavyTail(r), intended: at, gw: math.NaN(), gwTTFB: math.NaN()})
				}
			}
		}
		return recs
	}
	k := knee(windows(build(0), time.Unix(0, 0), time.Minute, "direct"), "direct", 0.001, 25)
	if k.BrokeAtRPS != 0 {
		t.Fatalf("identical distributions breached: %+v", k)
	}
	k = knee(windows(build(300), time.Unix(0, 0), time.Minute, "direct"), "direct", 0.001, 25)
	if k.BrokeAtRPS == 0 {
		t.Fatalf("a 300ms shift did not breach: %+v", k)
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

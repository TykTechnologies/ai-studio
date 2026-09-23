// Package report turns a run directory (manifest.json, results.jsonl,
// samples.jsonl) into summary.json, report.md and report.html. Everything is
// recomputed from the raw per-request records, so a report can be regenerated
// or re-analysed without re-running the benchmark.
package report

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/probe"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/run"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/sampler"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/stats"
)

// rec is the compact in-memory form of one result.
type rec struct {
	arm      string
	seq      int64
	intended int64
	ok       bool
	status   int
	err      string
	ttfb     float64
	ttft     float64
	total    float64
	lag      float64
	// Server-Timing, NaN when absent.
	gw, gwPre, gwTTFB, upstream float64
	gwConn                      string
}

// Summary is the machine-readable result of a run.
type Summary struct {
	Scenario    string            `json:"scenario"`
	Title       string            `json:"title"`
	Mode        string            `json:"mode"`
	Baseline    string            `json:"baseline"`
	Quick       bool              `json:"quick"`
	Label       string            `json:"label,omitempty"`
	GitSHA      string            `json:"git_sha,omitempty"`
	GitDirty    bool              `json:"git_dirty,omitempty"`
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  time.Time         `json:"finished_at"`
	Host        run.HostInfo      `json:"load_generator"`
	Cells       []CellSummary     `json:"cells"`
	Skipped     map[string]string `json:"skipped,omitempty"`
	Checks      []Check           `json:"checks"`
	Valid       bool              `json:"valid"`
	Description string            `json:"description,omitempty"`
	Targets     map[string]string `json:"targets"`
	Analytics   *run.Analytics    `json:"analytics,omitempty"`
}

// Check is one validity check. A failed "fail" check invalidates the run.
type Check struct {
	Cell     string `json:"cell,omitempty"`
	Name     string `json:"name"`
	Severity string `json:"severity"` // fail | warn
	Passed   bool   `json:"passed"`
	Detail   string `json:"detail"`
}

// CellSummary holds everything reported for one cell.
type CellSummary struct {
	Name      string       `json:"name"`
	Arms      []ArmSummary `json:"arms"`
	Overhead  []Overhead   `json:"overhead,omitempty"`
	Windows   []Window     `json:"windows,omitempty"`
	Knee      *Knee        `json:"knee,omitempty"`
	StoppedBy string       `json:"stopped_by,omitempty"`
	Resources *Resources   `json:"resources,omitempty"`
	Started   time.Time    `json:"started"`
	Finished  time.Time    `json:"finished"`
}

// ArmSummary is one arm's distribution.
type ArmSummary struct {
	Arm        string                   `json:"arm"`
	Requests   int                      `json:"requests"`
	Errors     int                      `json:"errors"`
	RateLimits int                      `json:"rate_limited"`
	ErrorKinds map[string]int           `json:"error_kinds,omitempty"`
	TTFB       stats.Summary            `json:"ttfb_ms"`
	TTFT       stats.Summary            `json:"ttft_ms"`
	Total      stats.Summary            `json:"total_ms"`
	Lag        stats.Summary            `json:"lag_ms"`
	Timing     map[string]stats.Summary `json:"server_timing_ms,omitempty"`
	// UpstreamNewConnPct is the share of requests on which the gateway
	// opened a new connection to the upstream (from Server-Timing conn).
	UpstreamNewConnPct float64 `json:"upstream_new_conn_pct"`
	AchievedRPS        float64 `json:"achieved_rps"`
}

// Overhead compares an arm with the baseline arm.
type Overhead struct {
	Arm    string `json:"arm"`
	Metric string `json:"metric"` // ttft | total
	// Independent-sample differences of percentiles (arm - baseline).
	P50 stats.CI `json:"p50"`
	P90 stats.CI `json:"p90"`
	P99 stats.CI `json:"p99"`
	// Paired: median of within-block differences (paired mode only).
	Paired *stats.CI `json:"paired_median,omitempty"`
	Pairs  int       `json:"pairs,omitempty"`
}

// Window is one time bucket of an open-loop cell.
type Window struct {
	StartS    float64            `json:"start_s"`
	OfferedRS float64            `json:"offered_rps"`
	Arms      map[string]WinStat `json:"arms"`
	// BaselineCumTTFTP99 is the baseline arm's p99 TTFT over this window and
	// every earlier one. The baseline is a small sample (10% of traffic in
	// the ramps) against an upstream whose latency does not depend on the
	// gateway's load, so pooling it gives a stable reference where a single
	// window's p99 is noise.
	BaselineCumTTFTP99 float64 `json:"baseline_cum_ttft_p99"`
	BaselineCumN       int     `json:"baseline_cum_n"`
}

// WinStat is one arm within a window.
type WinStat struct {
	N         int     `json:"n"`
	RPS       float64 `json:"rps"`
	ErrorRate float64 `json:"error_rate"`
	TTFTP50   float64 `json:"ttft_p50"`
	TTFTP99   float64 `json:"ttft_p99"`
	TotalP50  float64 `json:"total_p50"`
	TotalP99  float64 `json:"total_p99"`
	GWP99     float64 `json:"gw_p99"`
	// GWTTFBP99 is the gateway's own p99 share of time to first byte
	// (Server-Timing gw-ttfb); NaN when the gateway did not report it.
	GWTTFBP99 float64 `json:"gw_ttfb_p99"`
}

// Knee is where a ramp stopped being sustainable.
type Knee struct {
	// StoppedBy is set when the run's stop condition ended the ramp, during
	// the step after SustainedRPS.
	StoppedBy string `json:"stopped_by,omitempty"`
	// SustainedRPS is the highest offered rate whose window met the SLO.
	SustainedRPS float64 `json:"sustained_rps"`
	// BrokeAtRPS is the first offered rate that did not, 0 if none did.
	BrokeAtRPS float64 `json:"broke_at_rps"`
	Reason     string  `json:"reason,omitempty"`
	SLOMS      float64 `json:"slo_ms"`
}

// Resources summarises gateway samples taken while a cell ran.
type Resources struct {
	Samples          int        `json:"samples"`
	RSSStartMB       float64    `json:"rss_start_mb"`
	RSSEndMB         float64    `json:"rss_end_mb"`
	RSSMaxMB         float64    `json:"rss_max_mb"`
	RSSSlopeMBPerH   float64    `json:"rss_slope_mb_per_hour"`
	GoroutinesStart  float64    `json:"goroutines_start"`
	GoroutinesEnd    float64    `json:"goroutines_end"`
	GoroutinesMax    float64    `json:"goroutines_max"`
	GoroutineSlopePH float64    `json:"goroutines_slope_per_hour"`
	FDsMax           float64    `json:"open_fds_max"`
	CPUCoresAvg      float64    `json:"cpu_cores_avg"`
	CPUCoresMax      float64    `json:"cpu_cores_max"`
	Series           []ResPoint `json:"series,omitempty"`
}

// ResPoint is one sample in the series.
type ResPoint struct {
	T          float64 `json:"t_s"`
	RSSMB      float64 `json:"rss_mb"`
	Goroutines float64 `json:"goroutines"`
	FDs        float64 `json:"fds"`
	CPUCores   float64 `json:"cpu_cores"`
}

// Analyze reads a run directory and computes the Summary.
func Analyze(dir string) (*Summary, error) {
	var m run.Manifest
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("reading manifest (did the run finish?): %w", err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	sc := m.Scenario

	byCell, err := readResults(filepath.Join(dir, "results.jsonl"))
	if err != nil {
		return nil, err
	}
	samples, _ := readSamples(filepath.Join(dir, "samples.jsonl"))

	s := &Summary{
		Scenario: sc.Name, Title: sc.Title, Mode: sc.Mode, Baseline: sc.Baseline, Quick: m.Quick,
		Label: m.Label, GitSHA: m.GitSHA, GitDirty: m.GitDirty, StartedAt: m.StartedAt,
		FinishedAt: m.FinishedAt, Host: m.Host, Skipped: m.Skipped, Description: sc.Description,
		Targets: m.Targets, Valid: true,
	}
	slo := sc.OverheadSLOMS
	if slo <= 0 {
		slo = 25
	}

	for ci, cell := range m.Cells {
		recs := byCell[cell.Name]
		cs := CellSummary{Name: cell.Name, Started: cell.Started, Finished: cell.Finished, StoppedBy: m.StoppedBy[cell.Name]}
		dur := cell.Finished.Sub(cell.Started).Seconds()

		arms := groupByArm(recs)
		for _, arm := range cell.Arms {
			cs.Arms = append(cs.Arms, summarizeArm(arm, arms[arm], dur))
		}
		base := arms[sc.Baseline]
		for _, arm := range cell.Arms {
			if arm == sc.Baseline || base == nil {
				continue
			}
			seed := uint64(ci*31 + len(arm))
			for _, metric := range []string{"ttft", "total"} {
				o := Overhead{Arm: arm, Metric: metric,
					P50: stats.DiffOfPercentiles(values(base, metric), values(arms[arm], metric), 50, 0.95, seed),
					P90: stats.DiffOfPercentiles(values(base, metric), values(arms[arm], metric), 90, 0.95, seed+1),
					P99: stats.DiffOfPercentiles(values(base, metric), values(arms[arm], metric), 99, 0.95, seed+2),
				}
				if sc.Mode == "paired" {
					d := pairedDiffs(recs, arm, sc.Baseline, metric)
					ci := stats.MedianOfPaired(d, 0.95, seed+3)
					o.Paired, o.Pairs = &ci, len(d)
				}
				cs.Overhead = append(cs.Overhead, o)
			}
		}

		if sc.Mode == "open" {
			win := sc.Open.ReportWindow
			if win <= 0 {
				switch sc.Open.Rate.Kind {
				case "steps":
					win = sc.Open.Rate.Hold
				case "spike":
					win = 5 * time.Second
				default:
					win = 60 * time.Second
				}
			}
			cs.Windows = windows(recs, cell.Started, win, sc.Baseline)
			if sc.Open.Rate.Kind == "steps" {
				cs.Knee = knee(cs.Windows, sc.Baseline, sc.Open.Stop.MaxErrorRate, slo)
				// A ramp ended by its stop condition broke at its last
				// step, whatever the per-window check concluded.
				if cs.StoppedBy != "" && cs.Knee.BrokeAtRPS == 0 {
					cs.Knee.StoppedBy = cs.StoppedBy
				}
			}
		}
		cs.Resources = resources(samples, cell.Started, cell.Finished)
		s.Cells = append(s.Cells, cs)
	}

	maxLag := sc.Open.MaxLagMS
	if maxLag <= 0 {
		maxLag = 1
	}
	s.Checks = checks(sc.Mode, sc.Calibration, sc.Baseline, maxLag, s.Cells)
	if a := m.Analytics; a != nil {
		got := a.After - a.Before
		c := Check{Name: "analytics recorded every gateway request", Severity: "warn", Passed: a.Err == "" && got >= a.Expected,
			Detail: fmt.Sprintf("Studio gained %d proxy-log rows for %d gateway responses", got, a.Expected)}
		if a.Err != "" {
			c.Detail += " (" + a.Err + ")"
		} else if !a.Settled {
			c.Detail += " (count still rising when the wait ended)"
		}
		if got > a.Expected {
			c.Detail += "; extra rows come from other traffic to the same app during the run"
		}
		s.Checks = append(s.Checks, c)
		s.Analytics = a
	}
	for _, c := range s.Checks {
		if c.Severity == "fail" && !c.Passed {
			s.Valid = false
		}
	}
	return s, nil
}

func readResults(path string) (map[string][]rec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string][]rec{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	for sc.Scan() {
		var r probe.Result
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if r.Warmup {
			continue
		}
		t := func(k string) float64 {
			if v, ok := r.Timing[k]; ok {
				return v
			}
			return math.NaN()
		}
		out[r.Cell] = append(out[r.Cell], rec{
			arm: r.Arm, seq: r.Seq, intended: r.Intended, ok: r.OK(), status: r.Status, err: r.Err,
			ttfb: r.TTFB, ttft: r.TTFT, total: r.Total, lag: r.Lag,
			gw: t("gw"), gwPre: t("gw-pre"), gwTTFB: t("gw-ttfb"), upstream: t("upstream"), gwConn: r.ConnGw,
		})
	}
	return out, sc.Err()
}

func readSamples(path string) ([]sampler.Sample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []sampler.Sample
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var s sampler.Sample
		if json.Unmarshal(sc.Bytes(), &s) == nil {
			out = append(out, s)
		}
	}
	return out, sc.Err()
}

func groupByArm(recs []rec) map[string][]rec {
	out := map[string][]rec{}
	for _, r := range recs {
		out[r.arm] = append(out[r.arm], r)
	}
	return out
}

// values returns the metric for successful requests only: a failed request
// has no meaningful latency, and is counted in the error rate instead.
func values(recs []rec, metric string) []float64 {
	out := make([]float64, 0, len(recs))
	for _, r := range recs {
		if !r.ok {
			continue
		}
		switch metric {
		case "ttft":
			out = append(out, r.ttft)
		case "ttfb":
			out = append(out, r.ttfb)
		case "lag":
			out = append(out, r.lag)
		default:
			out = append(out, r.total)
		}
	}
	return out
}

func summarizeArm(arm string, recs []rec, durS float64) ArmSummary {
	a := ArmSummary{Arm: arm, Requests: len(recs), ErrorKinds: map[string]int{}, Timing: map[string]stats.Summary{}}
	var lag []float64
	timing := map[string][]float64{}
	var withConn, newConn int
	for _, r := range recs {
		lag = append(lag, r.lag)
		if r.status == 429 {
			a.RateLimits++
		}
		if !r.ok {
			a.Errors++
			k := r.err
			if k == "" {
				k = fmt.Sprintf("HTTP %d", r.status)
			}
			a.ErrorKinds[k]++
			continue
		}
		for k, v := range map[string]float64{"gw": r.gw, "gw-pre": r.gwPre, "gw-ttfb": r.gwTTFB, "upstream": r.upstream} {
			if !math.IsNaN(v) {
				timing[k] = append(timing[k], v)
			}
		}
		if r.gwConn != "" {
			withConn++
			if r.gwConn == "new" {
				newConn++
			}
		}
	}
	a.TTFB = stats.Summarize(values(recs, "ttfb"))
	a.TTFT = stats.Summarize(values(recs, "ttft"))
	a.Total = stats.Summarize(values(recs, "total"))
	a.Lag = stats.Summarize(lag)
	for k, v := range timing {
		a.Timing[k] = stats.Summarize(v)
	}
	if withConn > 0 {
		a.UpstreamNewConnPct = 100 * float64(newConn) / float64(withConn)
	}
	if durS > 0 {
		a.AchievedRPS = float64(len(recs)) / durS
	}
	return a
}

// pairedDiffs differences arm and baseline within each ABBA block (4
// consecutive seq numbers), using block means. Blocks with any failed request
// are dropped.
func pairedDiffs(recs []rec, arm, baseline, metric string) []float64 {
	type block struct {
		a, b   []float64
		failed bool
	}
	blocks := map[int64]*block{}
	for _, r := range recs {
		id := r.seq / 4
		bl := blocks[id]
		if bl == nil {
			bl = &block{}
			blocks[id] = bl
		}
		if !r.ok {
			bl.failed = true
			continue
		}
		v := r.total
		if metric == "ttft" {
			v = r.ttft
		}
		switch r.arm {
		case arm:
			bl.a = append(bl.a, v)
		case baseline:
			bl.b = append(bl.b, v)
		}
	}
	var out []float64
	for _, bl := range blocks {
		if bl.failed || len(bl.a) == 0 || len(bl.b) == 0 {
			continue
		}
		out = append(out, mean(bl.a)-mean(bl.b))
	}
	sort.Float64s(out)
	return out
}

func mean(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func windows(recs []rec, start time.Time, win time.Duration, baseline string) []Window {
	if len(recs) == 0 {
		return nil
	}
	t0 := start.UnixNano()
	type bucket map[string][]rec
	buckets := map[int]bucket{}
	maxIdx := 0
	for _, r := range recs {
		idx := int((r.intended - t0) / int64(win))
		if idx < 0 {
			idx = 0
		}
		if buckets[idx] == nil {
			buckets[idx] = bucket{}
		}
		buckets[idx][r.arm] = append(buckets[idx][r.arm], r)
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	var out []Window
	var baseCum []float64
	for i := 0; i <= maxIdx; i++ {
		b := buckets[i]
		if b == nil {
			continue
		}
		w := Window{StartS: float64(i) * win.Seconds(), Arms: map[string]WinStat{}}
		baseCum = append(baseCum, values(b[baseline], "ttft")...)
		w.BaselineCumTTFTP99 = stats.Percentile(baseCum, 99)
		w.BaselineCumN = len(baseCum)
		var n int
		for arm, rs := range b {
			n += len(rs)
			errs := 0
			var gw, gwTTFB []float64
			for _, r := range rs {
				if !r.ok {
					errs++
					continue
				}
				if !math.IsNaN(r.gw) {
					gw = append(gw, r.gw)
				}
				if !math.IsNaN(r.gwTTFB) {
					gwTTFB = append(gwTTFB, r.gwTTFB)
				}
			}
			ttft, total := values(rs, "ttft"), values(rs, "total")
			w.Arms[arm] = WinStat{
				N: len(rs), RPS: float64(len(rs)) / win.Seconds(), ErrorRate: float64(errs) / float64(len(rs)),
				TTFTP50: stats.Percentile(ttft, 50), TTFTP99: stats.Percentile(ttft, 99),
				TotalP50: stats.Percentile(total, 50), TotalP99: stats.Percentile(total, 99),
				GWP99:     stats.Percentile(gw, 99),
				GWTTFBP99: stats.Percentile(gwTTFB, 99),
			}
		}
		w.OfferedRS = float64(n) / win.Seconds()
		out = append(out, w)
	}
	// The last window is usually partial; its offered rate would read low.
	if len(out) > 1 {
		out = out[:len(out)-1]
	}
	return out
}

// kneeMinSamples is the fewest requests a p99 is computed from before it is
// compared with the SLO; below it the p99 is little more than the maximum.
const kneeMinSamples = 200

// knee finds the highest window that met the SLO before the first that did
// not. A window breaches when the measured arm's error rate exceeds maxErr, or
// its p99 overhead on time to first byte exceeds the SLO by either measure:
//
//   - the gateway's own account (Server-Timing gw-ttfb), exact per request
//     and free of baseline noise, but blind to queueing before the handler
//     (a full accept backlog at saturation);
//   - the measured arm's p99 TTFT against the baseline's p99 pooled over the
//     ramp so far, which sees everything but is only trusted once both sides
//     have kneeMinSamples.
//
// Rates are the measured arm's own request rate.
func knee(ws []Window, baseline string, maxErr, slo float64) *Knee {
	if maxErr <= 0 {
		maxErr = 0.001
	}
	k := &Knee{SLOMS: slo}
	for _, w := range ws {
		var rate float64
		for arm, st := range w.Arms {
			if arm == baseline {
				continue
			}
			rate = math.Max(rate, st.RPS)
			reason := ""
			switch {
			case st.ErrorRate > maxErr:
				reason = fmt.Sprintf("%s error rate %.2f%%", arm, 100*st.ErrorRate)
			case st.N >= kneeMinSamples && !math.IsNaN(st.GWTTFBP99) && st.GWTTFBP99 > slo:
				reason = fmt.Sprintf("%s Server-Timing p99 gateway time to first byte %.1fms (SLO %.0fms)", arm, st.GWTTFBP99, slo)
			case st.N >= kneeMinSamples && w.BaselineCumN >= kneeMinSamples &&
				!math.IsNaN(w.BaselineCumTTFTP99) && st.TTFTP99-w.BaselineCumTTFTP99 > slo:
				reason = fmt.Sprintf("%s p99 TTFT %.1fms above baseline (SLO %.0fms)", arm, st.TTFTP99-w.BaselineCumTTFTP99, slo)
			}
			if reason != "" {
				k.BrokeAtRPS, k.Reason = st.RPS, reason
				return k
			}
		}
		k.SustainedRPS = rate
	}
	return k
}

func resources(samples []sampler.Sample, from, to time.Time) *Resources {
	var pts []ResPoint
	var prevCPU, prevT float64
	havePrev := false
	for _, s := range samples {
		if s.T < from.UnixNano() || s.T > to.UnixNano() || s.Gateway == nil {
			continue
		}
		t := float64(s.T-from.UnixNano()) / 1e9
		p := ResPoint{T: t, RSSMB: s.Gateway["process_resident_memory_bytes"] / 1e6,
			Goroutines: s.Gateway["go_goroutines"], FDs: s.Gateway["process_open_fds"], CPUCores: math.NaN()}
		cpu, hasCPU := s.Gateway["process_cpu_seconds_total"]
		if hasCPU && havePrev && t > prevT {
			p.CPUCores = (cpu - prevCPU) / (t - prevT)
		}
		if hasCPU {
			prevCPU, prevT, havePrev = cpu, t, true
		}
		pts = append(pts, p)
	}
	if len(pts) == 0 {
		return nil
	}
	r := &Resources{Samples: len(pts), Series: pts,
		RSSStartMB: pts[0].RSSMB, RSSEndMB: pts[len(pts)-1].RSSMB,
		GoroutinesStart: pts[0].Goroutines, GoroutinesEnd: pts[len(pts)-1].Goroutines}
	var ts, rss, gr, cpu []float64
	for _, p := range pts {
		r.RSSMaxMB = math.Max(r.RSSMaxMB, p.RSSMB)
		r.GoroutinesMax = math.Max(r.GoroutinesMax, p.Goroutines)
		r.FDsMax = math.Max(r.FDsMax, p.FDs)
		ts = append(ts, p.T/3600)
		rss = append(rss, p.RSSMB)
		gr = append(gr, p.Goroutines)
		if !math.IsNaN(p.CPUCores) {
			cpu = append(cpu, p.CPUCores)
			r.CPUCoresMax = math.Max(r.CPUCoresMax, p.CPUCores)
		}
	}
	if len(cpu) > 0 {
		r.CPUCoresAvg = mean(cpu)
	}
	// Growth rates only mean something over a long enough run.
	if pts[len(pts)-1].T >= 600 {
		r.RSSSlopeMBPerH = stats.Slope(ts, rss)
		r.GoroutineSlopePH = stats.Slope(ts, gr)
	}
	// Keep the stored series small: at most ~600 points.
	if n := len(pts); n > 600 {
		step := n / 600
		var thin []ResPoint
		for i := 0; i < n; i += step {
			thin = append(thin, pts[i])
		}
		r.Series = thin
	}
	return r
}

// checks are the conditions under which a run's numbers can be trusted.
func checks(mode string, calibration bool, baseline string, maxLag float64, cells []CellSummary) []Check {
	var out []Check
	for _, c := range cells {
		for _, a := range c.Arms {
			if mode == "open" {
				lag := a.Lag.P99
				out = append(out, Check{Cell: c.Name, Name: "load generator kept schedule (" + a.Arm + ")", Severity: "fail",
					Passed: lag <= maxLag,
					Detail: fmt.Sprintf("p99 release lag %.3fms (limit %.0fms); beyond it the load generator, not the target, shapes the results", lag, maxLag)})
			}
			if a.Requests > 0 {
				rate := float64(a.Errors) / float64(a.Requests)
				out = append(out, Check{Cell: c.Name, Name: "error rate (" + a.Arm + ")", Severity: "warn",
					Passed: rate <= 0.001,
					Detail: fmt.Sprintf("%d of %d failed (%.3f%%)%s", a.Errors, a.Requests, 100*rate, errorKinds(a.ErrorKinds))})
				// Latency percentiles cover successful requests only. In the
				// measurement scenarios (no deliberate overload) more than 1%
				// failures means the comparison is between different
				// populations, so the cell's numbers cannot be quoted. Under
				// open-loop stress, errors are a result, not a defect of the
				// measurement.
				if mode != "open" && rate > 0.01 {
					out = append(out, Check{Cell: c.Name, Name: "few enough failures to compare (" + a.Arm + ")", Severity: "fail",
						Passed: false,
						Detail: fmt.Sprintf("%.1f%% of requests failed; latency figures would describe only the survivors", 100*rate)})
				}
			}
		}
		for _, o := range c.Overhead {
			if calibration && o.Metric == "total" {
				out = append(out, Check{Cell: c.Name, Name: "A/A calibration: no difference between identical arms", Severity: "fail",
					Passed: o.P50.Contains(0) || math.Abs(o.P50.Estimate) < 0.25,
					Detail: fmt.Sprintf("median difference %.3fms, 95%% CI [%.3f, %.3f]", o.P50.Estimate, o.P50.Lo, o.P50.Hi)})
			}
		}
		// Cross-check: the gateway's own Server-Timing "gw" should roughly
		// match the black-box difference of medians. Only meaningful when
		// the system is unloaded (closed mode).
		if mode == "closed" && !calibration {
			for _, a := range c.Arms {
				gw, ok := a.Timing["gw"]
				if !ok || a.Arm == baseline {
					continue
				}
				for _, o := range c.Overhead {
					if o.Arm != a.Arm || o.Metric != "total" {
						continue
					}
					diff := math.Abs(o.P50.Estimate - gw.P50)
					tol := math.Max(1.0, 0.5*math.Abs(o.P50.Estimate))
					out = append(out, Check{Cell: c.Name, Name: "Server-Timing agrees with black-box overhead (" + a.Arm + ")", Severity: "warn",
						Passed: diff <= tol,
						Detail: fmt.Sprintf("black-box median overhead %.3fms vs Server-Timing gw median %.3fms; the gap is network and kernel time outside the gateway process",
							o.P50.Estimate, gw.P50)})
				}
			}
		}
	}
	return out
}

func errorKinds(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	var parts []string
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s x%d", k, v))
	}
	sort.Strings(parts)
	return ": " + strings.Join(parts, ", ")
}

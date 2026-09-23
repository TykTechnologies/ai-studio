// Package run executes a resolved scenario: it drives every cell, writes each
// request's measurements to results.jsonl, samples the gateway's resource use
// alongside, and records a manifest describing exactly what was measured.
package run

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/probe"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/sampler"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/scenario"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/sched"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/stats"
)

// Options configures one scenario run.
type Options struct {
	Scenario *scenario.Scenario
	Env      scenario.Env
	OutRoot  string
	Quick    bool
	// Label is free text recorded in the manifest (e.g. "cloud c7i.2xlarge").
	Label   string
	Sampler *sampler.Config
	// AnalyticsCount, when set, returns how many proxy-log rows Studio holds
	// for the benchmark app. It is read before and after the run to check
	// that the gateway recorded every request it answered.
	AnalyticsCount func(context.Context) (int, error)
	// AnalyticsArm is the arm whose requests reach the gateway (default
	// "gateway").
	AnalyticsArm string
	Log          func(string, ...any)
}

// Analytics is the before/after proxy-log count.
type Analytics struct {
	Before int `json:"before"`
	After  int `json:"after"`
	// Expected is how many requests the gateway answered (any HTTP status).
	Expected int `json:"expected"`
	// Settled is false if the count was still changing when we gave up.
	Settled bool   `json:"settled"`
	Err     string `json:"err,omitempty"`
}

// Manifest describes a run well enough to reproduce or challenge it.
type Manifest struct {
	Scenario   *scenario.Scenario `json:"scenario"`
	Quick      bool               `json:"quick"`
	Label      string             `json:"label,omitempty"`
	StartedAt  time.Time          `json:"started_at"`
	FinishedAt time.Time          `json:"finished_at"`
	Cells      []CellRecord       `json:"cells"`
	Skipped    map[string]string  `json:"skipped,omitempty"`
	Targets    map[string]string  `json:"targets"` // name -> base URL (no credentials)
	Host       HostInfo           `json:"load_generator"`
	GitSHA     string             `json:"git_sha,omitempty"`
	GitDirty   bool               `json:"git_dirty,omitempty"`
	Seed       map[string]any     `json:"seed,omitempty"`
	StoppedBy  map[string]string  `json:"stopped_by,omitempty"`
	Analytics  *Analytics         `json:"analytics,omitempty"`
}

// CellRecord is when each cell ran, so time windows can be reconstructed.
type CellRecord struct {
	Name     string    `json:"name"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	Arms     []string  `json:"arms"`
}

// HostInfo describes the load-generator host.
type HostInfo struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	CPUs     int    `json:"cpus"`
	Kernel   string `json:"kernel,omitempty"`
	Go       string `json:"go"`
}

// Run executes the scenario and returns the output directory.
func Run(ctx context.Context, o Options) (string, error) {
	sc := o.Scenario
	if o.Quick {
		sc.ApplyQuick()
	}
	cells, skipped, err := sc.Resolve(o.Env)
	if err != nil {
		return "", err
	}
	if len(cells) == 0 {
		return "", fmt.Errorf("scenario %s: every cell was skipped: %v", sc.Name, skipped)
	}

	dir := filepath.Join(o.OutRoot, time.Now().UTC().Format("20060102-150405")+"-"+sc.Name)
	if o.Quick {
		dir += "-quick"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	m := &Manifest{Scenario: sc, Quick: o.Quick, Label: o.Label, StartedAt: time.Now().UTC(),
		Skipped: skipped, Targets: map[string]string{}, Host: hostInfo(), StoppedBy: map[string]string{}}
	m.GitSHA, m.GitDirty = gitInfo()
	for name, t := range o.Env.Targets {
		m.Targets[name] = t.BaseURL
	}
	if o.Env.State != nil {
		m.Seed = map[string]any{"seeded_at": o.Env.State.SeededAt, "config_checksum": o.Env.State.Checksum,
			"models": o.Env.State.Models, "app_id": o.Env.State.AppID}
	}
	for name, reason := range skipped {
		o.Log("skipping cell %s: %s", name, reason)
	}

	w, err := newWriter(filepath.Join(dir, "results.jsonl"))
	if err != nil {
		return "", err
	}
	defer w.close()

	analyticsArm := o.AnalyticsArm
	if analyticsArm == "" {
		analyticsArm = "gateway"
	}
	var answered atomic.Int64
	if o.AnalyticsCount != nil {
		m.Analytics = &Analytics{}
		if m.Analytics.Before, err = o.AnalyticsCount(ctx); err != nil {
			o.Log("analytics count unavailable, skipping that check: %v", err)
			m.Analytics = nil
		}
	}

	var stopSampler func()
	if o.Sampler != nil {
		stopSampler, err = sampler.Start(ctx, *o.Sampler, filepath.Join(dir, "samples.jsonl"), o.Log)
		if err != nil {
			return "", err
		}
	}

	for _, cell := range cells {
		if ctx.Err() != nil {
			break
		}
		rec := CellRecord{Name: cell.Name, Started: time.Now().UTC()}
		for _, a := range cell.Arms {
			rec.Arms = append(rec.Arms, a.Name)
		}
		o.Log("cell %s: running (%s)", cell.Name, sc.Mode)

		live := newLive(sc.Baseline)
		sink := func(r probe.Result) {
			w.write(r)
			live.add(r)
			if r.Arm == analyticsArm && r.Status > 0 {
				answered.Add(1)
			}
		}
		switch sc.Mode {
		case "closed":
			sched.RunClosed(ctx, cell, sched.ClosedOptions{
				Concurrency: sc.Closed.Concurrency, Requests: sc.Closed.Requests,
				WarmupRequests: sc.Closed.WarmupRequests, Timeout: sc.Timeout,
			}, sink)
		case "paired":
			sched.RunPaired(ctx, cell, sched.PairedOptions{
				Blocks: sc.Paired.Blocks, WarmupBlocks: sc.Paired.WarmupBlocks,
				Workers: sc.Paired.Workers, Gap: sc.Paired.Gap, Timeout: sc.Timeout,
			}, sink)
		case "open":
			rate, err := sc.Open.RateFunc()
			if err != nil {
				return "", err
			}
			stopCell := live.stopper(sc.Open.Stop, func(reason string) {
				m.StoppedBy[cell.Name] = reason
				o.Log("cell %s: stopping early: %s", cell.Name, reason)
			})
			sched.RunOpen(ctx, cell, sched.OpenOptions{
				Rate: rate, Poisson: sc.Open.Arrival == "poisson", Warmup: sc.Open.Warmup,
				Timeout: sc.Timeout, MaxInFlight: sc.Open.MaxInFlight, Stop: stopCell,
			}, sink)
		}
		rec.Finished = time.Now().UTC()
		m.Cells = append(m.Cells, rec)
		for _, line := range live.summary() {
			o.Log("  %s", line)
		}
	}

	if stopSampler != nil {
		stopSampler()
	}
	m.FinishedAt = time.Now().UTC()
	if m.Analytics != nil {
		m.Analytics.Expected = int(answered.Load())
		waitForAnalytics(ctx, o, m.Analytics)
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o644); err != nil {
		return "", err
	}
	return dir, ctx.Err()
}

// waitForAnalytics polls until the count covers every answered request, or
// stops changing for 30s. Edge analytics reach Studio on the analytics pulse,
// so they lag the traffic by up to one pulse interval plus a batch write.
func waitForAnalytics(ctx context.Context, o Options, a *Analytics) {
	o.Log("waiting for analytics to reach Studio (%d requests answered by the gateway)...", a.Expected)
	deadline := time.Now().Add(5 * time.Minute)
	last, lastChange := -1, time.Now()
	for {
		n, err := o.AnalyticsCount(ctx)
		if err != nil {
			a.Err = err.Error()
			return
		}
		a.After = n
		if n-a.Before >= a.Expected {
			a.Settled = true
			return
		}
		if n != last {
			last, lastChange = n, time.Now()
		}
		if time.Since(lastChange) > 30*time.Second || time.Now().After(deadline) || ctx.Err() != nil {
			a.Settled = time.Since(lastChange) > 30*time.Second
			return
		}
		time.Sleep(3 * time.Second)
	}
}

// --- result writer -------------------------------------------------------------

type writer struct {
	mu   sync.Mutex
	f    *os.File
	buf  *bufio.Writer
	enc  *json.Encoder
	done chan struct{}
}

func newWriter(path string) (*writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	buf := bufio.NewWriterSize(f, 1<<20)
	w := &writer{f: f, buf: buf, enc: json.NewEncoder(buf), done: make(chan struct{})}
	// Flush every few seconds, so a long run can be watched as it goes and
	// an interrupted one keeps what it measured.
	go func() {
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-w.done:
				return
			case <-t.C:
				w.mu.Lock()
				_ = w.buf.Flush()
				w.mu.Unlock()
			}
		}
	}()
	return w, nil
}

func (w *writer) write(r probe.Result) {
	w.mu.Lock()
	_ = w.enc.Encode(r)
	w.mu.Unlock()
}

func (w *writer) close() {
	close(w.done)
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.buf.Flush()
	_ = w.f.Close()
}

// --- live view ---------------------------------------------------------------

// live keeps just enough in memory to print a per-cell summary and to evaluate
// ramp stop conditions over a sliding window.
type live struct {
	mu       sync.Mutex
	baseline string
	byArm    map[string]*armLive
	// recent feeds the ramp stop check; kept only when one is configured.
	trackRecent bool
	recent      []probe.Result
}

type armLive struct {
	ttft, total []float64
	n, errs     int
	errKinds    map[string]int
}

func newLive(baseline string) *live {
	return &live{baseline: baseline, byArm: map[string]*armLive{}}
}

func (l *live) add(r probe.Result) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if r.Warmup {
		return
	}
	a := l.byArm[r.Arm]
	if a == nil {
		a = &armLive{errKinds: map[string]int{}}
		l.byArm[r.Arm] = a
	}
	a.n++
	if !r.OK() {
		a.errs++
		k := r.Err
		if k == "" {
			k = fmt.Sprintf("HTTP %d", r.Status)
		}
		a.errKinds[k]++
	} else {
		a.ttft = append(a.ttft, r.TTFT)
		a.total = append(a.total, r.Total)
	}
	if l.trackRecent {
		l.recent = append(l.recent, r)
	}
}

const stopWindow = 10 * time.Second

// stopper returns the ramp stop check, or nil when no condition is set.
func (l *live) stopper(s scenario.Stop, onStop func(string)) func() bool {
	if s.MaxErrorRate <= 0 && s.MaxP99OverheadMS <= 0 {
		return nil
	}
	l.mu.Lock()
	l.trackRecent = true
	l.mu.Unlock()
	return func() bool {
		l.mu.Lock()
		cutoff := time.Now().Add(-stopWindow).UnixNano()
		kept := l.recent[:0]
		for _, r := range l.recent {
			if r.Intended >= cutoff {
				kept = append(kept, r)
			}
		}
		l.recent = kept
		window := append([]probe.Result(nil), kept...)
		l.mu.Unlock()

		byArm := map[string][]float64{}
		errs := map[string]int{}
		n := map[string]int{}
		for _, r := range window {
			n[r.Arm]++
			if !r.OK() {
				errs[r.Arm]++
				continue
			}
			v := r.Total
			if s.Metric == "ttft" {
				v = r.TTFT
			}
			byArm[r.Arm] = append(byArm[r.Arm], v)
		}
		for arm := range n {
			if arm == l.baseline || n[arm] < 50 {
				continue
			}
			if rate := float64(errs[arm]) / float64(n[arm]); s.MaxErrorRate > 0 && rate > s.MaxErrorRate {
				onStop(fmt.Sprintf("%s error rate %.2f%% over the last %v", arm, 100*rate, stopWindow))
				return true
			}
			base := byArm[l.baseline]
			if s.MaxP99OverheadMS > 0 && len(base) >= 20 && len(byArm[arm]) >= 50 {
				over := stats.Percentile(byArm[arm], 99) - stats.Percentile(base, 99)
				if over > s.MaxP99OverheadMS {
					onStop(fmt.Sprintf("%s p99 %s is %.1fms above %s over the last %v", arm, metricName(s.Metric), over, l.baseline, stopWindow))
					return true
				}
			}
		}
		return false
	}
}

func metricName(m string) string {
	if m == "ttft" {
		return "TTFT"
	}
	return "total"
}

func (l *live) summary() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var names []string
	for k := range l.byArm {
		names = append(names, k)
	}
	sort.Strings(names)
	var out []string
	for _, name := range names {
		a := l.byArm[name]
		line := fmt.Sprintf("%-10s n=%-7d err=%-5d TTFT p50=%8.2f p99=%8.2f  total p50=%8.2f p99=%8.2f ms",
			name, a.n, a.errs,
			stats.Percentile(a.ttft, 50), stats.Percentile(a.ttft, 99),
			stats.Percentile(a.total, 50), stats.Percentile(a.total, 99))
		if a.errs > 0 {
			var kinds []string
			for k, v := range a.errKinds {
				kinds = append(kinds, fmt.Sprintf("%s x%d", k, v))
			}
			sort.Strings(kinds)
			line += "  errors: " + strings.Join(kinds, ", ")
		}
		out = append(out, line)
	}
	return out
}

// --- environment ---------------------------------------------------------------

func hostInfo() HostInfo {
	h := HostInfo{OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(), Go: runtime.Version()}
	h.Hostname, _ = os.Hostname()
	if out, err := exec.Command("uname", "-sr").Output(); err == nil {
		h.Kernel = strings.TrimSpace(string(out))
	}
	return h
}

// gitInfo identifies the code under test. Inside the tools container there is
// no checkout, so the Makefile passes GWBENCH_GIT_SHA / GWBENCH_GIT_DIRTY.
func gitInfo() (string, bool) {
	if sha := strings.TrimSpace(os.Getenv("GWBENCH_GIT_SHA")); sha != "" {
		return sha, os.Getenv("GWBENCH_GIT_DIRTY") == "true"
	}
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", false
	}
	status, _ := exec.Command("git", "status", "--porcelain", "--untracked-files=no").Output()
	return strings.TrimSpace(string(out)), len(strings.TrimSpace(string(status))) > 0
}

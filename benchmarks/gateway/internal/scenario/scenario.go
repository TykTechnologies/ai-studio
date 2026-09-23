// Package scenario loads benchmark scenario files and resolves them into
// runnable cells: which targets each arm hits, with which body, under which
// load shape.
package scenario

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/probe"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/sched"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/seed"
)

// Scenario is one YAML file.
type Scenario struct {
	Name        string `yaml:"name"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	// Mode is closed, open or paired.
	Mode    string        `yaml:"mode"`
	Timeout time.Duration `yaml:"timeout"`
	// Baseline names the arm every other arm is compared against.
	Baseline string `yaml:"baseline"`
	// ColdConn opens a new client connection for every request.
	ColdConn bool `yaml:"cold_conn"`
	// Calibration marks an A/A scenario: both arms are identical, so any
	// difference the report finds is noise, and the run is only valid if the
	// confidence interval for it contains zero.
	Calibration bool `yaml:"calibration"`
	// OverheadSLOMS is the p99 overhead the report treats as the capacity
	// limit when finding the knee of a ramp (default 25ms).
	OverheadSLOMS float64 `yaml:"overhead_slo_ms"`

	Closed Closed `yaml:"closed"`
	Open   Open   `yaml:"open"`
	Paired Paired `yaml:"paired"`

	// Quick scales request counts and durations for smoke runs.
	QuickScale float64 `yaml:"quick_scale"`

	Cells []Cell `yaml:"cells"`
}

// Closed is the closed-loop load shape.
type Closed struct {
	Concurrency    int `yaml:"concurrency"`
	Requests       int `yaml:"requests"`
	WarmupRequests int `yaml:"warmup_requests"`
}

// Open is the open-loop load shape.
type Open struct {
	Arrival     string        `yaml:"arrival"` // constant | poisson
	Rate        Rate          `yaml:"rate"`
	Warmup      time.Duration `yaml:"warmup"`
	MaxInFlight int           `yaml:"max_in_flight"`
	Stop        Stop          `yaml:"stop"`
	// ReportWindow buckets results over time in the report (defaults: the
	// step hold for steps, 5s for spike, 60s otherwise).
	ReportWindow time.Duration `yaml:"report_window"`
	// MaxLagMS is the p99 release lag above which the run is invalid
	// (default 1ms). Lag is always charged to the request's latency, so it
	// cannot hide queueing; the check catches a saturated load generator,
	// whose arrivals no longer follow the schedule. Scenarios measuring
	// latencies of hundreds of milliseconds can tolerate a few.
	MaxLagMS float64 `yaml:"max_lag_ms"`
}

// Rate describes how the arrival rate changes over the run.
type Rate struct {
	Kind     string        `yaml:"kind"` // constant | steps | spike
	RPS      float64       `yaml:"rps"`
	Duration time.Duration `yaml:"duration"`
	Start    float64       `yaml:"start"`
	Step     float64       `yaml:"step"`
	Max      float64       `yaml:"max"`
	Hold     time.Duration `yaml:"hold"`
	Base     float64       `yaml:"base"`
	Peak     float64       `yaml:"peak"`
	At       time.Duration `yaml:"at"`
	Width    time.Duration `yaml:"width"`
}

// Stop ends a ramp early once the system is clearly past its limit. Each
// check, once a second, looks at the last 10 seconds of results.
type Stop struct {
	MaxErrorRate float64 `yaml:"max_error_rate"`
	// MaxP99OverheadMS compares the gateway arm's p99 to the baseline's.
	MaxP99OverheadMS float64 `yaml:"max_p99_overhead_ms"`
	// Metric is ttft or total.
	Metric string `yaml:"metric"`
}

// Paired is the ABBA shape for real upstreams.
type Paired struct {
	Blocks       int           `yaml:"blocks"`
	WarmupBlocks int           `yaml:"warmup_blocks"`
	Workers      int           `yaml:"workers"`
	Gap          time.Duration `yaml:"gap"`
}

// Cell is one measured configuration.
type Cell struct {
	Name string         `yaml:"name"`
	Body probe.BodySpec `yaml:"body"`
	Arms []Arm          `yaml:"arms"`
	// Requires lists target names that must be configured; a cell whose
	// targets are missing (e.g. no Anthropic key) is skipped, and the skip is
	// reported.
	Requires []string `yaml:"requires"`
}

// Arm is one side of a cell.
type Arm struct {
	Name    string            `yaml:"name"`
	Target  string            `yaml:"target"` // gateway | mock | openai | anthropic
	Path    string            `yaml:"path"`
	Weight  float64           `yaml:"weight"`
	Headers map[string]string `yaml:"headers"`
	// Model overrides the body's model for this arm only.
	Model string `yaml:"model"`
}

// Load reads and validates a scenario file.
func Load(path string) (*Scenario, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Scenario
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if s.Name == "" || len(s.Cells) == 0 {
		return nil, fmt.Errorf("%s: name and at least one cell are required", path)
	}
	switch s.Mode {
	case "closed", "open", "paired":
	default:
		return nil, fmt.Errorf("%s: mode must be closed, open or paired", path)
	}
	for _, c := range s.Cells {
		if s.Mode == "paired" && len(c.Arms) != 2 {
			return nil, fmt.Errorf("%s: cell %s: paired mode needs exactly two arms", path, c.Name)
		}
	}
	return &s, nil
}

// ApplyQuick shrinks the scenario for a smoke run.
func (s *Scenario) ApplyQuick() {
	f := s.QuickScale
	if f <= 0 {
		f = 0.1
	}
	scaleI := func(n int, min int) int {
		v := int(float64(n) * f)
		if v < min {
			return min
		}
		return v
	}
	scaleD := func(d time.Duration, min time.Duration) time.Duration {
		v := time.Duration(float64(d) * f)
		if v < min {
			return min
		}
		return v
	}
	s.Closed.Requests = scaleI(s.Closed.Requests, 20)
	s.Closed.WarmupRequests = scaleI(s.Closed.WarmupRequests, 5)
	s.Paired.Blocks = scaleI(s.Paired.Blocks, 5)
	s.Paired.WarmupBlocks = scaleI(s.Paired.WarmupBlocks, 1)
	r := &s.Open.Rate
	r.Duration = scaleD(r.Duration, 10*time.Second)
	r.Hold = scaleD(r.Hold, 5*time.Second)
	r.At = scaleD(r.At, 5*time.Second)
	r.Width = scaleD(r.Width, 5*time.Second)
	s.Open.Warmup = scaleD(s.Open.Warmup, 2*time.Second)
}

// ScaleRate multiplies every rate in the open-loop spec by f, so the spike
// and soak scenarios can be sized from the capacity ramp's result.
func (s *Scenario) ScaleRate(f float64) {
	r := &s.Open.Rate
	r.RPS *= f
	r.Start *= f
	r.Step *= f
	r.Max *= f
	r.Base *= f
	r.Peak *= f
}

// RateFunc converts the rate spec.
func (o Open) RateFunc() (sched.RateFunc, error) {
	r := o.Rate
	switch r.Kind {
	case "constant", "":
		if r.RPS <= 0 || r.Duration <= 0 {
			return nil, fmt.Errorf("constant rate needs rps and duration")
		}
		return sched.Constant(r.RPS, r.Duration), nil
	case "steps":
		if r.Start <= 0 || r.Step <= 0 || r.Max < r.Start || r.Hold <= 0 {
			return nil, fmt.Errorf("steps rate needs start, step, max >= start and hold")
		}
		return sched.Steps(r.Start, r.Step, r.Max, r.Hold), nil
	case "spike":
		if r.Base <= 0 || r.Peak <= 0 || r.Width <= 0 || r.Duration <= r.At+r.Width {
			return nil, fmt.Errorf("spike rate needs base, peak, at, width and duration > at+width")
		}
		return sched.Spike(r.Base, r.Peak, r.At, r.Width, r.Duration), nil
	}
	return nil, fmt.Errorf("unknown rate kind %q", r.Kind)
}

// Target is a resolved endpoint family: a base URL plus credential headers.
type Target struct {
	BaseURL string
	Headers map[string]string
}

// Env is everything placeholders and targets resolve against.
type Env struct {
	Targets map[string]Target
	State   *seed.State
}

var placeholder = regexp.MustCompile(`\{(mock|model):([a-z0-9_-]+)\}`)

func (e Env) expand(s string) (string, error) {
	var firstErr error
	out := placeholder.ReplaceAllStringFunc(s, func(m string) string {
		parts := placeholder.FindStringSubmatch(m)
		switch parts[1] {
		case "mock":
			spec, ok := seed.MockProfiles[parts[2]]
			if !ok {
				firstErr = fmt.Errorf("unknown mock profile %q", parts[2])
				return m
			}
			return "/p/" + url.PathEscape(spec)
		case "model":
			if e.State == nil {
				firstErr = fmt.Errorf("%s needs a seed state file", m)
				return m
			}
			model, ok := e.State.Models[parts[2]]
			if !ok {
				firstErr = fmt.Errorf("no model seeded for %q", parts[2])
				return m
			}
			return model
		}
		return m
	})
	return out, firstErr
}

// Resolve turns scenario cells into scheduler cells. Cells whose required
// targets are not configured are returned in skipped with a reason.
func (s *Scenario) Resolve(env Env) (cells []sched.Cell, skipped map[string]string, err error) {
	skipped = map[string]string{}
	// One connection pool per arm name, shared across cells, so a cell does
	// not start cold because the previous one used a different pool.
	pools := map[string]*http.Client{}

	for _, c := range s.Cells {
		missing := ""
		for _, req := range append(append([]string{}, c.Requires...), armTargets(c.Arms)...) {
			if _, ok := env.Targets[req]; !ok {
				missing = req
				break
			}
		}
		if missing != "" {
			skipped[c.Name] = fmt.Sprintf("target %q is not configured", missing)
			continue
		}

		body := c.Body
		if body.Model, err = env.expand(body.Model); err != nil {
			return nil, nil, fmt.Errorf("cell %s: %w", c.Name, err)
		}
		sc := sched.Cell{Name: c.Name, Request: body.Build()}
		for _, a := range c.Arms {
			t := env.Targets[a.Target]
			path, err := env.expand(a.Path)
			if err != nil {
				return nil, nil, fmt.Errorf("cell %s arm %s: %w", c.Name, a.Name, err)
			}
			headers := map[string]string{}
			for k, v := range t.Headers {
				headers[k] = v
			}
			for k, v := range a.Headers {
				headers[k] = v
			}
			client, ok := pools[a.Name]
			if !ok {
				client = sched.NewClient(s.ColdConn)
				pools[a.Name] = client
			}
			arm := sched.Arm{
				Name:   a.Name,
				Target: probe.Target{URL: strings.TrimSuffix(t.BaseURL, "/") + path, Headers: headers},
				Weight: a.Weight,
				Client: client,
			}
			if a.Model != "" {
				ab := c.Body
				if ab.Model, err = env.expand(a.Model); err != nil {
					return nil, nil, fmt.Errorf("cell %s arm %s: %w", c.Name, a.Name, err)
				}
				req := ab.Build()
				arm.Request = &req
			}
			sc.Arms = append(sc.Arms, arm)
		}
		cells = append(cells, sc)
	}
	return cells, skipped, nil
}

func armTargets(arms []Arm) []string {
	var out []string
	for _, a := range arms {
		out = append(out, a.Target)
	}
	return out
}

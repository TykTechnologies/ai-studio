// Package sampler records the gateway's resource use during a run: RSS, CPU
// seconds, goroutines, open file descriptors and heap, scraped once a second
// from its Prometheus endpoint, plus the mock upstream's request counters. The
// report turns these into time series and growth rates, which is how a soak
// shows a leak or a stress run shows saturation.
package sampler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config says what to scrape.
type Config struct {
	GatewayMetricsURL string
	// MetricsToken is sent as a Bearer token when set (METRICS_AUTH_TOKEN).
	MetricsToken string
	MockStatsURL string
	Interval     time.Duration
}

// Metrics copied from the gateway's /metrics. Missing ones are left out of the
// sample, not zeroed.
var gatewayMetrics = []string{
	"process_resident_memory_bytes",
	"process_cpu_seconds_total",
	"process_open_fds",
	"go_goroutines",
	"go_memstats_heap_inuse_bytes",
	"go_gc_duration_seconds_count",
}

// Sample is one line of samples.jsonl.
type Sample struct {
	T       int64              `json:"t_unix_ns"`
	Gateway map[string]float64 `json:"gateway,omitempty"`
	Mock    map[string]float64 `json:"mock,omitempty"`
	Err     string             `json:"err,omitempty"`
}

// Start begins sampling into path and returns a function that stops it and
// waits for the last write.
func Start(ctx context.Context, cfg Config, path string, log func(string, ...any)) (func(), error) {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Second
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: cfg.Interval}
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	warned := false
	go func() {
		defer wg.Done()
		defer f.Close()
		enc := json.NewEncoder(f)
		tick := time.NewTicker(cfg.Interval)
		defer tick.Stop()
		for {
			s := Sample{T: time.Now().UnixNano()}
			var errs []string
			if cfg.GatewayMetricsURL != "" {
				m, err := scrapeProm(ctx, client, cfg.GatewayMetricsURL, cfg.MetricsToken)
				if err != nil {
					errs = append(errs, "gateway: "+err.Error())
				} else {
					s.Gateway = m
				}
			}
			if cfg.MockStatsURL != "" {
				m, err := scrapeJSON(ctx, client, cfg.MockStatsURL)
				if err != nil {
					errs = append(errs, "mock: "+err.Error())
				} else {
					s.Mock = m
				}
			}
			if len(errs) > 0 {
				s.Err = strings.Join(errs, "; ")
				if !warned {
					warned = true
					log("sampler: %s (further errors are recorded in samples.jsonl only)", s.Err)
				}
			}
			_ = enc.Encode(s)
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
	return func() { cancel(); wg.Wait() }, nil
}

func scrapeProm(ctx context.Context, c *http.Client, url, token string) (map[string]float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return ParseProm(resp.Body, gatewayMetrics), nil
}

// ParseProm reads the Prometheus text format and returns the unlabelled
// series named in want (labelled series of the same name are summed).
func ParseProm(r io.Reader, want []string) map[string]float64 {
	wanted := map[string]bool{}
	for _, w := range want {
		wanted[w] = true
	}
	out := map[string]float64{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		name := line
		if i := strings.IndexAny(line, "{ "); i >= 0 {
			name = line[:i]
		}
		if !wanted[name] {
			continue
		}
		rest := line[len(name):]
		if i := strings.LastIndex(rest, "}"); i >= 0 {
			rest = rest[i+1:]
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		out[name] += v
	}
	return out
}

func scrapeJSON(ctx context.Context, c *http.Client, url string) (map[string]float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

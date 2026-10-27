//go:build agenttracebench

package proxy

// Agent tracing overhead gate (spec §10, bead jyx.34). It runs one live proxy
// with an in-process OpenAI-shaped vendor and compares requests with tracing
// off against tracing on (content off, and full content where the Enterprise
// pipeline is linked), interleaved in blocks so drift hits every mode alike.
//
//	go test -tags agenttracebench -run TestAgentTraceOverheadGate -v ./proxy/
//	go test -tags 'agenttracebench enterprise' ...   (adds the full-content mode)
//
// Budgets (spec §10), gated: added latency p50 <= 0.2 ms and p99 <= 1 ms on
// non-streaming calls, streaming time to first byte p99 <= 0.5 ms. CPU
// (<= 5 % content off, <= 15 % full) is reported only: this process also
// runs the load generator and the vendor, so the CPU budget is judged on
// gwbench. AGENT_TRACE_BENCH_N sets the requests per mode (default 3000);
// AGENT_TRACE_BENCH_REPORT=1 reports without failing (for noisy hosts);
// AGENT_TRACE_BENCH_STRICT=1 also gates the p99 budgets (quiet fixed runner).

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

type discardSink struct{}

func (discardSink) Enqueue(*agenttrace.Event) bool { return true }

type benchMode struct {
	name    string
	on      bool
	content agenttrace.ContentProcessor
}

type benchStats struct {
	lat []time.Duration
	cpu time.Duration
}

func (s benchStats) pct(p float64) time.Duration {
	if len(s.lat) == 0 {
		return 0
	}
	v := append([]time.Duration(nil), s.lat...)
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	return v[int(p*float64(len(v)-1))]
}

func processCPU() time.Duration {
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

func benchN() int {
	if n, err := strconv.Atoi(os.Getenv("AGENT_TRACE_BENCH_N")); err == nil && n > 0 {
		return n
	}
	return 3000
}

// timedPost sends one request and returns the time to the first response
// byte and to the end of the body.
func timedPost(t *testing.T, client *http.Client, url, apiKey, body string) (ttfb, total time.Duration) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	ttfb = time.Since(start)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	return ttfb, time.Since(start)
}

func runOverhead(t *testing.T, h *failoverHarness, path, body string, useTTFB bool, modes []benchMode) map[string]*benchStats {
	client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 4}}
	url := "http://127.0.0.1:" + strconv.Itoa(h.port) + path
	set := func(m benchMode) {
		agenttrace.SetEnabled(m.on)
		agenttrace.SetContentProcessor(m.content)
	}
	for _, m := range modes { // warm-up
		set(m)
		for i := 0; i < 200; i++ {
			timedPost(t, client, url, h.apiKey, body)
		}
	}
	stats := map[string]*benchStats{}
	for _, m := range modes {
		stats[m.name] = &benchStats{}
	}
	const block = 50
	n := benchN()
	for done := 0; done < n; done += block {
		for _, m := range modes {
			set(m)
			h.proxy.waitForAnalyzers()
			s := stats[m.name]
			cpu0 := processCPU()
			for i := 0; i < block; i++ {
				ttfb, total := timedPost(t, client, url, h.apiKey, body)
				if useTTFB {
					s.lat = append(s.lat, ttfb)
				} else {
					s.lat = append(s.lat, total)
				}
			}
			h.proxy.waitForAnalyzers() // the analysis this mode caused is its cost
			s.cpu += processCPU() - cpu0
		}
	}
	return stats
}

func TestAgentTraceOverheadGate(t *testing.T) {
	agenttrace.SetSink(discardSink{})
	agenttrace.SetFingerprinter(agenttrace.StaticFingerprinter{ID: 1, Key: []byte("proxy-agent-trace-test-key-32by")})
	t.Cleanup(func() {
		agenttrace.SetEnabled(false)
		agenttrace.SetSink(nil)
		agenttrace.SetFingerprinter(nil)
		agenttrace.SetContentProcessor(nil)
	})

	pipeline := func(mode string) agenttrace.ContentProcessor {
		return agenttrace.NewContentProcessor(agenttrace.ContentGlobals{
			Ceiling: mode, DefaultMode: mode, MaxRequestOverride: mode, StoreCap: agenttrace.ContentFull,
			MaxPartBytes: 256 << 10, MaxSpanBytes: 1 << 20,
		}, func(uint, agenttrace.Resource) agenttrace.ContentOverrides { return agenttrace.ContentOverrides{} })
	}
	modes := []benchMode{{name: "off"}, {name: "on, content off", on: true, content: pipeline(agenttrace.ContentOff)}}
	if full := pipeline(agenttrace.ContentFull); full != nil {
		modes = append(modes, benchMode{name: "on, content full", on: true, content: full})
	} else {
		t.Log("content pipeline not linked (Community build): full-content mode skipped")
	}

	// 32 KiB request bodies (spec §10 scenario).
	pad := strings.Repeat("x", 32<<10)
	chat := `{"model":"gpt-4o","messages":[{"role":"user","content":"` + pad + `"}]}`
	stream := `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"` + pad + `"}]}`

	h := newFailoverHarness(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "stream") || r.Header.Get("Accept") == "text/event-stream" {
			serveStreamedToolCall(w, r)
			return
		}
		serveToolCall(w, r)
	}, serveOpenAIText("never"), nil)

	report := os.Getenv("AGENT_TRACE_BENCH_REPORT") == "1"
	strict := os.Getenv("AGENT_TRACE_BENCH_STRICT") == "1"
	check := func(ok bool, format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		if ok {
			t.Log("PASS " + msg)
		} else if report {
			t.Log("OVER " + msg)
		} else {
			t.Error("FAIL " + msg)
		}
	}
	// Tail latencies swing by milliseconds between identical runs on a
	// shared machine (measured: off-mode p99 0.8-3.5 ms), so p99 budgets
	// gate only on a quiet, fixed runner (AGENT_TRACE_BENCH_STRICT=1).
	checkTail := func(ok bool, format string, args ...interface{}) {
		if strict {
			check(ok, format, args...)
			return
		}
		state := "PASS"
		if !ok {
			state = "OVER"
		}
		t.Logf("%s (tail, gated with AGENT_TRACE_BENCH_STRICT=1) %s", state, fmt.Sprintf(format, args...))
	}

	rest := runOverhead(t, h, "/llm/rest/primary/v1/chat/completions", chat, false, modes)
	base := rest["off"]
	t.Logf("non-streaming, %d requests per mode, 32 KiB bodies", len(base.lat))
	for _, m := range modes {
		s := rest[m.name]
		t.Logf("  %-18s p50 %8v  p99 %8v  cpu %v", m.name, s.pct(0.50), s.pct(0.99), s.cpu)
	}
	for _, m := range modes[1:] {
		s := rest[m.name]
		d50, d99 := s.pct(0.50)-base.pct(0.50), s.pct(0.99)-base.pct(0.99)
		cpu := float64(s.cpu-base.cpu) / float64(base.cpu) * 100
		budget := 5.0
		if m.name == "on, content full" {
			budget = 15
		}
		check(d50 <= 200*time.Microsecond, "%s: added p50 %v (budget 0.2 ms)", m.name, d50)
		checkTail(d99 <= time.Millisecond, "%s: added p99 %v (budget 1 ms)", m.name, d99)
		// Process CPU here includes the load generator and the fake vendor
		// sharing this process, so it is reported, not gated: the CPU budget
		// is judged on gwbench, with the gateway in a process of its own.
		t.Logf("INFO %s: added process CPU %.1f%% (gateway budget %.0f%%, judged on gwbench)", m.name, cpu, budget)
	}

	st := runOverhead(t, h, "/llm/stream/primary/v1/chat/completions", stream, true, modes[:2])
	d := st["on, content off"].pct(0.99) - st["off"].pct(0.99)
	t.Logf("streaming time to first byte: off p50 %v p99 %v, on p50 %v p99 %v",
		st["off"].pct(0.50), st["off"].pct(0.99), st["on, content off"].pct(0.50), st["on, content off"].pct(0.99))
	checkTail(d <= 500*time.Microsecond, "streaming: added TTFB p99 %v (budget 0.5 ms)", d)
	d50 := st["on, content off"].pct(0.50) - st["off"].pct(0.50)
	check(d50 <= 200*time.Microsecond, "streaming: added TTFB p50 %v (budget 0.2 ms)", d50)
}

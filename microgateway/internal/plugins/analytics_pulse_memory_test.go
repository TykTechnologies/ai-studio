package plugins

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/metrics"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/overload"
	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// Memory of an edge through a control-plane outage and the backlog's drain,
// at a production request rate, in real time (bead midsommar-clean-4x5y.49).
// A measurement, not a unit test: it runs only with PULSE_MEMORY_MEASURE=1
// and takes the outage plus the drain (about 6 minutes with the defaults).
//
//	PULSE_MEMORY_MEASURE=1 PULSE_BACKLOG=67108864 PULSE_LIMIT=536870912 \
//	  go test ./internal/plugins -run TestPulseMemoryThroughOutage -v -timeout 30m
//
// Settings (environment): PULSE_RATE requests/s (3000), PULSE_OUTAGE (3m30s),
// PULSE_BACKLOG max_backlog_bytes (64 MiB), PULSE_LIMIT the container memory
// limit in bytes given to the gateway's runtime tuning (0: none, as on the
// load-test rig), PULSE_HUB_LATENCY per delivered pulse (300ms: the 92 s
// drain of e4), PULSE_CSV a file for the samples.

// measureHub fails every pulse while down; up, it takes latency per pulse
// and keeps only counts.
type measureHub struct {
	pb.ConfigurationSyncServiceClient
	down    atomic.Bool
	latency time.Duration
	records atomic.Uint64
	pulses  atomic.Uint64
}

func (h *measureHub) SendAnalyticsPulse(ctx context.Context, pulse *pb.AnalyticsPulse, _ ...grpc.CallOption) (*pb.AnalyticsPulseResponse, error) {
	if h.down.Load() {
		return nil, errors.New("control plane unreachable")
	}
	// The gRPC client serializes the message on the edge.
	if _, err := proto.Marshal(pulse); err != nil {
		return nil, err
	}
	select {
	case <-time.After(h.latency):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	h.records.Add(uint64(pulse.TotalRecords))
	h.pulses.Add(1)
	return &pb.AnalyticsPulseResponse{Success: true, ProcessedRecords: uint64(pulse.TotalRecords)}, nil
}

func envOr(t *testing.T, name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func envInt(t *testing.T, name string, def int64) int64 {
	v, err := strconv.ParseInt(envOr(t, name, strconv.FormatInt(def, 10)), 10, 64)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return v
}

func envDur(t *testing.T, name string, def time.Duration) time.Duration {
	v, err := time.ParseDuration(envOr(t, name, def.String()))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return v
}

func processRSS() uint64 {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if strings.HasPrefix(s.Text(), "VmRSS:") {
			kb, _ := strconv.ParseUint(strings.Fields(s.Text())[1], 10, 64)
			return kb << 10
		}
	}
	return 0
}

// One request as the gateway reports it: an analytics event, a budget
// record and a proxy summary, with the values a proxied chat call carries.
func recordRequest(p *AnalyticsPulsePlugin, i uint64) {
	now := time.Now()
	id := fmt.Sprintf("req-%016x", i)
	ctx := context.Background()
	_, _ = p.HandleAnalytics(ctx, &interfaces.AnalyticsData{
		RequestID: id, AppID: 7, LLMID: 3, UserID: 12, ModelName: "gpt-4o-mini", Vendor: "openai",
		InteractionType: "proxy", Choices: 1, Currency: "USD", StatusCode: 200,
		PromptTokens: 42, ResponseTokens: 18, TotalTokens: 60, Cost: 0.000123, TotalTimeMS: 35,
		Timestamp: now, GatewayRequestID: id,
	}, nil)
	_, _ = p.HandleBudgetUsage(ctx, &interfaces.BudgetUsageData{
		RequestID: id, AppID: 7, LLMID: 3, TokensUsed: 60, Cost: 0.000123, PromptTokens: 42,
		CompletionTokens: 18, RequestsCount: 1, Timestamp: now, PeriodStart: now.Add(-time.Hour), PeriodEnd: now.Add(time.Hour),
	}, nil)
	_, _ = p.HandleProxyLog(ctx, &interfaces.ProxyLogData{
		RequestID: id, AppID: 7, UserID: 12, Vendor: "openai", ResponseCode: 200, Timestamp: now,
	}, nil)
}

func TestPulseMemoryThroughOutage(t *testing.T) {
	if os.Getenv("PULSE_MEMORY_MEASURE") != "1" {
		t.Skip("a real-time measurement: set PULSE_MEMORY_MEASURE=1")
	}
	rate := envInt(t, "PULSE_RATE", 3000)
	outage := envDur(t, "PULSE_OUTAGE", 210*time.Second)
	backlog := envInt(t, "PULSE_BACKLOG", 64<<20)
	limit := envInt(t, "PULSE_LIMIT", 0)
	hub := &measureHub{latency: envDur(t, "PULSE_HUB_LATENCY", 300*time.Millisecond)}

	// The gateway's own garbage-collector tuning, as main applies it.
	tuning := overload.TuneRuntime(uint64(limit))
	t.Logf("runtime tuning: gogc %d adaptive %v, memory limit %d, soft limit %d",
		tuning.GCPercent, tuning.GCPercentAdaptive, tuning.MemoryLimit, tuning.SoftLimit)

	// The packaged pulse settings (Helm chart values).
	cfg := PulsePluginConfig{
		IntervalSeconds: 10, MaxBufferSize: 10000, MaxBacklogBytes: backlog,
		CompressionEnabled: true, IncludeProxySummaries: true, IncludeRequestResponseData: true,
		EdgeRetentionHours: 24, TimeoutSeconds: 30, MaxRetries: 3, RetryIntervalSecs: 5,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &AnalyticsPulsePlugin{
		config: &cfg, edgeID: "edge-measure", edgeNamespace: "test",
		clients: staticClient{hub}, sequenceNumber: 1, lastPulseTime: time.Now(), ctx: ctx,
	}
	if err := p.Initialize(nil); err != nil {
		t.Fatal(err)
	}
	defer p.stopTimer()

	var produced atomic.Uint64
	stop := make(chan struct{})
	var load sync.WaitGroup
	const writers = 8
	for w := 0; w < writers; w++ {
		load.Add(1)
		go func(w int) {
			defer load.Done()
			per := time.Duration(int64(time.Second) * writers / rate)
			next := time.Now()
			for i := uint64(w); ; i += writers {
				select {
				case <-stop:
					return
				default:
				}
				recordRequest(p, i)
				produced.Add(1)
				next = next.Add(per)
				if d := time.Until(next); d > 0 {
					time.Sleep(d)
				}
			}
		}(w)
	}

	var csv *os.File
	if path := os.Getenv("PULSE_CSV"); path != "" {
		var err error
		if csv, err = os.Create(path); err != nil {
			t.Fatal(err)
		}
		defer csv.Close()
		fmt.Fprintln(csv, "t_s,phase,rss_mb,heap_alloc_mb,heap_live_mb,heap_goal_mb,go_mem_mb,backlog_mb,backlog_batches,buffered_records,gogc")
	}
	samples := []metrics.Sample{{Name: "/gc/heap/live:bytes"}, {Name: "/gc/heap/goal:bytes"}, {Name: "/memory/classes/heap/stacks:bytes"}}
	type peak struct{ rss, heap, live, goMem, backlog, buffered uint64 }
	peaks := map[string]*peak{}
	var shedSamples, allSamples int
	phase := atomic.Value{}
	phase.Store("before")
	started := time.Now()
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
			}
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			metrics.Read(samples)
			live, goal, stacks := samples[0].Value.Uint64(), samples[1].Value.Uint64(), samples[2].Value.Uint64()
			goMem := goal + stacks // what overload shedding judges against
			rss := processRSS()
			bb, nb := PulseBacklog()
			p.bufferMutex.Lock()
			buffered := uint64(p.totalBufferedLocked())
			p.bufferMutex.Unlock()
			ph := phase.Load().(string)
			pk := peaks[ph]
			if pk == nil {
				pk = &peak{}
				peaks[ph] = pk
			}
			for _, x := range []struct {
				v uint64
				p *uint64
			}{{rss, &pk.rss}, {m.HeapAlloc, &pk.heap}, {live, &pk.live}, {goMem, &pk.goMem}, {uint64(bb), &pk.backlog}, {buffered, &pk.buffered}} {
				if x.v > *x.p {
					*x.p = x.v
				}
			}
			allSamples++
			if limit > 0 && float64(goMem) >= 0.85*float64(limit) {
				shedSamples++
			}
			if csv != nil {
				mb := func(v uint64) string { return fmt.Sprintf("%.1f", float64(v)/(1<<20)) }
				fmt.Fprintf(csv, "%.1f,%s,%s,%s,%s,%s,%s,%s,%d,%d,%d\n", time.Since(started).Seconds(), ph,
					mb(rss), mb(m.HeapAlloc), mb(live), mb(goal), mb(goMem), mb(uint64(bb)), nb, buffered, overload.CurrentGCPercent())
			}
		}
	}()

	time.Sleep(30 * time.Second)
	phase.Store("outage")
	hub.down.Store(true)
	time.Sleep(outage)
	phase.Store("drain")
	hub.down.Store(false)
	drainStart := time.Now()
	for {
		time.Sleep(500 * time.Millisecond)
		bb, _ := PulseBacklog()
		if bb == 0 || time.Since(drainStart) > 10*time.Minute {
			break
		}
	}
	drained := time.Since(drainStart)
	phase.Store("after")
	time.Sleep(60 * time.Second)
	close(stop)
	load.Wait()
	<-sampled
	// Send what is left, all of it: the timer would, every interval.
	for i := 0; i < 100; i++ {
		for p.sending.Load() || p.spillQueued.Load() || p.flushQueued.Load() {
			time.Sleep(10 * time.Millisecond)
		}
		p.bufferMutex.Lock()
		left := p.totalBufferedLocked() + len(p.backlog)
		p.bufferMutex.Unlock()
		if left == 0 {
			break
		}
		p.sendPulse()
	}

	dropped := PulseDroppedRecords()
	unaccounted := int64(3*produced.Load()) - int64(hub.records.Load()) - int64(dropped["over_backlog"]+dropped["over_cap"]+dropped["expired"]+dropped["oversized"])
	t.Logf("records neither delivered nor counted as dropped: %d", unaccounted)
	t.Logf("rate %d/s, outage %s, max_backlog_bytes %d MiB, limit %d MiB, hub latency %s: backlog drained in %s",
		rate, outage, backlog>>20, limit>>20, hub.latency, drained.Round(time.Second))
	t.Logf("requests %d (records %d); delivered records %d in %d pulses; dropped %v",
		produced.Load(), 3*produced.Load(), hub.records.Load(), hub.pulses.Load(), dropped)
	for _, ph := range []string{"before", "outage", "drain", "after"} {
		if pk := peaks[ph]; pk != nil {
			t.Logf("%-6s peak: rss %.0f MB, heap %.0f MB, live %.0f MB, go memory (shedding measure) %.0f MB, backlog %.0f MB, buffered records %d",
				ph, float64(pk.rss)/(1<<20), float64(pk.heap)/(1<<20), float64(pk.live)/(1<<20), float64(pk.goMem)/(1<<20),
				float64(pk.backlog)/(1<<20), pk.buffered)
		}
	}
	if limit > 0 {
		t.Logf("samples at or above the shedding threshold (85%% of the limit): %d of %d", shedSamples, allSamples)
	}

	// Acceptance (bead .49): the analytics hold at most the backlog plus
	// about two batches in memory, through the outage and the drain;
	// nothing is lost without being counted; and under a memory limit the
	// analytics never push the edge into shedding live traffic.
	const allowance = 40 << 20
	for _, ph := range []string{"outage", "drain"} {
		pk := peaks[ph]
		if pk == nil {
			t.Fatalf("no samples in %s", ph)
		}
		if pk.live > pk.backlog+allowance {
			t.Errorf("%s: live heap %d MB above the backlog (%d MB) plus %d MB", ph, pk.live>>20, pk.backlog>>20, allowance>>20)
		}
	}
	if unaccounted != 0 {
		t.Errorf("%d records neither delivered nor counted as dropped", unaccounted)
	}
	if limit > 0 && shedSamples > 0 {
		t.Errorf("memory at the shedding threshold in %d samples", shedSamples)
	}
}

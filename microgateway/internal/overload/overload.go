// Package overload refuses new proxy requests while the gateway is short of
// memory or at its in-flight cap, so an overload is answered with fast 503s
// instead of the process being killed for running out of memory.
//
// Past its CPU capacity a gateway takes requests faster than it finishes
// them, and every waiting request holds goroutines and buffers. Without a
// limit the backlog grows until the kernel kills the process, which drops
// every request in flight. Measured on a 2 GB container: ~48k goroutines and
// an out-of-memory kill.
//
// Memory is judged from the Go runtime's heap goal plus goroutine stacks,
// which is about the peak the process is heading for. Garbage the collector
// has already found is left out, so shedding stops once the backlog clears
// rather than when the runtime hands memory back to the OS.
package overload

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"runtime/debug"
	"runtime/metrics"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// Reason says why a request was refused.
type Reason int

const (
	// Admitted means the request may proceed.
	Admitted Reason = iota
	// Memory means the process is above its memory threshold.
	Memory
	// Inflight means MaxInflight requests are already in progress.
	Inflight
)

// Config configures a Manager.
type Config struct {
	// Enabled turns shedding on.
	Enabled bool
	// MemoryLimit in bytes; 0 detects it (GOMEMLIMIT, then the cgroup limit).
	MemoryLimit uint64
	// Threshold is the fraction of MemoryLimit above which new requests are
	// refused; they are admitted again below Threshold - hysteresis.
	Threshold float64
	// MaxInflight caps concurrent proxy requests; 0 means no cap.
	MaxInflight int64
	// SampleInterval is how often memory is sampled.
	SampleInterval time.Duration
	// Exempt reports requests that are never refused or counted: the
	// gateway's own loopback hop (/ai/ to /llm/call/), whose outer request
	// was already admitted. Nil exempts nothing.
	Exempt func(*http.Request) bool
}

// hysteresis is how far below the threshold memory must fall before new
// requests are admitted again, so shedding does not flap around the line.
const hysteresis = 0.05

// Manager tracks the gateway's load and decides whether to admit a request.
type Manager struct {
	cfg   Config
	limit uint64 // bytes; 0 disables memory shedding

	inflight atomic.Int64
	shedding atomic.Bool
	memory   atomic.Uint64 // last sample, bytes

	rejectedMemory   atomic.Uint64
	rejectedInflight atomic.Uint64

	sample func() uint64
}

// New returns a Manager. Call Start to begin sampling memory.
func New(cfg Config) *Manager {
	if cfg.Threshold <= 0 || cfg.Threshold > 1 {
		cfg.Threshold = 0.85
	}
	if cfg.SampleInterval <= 0 {
		cfg.SampleInterval = 100 * time.Millisecond
	}
	m := &Manager{cfg: cfg, sample: sampleGoMemory}
	if cfg.Enabled {
		m.limit = cfg.MemoryLimit
		if m.limit == 0 {
			m.limit = detectMemoryLimit()
		}
	}
	return m
}

// Start samples memory every SampleInterval until ctx is done. It does
// nothing when shedding is disabled or no memory limit is known.
func (m *Manager) Start(ctx context.Context) {
	if !m.cfg.Enabled {
		log.Info().Msg("Overload shedding disabled")
		return
	}
	if m.limit == 0 {
		log.Warn().Msg("Overload shedding: no memory limit found (set OVERLOAD_MEMORY_LIMIT, GOMEMLIMIT or a container limit); only MAX_INFLIGHT_REQUESTS applies")
		return
	}
	log.Info().
		Uint64("memory_limit_bytes", m.limit).
		Float64("threshold", m.cfg.Threshold).
		Int64("max_inflight", m.cfg.MaxInflight).
		Msg("Overload shedding enabled")
	go func() {
		t := time.NewTicker(m.cfg.SampleInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.update(m.sample())
			}
		}
	}()
}

// update records a memory sample and switches shedding on above the threshold
// and off below threshold - hysteresis.
func (m *Manager) update(used uint64) {
	m.memory.Store(used)
	if m.limit == 0 {
		return
	}
	frac := float64(used) / float64(m.limit)
	switch {
	case frac >= m.cfg.Threshold:
		if !m.shedding.Swap(true) {
			log.Warn().Uint64("memory_bytes", used).Uint64("limit_bytes", m.limit).Msg("Overload: memory above threshold, refusing new proxy requests")
		}
	case frac < m.cfg.Threshold-hysteresis:
		if m.shedding.Swap(false) {
			log.Info().Uint64("memory_bytes", used).Uint64("limit_bytes", m.limit).Msg("Overload: memory back below threshold, admitting proxy requests")
		}
	}
}

// Acquire admits a request or says why not. An admitted request must call
// Release when it finishes.
func (m *Manager) Acquire() Reason {
	if !m.cfg.Enabled {
		return Admitted
	}
	if m.shedding.Load() {
		m.rejectedMemory.Add(1)
		return Memory
	}
	n := m.inflight.Add(1)
	if m.cfg.MaxInflight > 0 && n > m.cfg.MaxInflight {
		m.inflight.Add(-1)
		m.rejectedInflight.Add(1)
		return Inflight
	}
	return Admitted
}

// Release ends an admitted request.
func (m *Manager) Release() {
	if m.cfg.Enabled {
		m.inflight.Add(-1)
	}
}

// Stats is a snapshot of the Manager's state.
type Stats struct {
	Inflight                         int64
	Shedding                         bool
	MemoryBytes, MemoryLimitBytes    uint64
	RejectedMemory, RejectedInflight uint64
}

// Stats returns the Manager's current state.
func (m *Manager) Stats() Stats {
	return Stats{
		Inflight:         m.inflight.Load(),
		Shedding:         m.shedding.Load(),
		MemoryBytes:      m.memory.Load(),
		MemoryLimitBytes: m.limit,
		RejectedMemory:   m.rejectedMemory.Load(),
		RejectedInflight: m.rejectedInflight.Load(),
	}
}

// Admission is Admit's decision.
type Admission int

const (
	// Refused: the response has been written; stop the chain.
	Refused Admission = iota
	// Counted: proceed, then call Release.
	Counted
	// Exempted: proceed; nothing to release.
	Exempted
)

// Admit decides on a proxy request. A refused request is answered here: 503
// with Retry-After and an OpenAI-shaped error, which the vendor SDKs retry.
// An admitted request must call Release when it finishes; an exempt one (the
// gateway's own loopback hop) must not.
func (m *Manager) Admit(w http.ResponseWriter, r *http.Request) Admission {
	if !m.cfg.Enabled || (m.cfg.Exempt != nil && m.cfg.Exempt(r)) {
		return Exempted
	}
	switch m.Acquire() {
	case Memory:
		refuse(w, "The gateway is overloaded (memory); retry shortly.")
		return Refused
	case Inflight:
		refuse(w, "The gateway is at its concurrent request limit; retry shortly.")
		return Refused
	}
	return Counted
}

// Middleware wraps a proxy handler with Admit.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch m.Admit(w, r) {
		case Refused:
			return
		case Counted:
			defer m.Release()
		}
		next.ServeHTTP(w, r)
	})
}

func refuse(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "1")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"message": message,
			"type":    "server_error",
			"code":    "overloaded",
		},
	})
}

var goMemorySamples = []metrics.Sample{
	{Name: "/gc/heap/goal:bytes"},
	{Name: "/memory/classes/heap/stacks:bytes"},
}

// sampleGoMemory returns the heap goal plus goroutine stacks: about the peak
// the Go heap is heading for before the next collection, plus what the
// goroutines themselves hold.
func sampleGoMemory() uint64 {
	s := make([]metrics.Sample, len(goMemorySamples))
	copy(s, goMemorySamples)
	metrics.Read(s)
	var total uint64
	for _, v := range s {
		if v.Value.Kind() == metrics.KindUint64 {
			total += v.Value.Uint64()
		}
	}
	return total
}

// detectMemoryLimit returns GOMEMLIMIT when set, else the cgroup memory
// limit, else 0.
func detectMemoryLimit() uint64 {
	if l := debug.SetMemoryLimit(-1); l > 0 && l < math.MaxInt64 {
		return uint64(l)
	}
	return cgroupMemoryLimit("/sys/fs/cgroup")
}

// cgroupMemoryLimit reads the memory limit of the cgroup mounted at root
// (v2 memory.max, then v1 memory/memory.limit_in_bytes); 0 when there is none.
func cgroupMemoryLimit(root string) uint64 {
	for _, p := range []string{root + "/memory.max", root + "/memory/memory.limit_in_bytes"} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		v := strings.TrimSpace(string(b))
		if v == "" || v == "max" {
			return 0
		}
		n, err := strconv.ParseUint(v, 10, 64)
		// cgroup v1 reports "no limit" as a huge page-aligned number.
		if err != nil || n >= 1<<62 {
			return 0
		}
		return n
	}
	return 0
}

// ParseBytes parses a size such as "1536MiB", "2GiB", "512M" or "1073741824".
func ParseBytes(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	units := []struct {
		suffix string
		mult   uint64
	}{
		{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10},
		{"GB", 1e9}, {"MB", 1e6}, {"KB", 1e3},
		{"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10},
		{"B", 1},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), 64)
			if err != nil || n < 0 {
				return 0, strconv.ErrSyntax
			}
			return uint64(n * float64(u.mult)), nil
		}
	}
	return strconv.ParseUint(s, 10, 64)
}

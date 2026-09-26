package server

import (
	"context"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/overload"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/rs/zerolog/log"
)

// newOverloadManager builds and starts the overload manager from the gateway
// configuration, and exports its state when metrics are enabled. The returned
// function stops its memory sampling.
func newOverloadManager(cfg *config.Config) (*overload.Manager, context.CancelFunc) {
	limit, err := overload.ParseBytes(cfg.Gateway.OverloadMemoryLimit)
	if err != nil {
		log.Warn().Err(err).Str("value", cfg.Gateway.OverloadMemoryLimit).Msg("Invalid OVERLOAD_MEMORY_LIMIT; detecting the limit instead")
		limit = 0
	}
	m := overload.New(overload.Config{
		Enabled:     cfg.Gateway.OverloadSheddingEnabled,
		MemoryLimit: limit,
		Threshold:   cfg.Gateway.OverloadMemoryThreshold,
		MaxInflight: cfg.Gateway.MaxInflightRequests,
		// The gateway's own /ai/ loopback hop: its outer request was
		// already admitted.
		Exempt: proxy.IsInternalHop,
	})
	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)

	if cfg.Observability.EnableMetrics {
		for _, g := range []struct {
			name, help string
			counter    bool
			value      func(overload.Stats) float64
		}{
			{"microgateway_overload_inflight_requests", "Proxy requests in progress", false,
				func(s overload.Stats) float64 { return float64(s.Inflight) }},
			{"microgateway_overload_shedding", "1 while new proxy requests are refused for memory", false,
				func(s overload.Stats) float64 {
					if s.Shedding {
						return 1
					}
					return 0
				}},
			{"microgateway_overload_memory_bytes", "Go memory judged for shedding (heap goal + stacks)", false,
				func(s overload.Stats) float64 { return float64(s.MemoryBytes) }},
			{"microgateway_overload_memory_limit_bytes", "Memory limit shedding is judged against (0: none)", false,
				func(s overload.Stats) float64 { return float64(s.MemoryLimitBytes) }},
			{"microgateway_gogc", "GOGC in effect (adapted to the live heap unless GOGC is set)", false,
				func(overload.Stats) float64 { return float64(overload.CurrentGCPercent()) }},
			{"microgateway_overload_rejected_memory_total", "Proxy requests refused because memory was above the threshold", true,
				func(s overload.Stats) float64 { return float64(s.RejectedMemory) }},
			{"microgateway_overload_rejected_inflight_total", "Proxy requests refused at MAX_INFLIGHT_REQUESTS", true,
				func(s overload.Stats) float64 { return float64(s.RejectedInflight) }},
		} {
			value := g.value
			fn := func() float64 { return value(m.Stats()) }
			var err error
			if g.counter {
				err = metrics.RegisterCounterFunc(g.name, g.help, fn)
			} else {
				err = metrics.RegisterGaugeFunc(g.name, g.help, fn)
			}
			if err != nil {
				log.Warn().Err(err).Str("metric", g.name).Msg("Failed to register overload metric")
			}
		}
	}
	return m, cancel
}

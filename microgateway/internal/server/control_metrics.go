package server

import (
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/plugins"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/rs/zerolog/log"
)

// registerControlLinkMetric exports, on an edge, which control endpoint it
// is connected to: microgateway_control_connected{endpoint} is 1 for the
// endpoint whose stream is open and 0 for the others, so all zeros means
// the edge has no control plane.
func registerControlLinkMetric(cfg *config.Config, sc *services.ServiceContainer) {
	if !cfg.IsEdge() || sc.ControlLink == nil {
		return
	}
	endpoints := cfg.HubSpoke.Endpoints()
	err := metrics.RegisterLabelledGaugeFunc(
		"microgateway_control_connected",
		"Whether this edge's stream to a control endpoint is open (1) or not (0), per configured endpoint",
		"endpoint",
		func() map[string]float64 { return controlLinkValues(endpoints, sc.ControlLink) },
	)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to register control connection metric")
	}
}

// registerConfigSourceMetrics exports, on an edge, where its configuration
// came from and how long it has been without a control plane:
//
//   - microgateway_config_source{source} is 1 for the source in use
//     ("control", or "last_known_good" for an edge that started from its
//     stored snapshot and has not reconnected yet) and 0 for the other;
//   - microgateway_config_age_seconds is the time since the edge last had
//     contact with a control plane (a registration or a heartbeat
//     acknowledged): a few seconds while connected, growing during an
//     outage, and what EDGE_LAST_KNOWN_GOOD_MAX_AGE is compared with at
//     start-up. 0 when the edge has never had one.
func registerConfigSourceMetrics(cfg *config.Config, sc *services.ServiceContainer) {
	if !cfg.IsEdge() || sc.ConfigSource == nil {
		return
	}
	if err := metrics.RegisterLabelledGaugeFunc(
		"microgateway_config_source",
		"Where this edge's configuration came from: 1 for the source in use (control, last_known_good), 0 for the other",
		"source",
		func() map[string]float64 { return configSourceValues(sc.ConfigSource) },
	); err != nil {
		log.Warn().Err(err).Msg("Failed to register configuration source metric")
	}
	if err := metrics.RegisterGaugeFunc(
		"microgateway_config_age_seconds",
		"Seconds since this edge last had contact with a control plane (0 if it never had)",
		func() float64 { return configAgeSeconds(sc.ConfigSource, time.Now()) },
	); err != nil {
		log.Warn().Err(err).Msg("Failed to register configuration age metric")
	}
}

func configSourceValues(source func() (string, time.Time)) map[string]float64 {
	current, _ := source()
	values := map[string]float64{"control": 0, "last_known_good": 0}
	values[current] = 1
	return values
}

func configAgeSeconds(source func() (string, time.Time), now time.Time) float64 {
	_, contact := source()
	if contact.IsZero() || now.Before(contact) {
		return 0
	}
	return now.Sub(contact).Seconds()
}

func controlLinkValues(endpoints []string, link func() (string, bool)) map[string]float64 {
	current, connected := link()
	values := make(map[string]float64, len(endpoints))
	for _, e := range endpoints {
		values[e] = 0
		if connected && e == current {
			values[e] = 1
		}
	}
	return values
}

// registerPulseDropMetric exports, on an edge, the analytics records its
// pulse gave up: microgateway_analytics_pulse_dropped_records_total{reason}.
// The control plane's analytics, and the spend it computes from them, are
// short by these.
//
//   - reason="over_cap": discarded, oldest first, to keep what the edge
//     holds for the control plane within max_buffer_size while pulses fail
//     (no control plane reachable, or one that answers but cannot store).
//     This is the number to look at after an outage.
//   - reason="expired": held for longer than edge_retention_hours.
//   - reason="oversized": one record larger than a pulse may be.
func registerPulseDropMetric(cfg *config.Config) {
	if !cfg.IsEdge() {
		return
	}
	if err := metrics.RegisterLabelledCounterFunc(
		"microgateway_analytics_pulse_dropped_records_total",
		"Analytics records this edge's pulse gave up, by reason (over_cap: beyond max_buffer_size while pulses failed; expired: older than edge_retention_hours; oversized: one record too large for a pulse). The control plane's analytics are short by these.",
		"reason",
		func() map[string]float64 { return pulseDropValues(plugins.PulseDroppedRecords()) },
	); err != nil {
		log.Warn().Err(err).Msg("Failed to register analytics pulse drop metric")
	}
}

func pulseDropValues(counts map[string]uint64) map[string]float64 {
	values := make(map[string]float64, len(counts))
	for reason, n := range counts {
		values[reason] = float64(n)
	}
	return values
}

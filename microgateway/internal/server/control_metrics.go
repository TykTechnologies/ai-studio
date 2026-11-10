package server

import (
	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
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

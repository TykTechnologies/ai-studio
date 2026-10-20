package main

import (
	"strings"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/plugins"
	"github.com/TykTechnologies/midsommar/v2/analytics"
	aiconfig "github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace/declared"
	"github.com/TykTechnologies/midsommar/v2/pkg/tracing"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/rs/zerolog/log"
)

// captureEnabler is the slice of the plugin manager the gate needs.
type captureEnabler interface {
	EnableAgentTracingCapture(plugins.CaptureSettings) bool
}

// startAgentTracing is the edge's agent tracing start gate. Capture runs
// only on an Enterprise build with a valid licence, AGENT_TRACING_ENABLED,
// in edge mode (the events go to the hub) and with the analytics pulse
// loaded to carry them. Only then does it turn the capture flag on; in every
// other case the gateway stays inert. It reports whether capture started.
func startAgentTracing(cfg *config.Config, pm captureEnabler, licenseValid func() bool) bool {
	agenttrace.SetEnabled(false)
	at := cfg.AgentTracing
	// The baggage allowlist applies in every edition, tracing on or off.
	allow := at.BaggageAllowlist
	if len(allow) == 1 && strings.EqualFold(strings.TrimSpace(allow[0]), "none") {
		allow = nil
	}
	tracing.SetBaggageAllowlist(allow)
	declared.SetDefault(declared.New(declared.Options{
		CompatFamilies:   at.CompatHeaders,
		Harness:          at.HarnessSessionHeaders,
		BaggageAllowlist: allow,
	}))
	tracing.SetOutboundPropagation(at.PropagateLLMProviders, at.PropagateHTTPUpstreams, at.PropagateMCP)
	proxy.SetOpenAIClientRequestID(at.OpenAIClientRequestID)
	proxy.SetAgentTraceStreamBufferBytes(at.MaxStreamBufferBytes)
	if !at.Enabled {
		return false
	}
	switch {
	case !aiconfig.IsEnterprise():
		log.Warn().Msg("AGENT_TRACING_ENABLED is set, but Agent Call Tracing is an Enterprise feature; it stays off in this build")
		return false
	case licenseValid == nil || !licenseValid():
		log.Warn().Msg("AGENT_TRACING_ENABLED is set, but the Enterprise licence is not valid; Agent Call Tracing stays off")
		return false
	case !cfg.IsEdge():
		log.Warn().Msg("AGENT_TRACING_ENABLED is set, but capture on a microgateway needs a hub (edge mode); Agent Call Tracing stays off")
		return false
	case pm == nil || !pm.EnableAgentTracingCapture(plugins.CaptureSettings{
		MaxEvents:      at.QueueMaxEvents,
		MaxBytes:       at.QueueMaxBytes,
		FlushInterval:  at.FlushInterval,
		FlushMaxEvents: at.FlushMaxEvents,
		ChunkMaxBytes:  at.ChunkMaxBytes,
	}):
		log.Warn().Msg("AGENT_TRACING_ENABLED is set, but the analytics pulse is not loaded to carry capture events; Agent Call Tracing stays off")
		return false
	}
	agenttrace.SetSink(analytics.CaptureSink())
	agenttrace.SetEnabled(true)
	log.Info().Msg("Agent Call Tracing capture started on this edge")
	return true
}

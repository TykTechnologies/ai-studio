package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/plugins"
	aiconfig "github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace/declared"
	"github.com/TykTechnologies/midsommar/v2/pkg/tracing"
	"github.com/TykTechnologies/midsommar/v2/proxy"
)

type fakeEnabler struct {
	ok       bool
	settings *plugins.CaptureSettings
}

func (f *fakeEnabler) EnableAgentTracingCapture(s plugins.CaptureSettings) bool {
	f.settings = &s
	return f.ok
}

func gateConfig(enabled bool, mode string) *config.Config {
	cfg := &config.Config{}
	cfg.HubSpoke.Mode = mode
	cfg.AgentTracing = config.AgentTracingConfig{Enabled: enabled, QueueMaxEvents: 7, QueueMaxBytes: 1 << 20,
		FlushInterval: 1, FlushMaxEvents: 3, ChunkMaxBytes: 1024}
	return cfg
}

func TestStartAgentTracingGate(t *testing.T) {
	t.Cleanup(func() { agenttrace.SetEnabled(false); agenttrace.SetSink(nil); resetEdgeTracingGlobals() })
	yes := func() bool { return true }
	no := func() bool { return false }

	cases := []struct {
		name     string
		cfg      *config.Config
		enabler  *fakeEnabler
		licensed func() bool
		want     bool
	}{
		{"flag off", gateConfig(false, "edge"), &fakeEnabler{ok: true}, yes, false},
		{"no licence", gateConfig(true, "edge"), &fakeEnabler{ok: true}, no, false},
		{"standalone", gateConfig(true, "standalone"), &fakeEnabler{ok: true}, yes, false},
		{"no pulse plugin", gateConfig(true, "edge"), &fakeEnabler{ok: false}, yes, false},
		{"enterprise edge", gateConfig(true, "edge"), &fakeEnabler{ok: true}, yes, aiconfig.IsEnterprise()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agenttrace.SetEnabled(true) // a stale flag must be cleared
			got := startAgentTracing(tc.cfg, tc.enabler, tc.licensed)
			if got != tc.want || agenttrace.Enabled() != tc.want {
				t.Fatalf("started=%v flag=%v, want %v", got, agenttrace.Enabled(), tc.want)
			}
			_, rec := agenttrace.Begin(context.Background())
			if (rec != nil) != tc.want {
				t.Fatalf("recorder created=%v, want %v", rec != nil, tc.want)
			}
			if tc.want && (tc.enabler.settings == nil || tc.enabler.settings.MaxEvents != 7 || tc.enabler.settings.FlushMaxEvents != 3) {
				t.Fatalf("settings not passed: %+v", tc.enabler.settings)
			}
		})
	}
}

// The start gate installs the declared-context extractor from the edge's
// configuration, whatever the gate decides (jyx.113).
func TestStartAgentTracingInstallsDeclaredExtractor(t *testing.T) {
	t.Cleanup(resetEdgeTracingGlobals)
	h := http.Header{"Helicone-Session-Id": {"h-1"}, "X-My-Session": {"m-1"}}

	cfg := gateConfig(false, "edge")
	cfg.AgentTracing.CompatHeaders = []string{"helicone"}
	startAgentTracing(cfg, &fakeEnabler{}, nil)
	if d := declared.Default().Headers(h, "", nil).Declared; d.SessionID != "h-1" || d.SessionSource != "helicone" {
		t.Fatalf("compat family not enabled: %+v", d)
	}

	cfg.AgentTracing.CompatHeaders = nil
	cfg.AgentTracing.HarnessSessionHeaders = []string{"x-my-session"}
	startAgentTracing(cfg, &fakeEnabler{}, nil)
	if d := declared.Default().Headers(h, "", nil).Declared; d.SessionID != "m-1" {
		t.Fatalf("configured session header not read: %+v", d)
	}
}

// Content crosses to the hub only over verified TLS: with TLS off or the
// certificate check skipped, the pulse is told to send metadata only
// (jyx.127).
func TestStartAgentTracingMetadataOnlyWithoutVerifiedTLS(t *testing.T) {
	if !aiconfig.IsEnterprise() {
		t.Skip("capture starts only in the Enterprise build")
	}
	t.Cleanup(func() { agenttrace.SetEnabled(false); agenttrace.SetSink(nil); resetEdgeTracingGlobals() })
	yes := func() bool { return true }
	for _, tc := range []struct {
		name         string
		tls, skip    bool
		metadataOnly bool
	}{
		{"verified TLS", true, false, false},
		{"TLS off", false, false, true},
		{"verification skipped", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := gateConfig(true, "edge")
			cfg.HubSpoke.ClientTLSEnabled, cfg.HubSpoke.SkipTLSVerify = tc.tls, tc.skip
			en := &fakeEnabler{ok: true}
			if !startAgentTracing(cfg, en, yes) {
				t.Fatal("capture did not start")
			}
			if en.settings.MetadataOnly != tc.metadataOnly {
				t.Fatalf("MetadataOnly=%v, want %v", en.settings.MetadataOnly, tc.metadataOnly)
			}
		})
	}
}

// resetEdgeTracingGlobals restores what startAgentTracing sets process-wide.
func resetEdgeTracingGlobals() {
	declared.SetDefault(nil)
	tracing.SetOutboundPropagation(true, true, true)
	proxy.SetOpenAIClientRequestID(true)
	proxy.SetAgentTraceStreamBufferBytes(0)
}

// The start gate applies the edge's outbound propagation settings in every
// edition, before deciding on capture (jyx.110).
func TestStartAgentTracingAppliesPropagation(t *testing.T) {
	t.Cleanup(resetEdgeTracingGlobals)
	cfg := gateConfig(false, "")
	cfg.AgentTracing.PropagateLLMProviders = false
	cfg.AgentTracing.PropagateHTTPUpstreams = true
	cfg.AgentTracing.PropagateMCP = false
	startAgentTracing(cfg, nil, nil)
	if tracing.PropagateToLLMProviders() || !tracing.PropagateToHTTPUpstreams() || tracing.PropagateToMCP() {
		t.Fatal("propagation settings not applied")
	}
}

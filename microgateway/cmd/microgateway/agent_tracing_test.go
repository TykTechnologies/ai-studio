package main

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/plugins"
	aiconfig "github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
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
	t.Cleanup(func() { agenttrace.SetEnabled(false); agenttrace.SetSink(nil) })
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

package main

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// AGENT_TRACING_KEEP_LIVE_SPAN is applied at start-up (jyx.176), and off
// by default.
func TestStartAgentTracingAppliesKeepLiveSpan(t *testing.T) {
	t.Cleanup(func() {
		resetEdgeTracingGlobals()
		agenttrace.SetKeepLiveSpan(false)
	})
	cfg := gateConfig(false, "")
	startAgentTracing(cfg, nil, nil)
	if agenttrace.KeepLiveSpan() {
		t.Fatal("off by default")
	}
	cfg.AgentTracing.KeepLiveSpan = true
	startAgentTracing(cfg, nil, nil)
	if !agenttrace.KeepLiveSpan() {
		t.Fatal("the opt-in is not applied")
	}
}

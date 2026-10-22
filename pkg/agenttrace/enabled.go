package agenttrace

import "sync/atomic"

var (
	enabled atomic.Bool
	// requireHub and hubOn gate capture on an edge by the hub it reports to
	// (RequireHubTracing, SetHubTracing). The hub process never requires one.
	requireHub atomic.Bool
	hubOn      atomic.Bool
)

// Enabled reports whether agent tracing is on for this process. It is set
// once at start-up and read once per request; it allocates nothing.
//
// Only the Enterprise start gate (services/agenttracing, or the Enterprise
// microgateway) turns it on: the factory is registered, the licence is valid
// and AGENT_TRACING_ENABLED=true. In Community builds it stays false. On an
// edge it is also off while its hub has agent tracing off.
func Enabled() bool { return enabled.Load() && (!requireHub.Load() || hubOn.Load()) }

// SetEnabled is called by the start gate. Tests use it too; restore the old
// value with t.Cleanup.
func SetEnabled(on bool) { enabled.Store(on) }

// RequireHubTracing makes capture depend on the hub (the edge's start gate
// calls it): events a hub with tracing off would discard are never built.
func RequireHubTracing(on bool) { requireHub.Store(on) }

// SetHubTracing records whether the hub has agent tracing on: its latest
// config snapshot carried the content policy. The edge's config sync calls
// it on every applied snapshot.
func SetHubTracing(on bool) { hubOn.Store(on) }

package agenttrace

import "sync/atomic"

var enabled atomic.Bool

// Enabled reports whether agent tracing is on for this process. It is set
// once at start-up and read once per request; it allocates nothing.
//
// Only the Enterprise start gate (services/agenttracing, or the Enterprise
// microgateway) turns it on: the factory is registered, the licence is valid
// and AGENT_TRACING_ENABLED=true. In Community builds it stays false.
func Enabled() bool { return enabled.Load() }

// SetEnabled is called by the start gate. Tests use it too; restore the old
// value with t.Cleanup.
func SetEnabled(on bool) { enabled.Store(on) }

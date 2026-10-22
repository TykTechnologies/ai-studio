package agenttrace

import "sync/atomic"

// exportActive is on while the Enterprise OTel exporter exports assembled
// agent-trace spans (spec §5.1). The assembled LLM span is exported under the
// gateway span id the upstream call already sees as its parent, so the proxy
// then leaves out its own live span for captured requests: one call, one
// span.
var exportActive atomic.Bool

// SetExportActive is called by the exporter when it starts and stops.
func SetExportActive(on bool) { exportActive.Store(on) }

// ExportActive reports whether assembled spans are being exported.
func ExportActive() bool { return exportActive.Load() }

// keepLiveSpan is an edge's opt-in (AGENT_TRACING_KEEP_LIVE_SPAN, jyx.176)
// to keep its live proxy span while the hub exports assembled spans, for an
// edge whose OTel collector is not the hub's export target. The live span
// then has its own span id and links to the gateway span id the assembled
// span is exported under.
var keepLiveSpan atomic.Bool

// SetKeepLiveSpan sets the opt-in; edges call it at start-up.
func SetKeepLiveSpan(on bool) { keepLiveSpan.Store(on) }

// KeepLiveSpan reports the opt-in.
func KeepLiveSpan() bool { return keepLiveSpan.Load() }

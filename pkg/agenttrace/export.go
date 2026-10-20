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

// Package agenttracing is the edition seam for Agent Call Tracing (spec:
// enterprise/features/agenttracing/SPEC.md). The feature is Enterprise-only
// (decision D2): this package holds the interface, the factory the
// Enterprise implementation registers through, the start gate, and the
// Community stub that answers every query with ErrEnterpriseFeature (the API
// maps it to 402 via authz.EnterpriseRequired).
//
// The gateway does not call this package on the request path; it reads the
// process flag pkg/agenttrace.Enabled, which only the start gate here turns on.
package agenttracing

import (
	"context"
	"errors"
)

// ErrEnterpriseFeature is returned by the Community stub for every query.
var ErrEnterpriseFeature = errors.New("agent call tracing is an Enterprise Edition feature - visit https://tyk.io/ai-studio/pricing for more information")

// ErrDisabled is returned when the Enterprise implementation is linked in but
// tracing is not running (AGENT_TRACING_ENABLED is off or the licence is not
// valid).
var ErrDisabled = errors.New("agent call tracing is disabled (set AGENT_TRACING_ENABLED=true with a valid Enterprise licence)")

// Service is Agent Call Tracing on the hub: the assembler, retention and
// export workers, and (added by the API beads) the query and export methods.
type Service interface {
	// Start launches background work (assembler, retention, exporters).
	Start(ctx context.Context) error
	// Stop stops background work. Safe to call more than once.
	Stop()
	// Running reports whether tracing is on for this node: the gate passed
	// and the implementation started.
	Running() bool
	// IsEnterpriseAvailable reports whether the Enterprise implementation is
	// linked in, whether or not it is running.
	IsEnterpriseAvailable() bool

	Querier
	ContentService
	Exporter
}

// Package agenttrace is the capture contract for Agent Call Tracing, shared
// by the gateway hooks in proxy/, the embedded and edge transports, and the
// Enterprise assembler (spec: enterprise/features/agenttracing/SPEC.md).
//
// It holds three things:
//
//   - The flag. Enabled is an atomic set once at start-up by the Enterprise
//     service (services/agenttracing) or the Enterprise microgateway; it is
//     false in Community builds. Gateway code reads it once per request.
//   - The capture event. Event is one observed span (an LLM call, a tool or
//     MCP call, a datasource call, a plugin endpoint or a gateway event) as
//     the gateway saw it, plus its correlation extracts. ToProto and
//     FromProto convert it for the edge pulse and the staging table.
//   - The per-request recorder. Begin attaches a Recorder to the request
//     context; hooks find it with From and fill the span; Finish hands the
//     events to the registered Sink. Every entry point is a no-op that
//     allocates nothing when the flag is off or no recorder is attached, so
//     call sites need no guard of their own.
//
// The package must not depend on proxy/, models/ or GORM (a test enforces
// this) so both runtimes and the assembler can import it.
package agenttrace

package guardrails

import "fmt"

// Capability is what an administrator attests an app can do with what its
// LLM produces (features/GuardrailExceptions.md). It bounds how far a
// guardrail exception may soften a verdict for the app: the less the app can
// do, the softer an exception may make a block.
//
// The gateway cannot see everything an app can do (a CI job's own GitHub
// token, say), so the value is attested by a full administrator; Studio only
// suggests one (services.SuggestAppCapability).
type Capability string

const (
	// CapabilityUnattested is an app nobody has attested yet. It is treated
	// as CapabilityActing, the strictest.
	CapabilityUnattested Capability = ""
	// CapabilityReadOnly: the app only reads model output; it has no tools.
	CapabilityReadOnly Capability = "read_only"
	// CapabilityReadTools: the app's tools only read (GET, read-only MCP).
	CapabilityReadTools Capability = "read_tools"
	// CapabilityActing: the app's tools can change things.
	CapabilityActing Capability = "acting"
)

// ParseCapability checks a stored or submitted value. The empty string is
// valid: it is the unattested state.
func ParseCapability(s string) (Capability, error) {
	switch c := Capability(s); c {
	case CapabilityUnattested, CapabilityReadOnly, CapabilityReadTools, CapabilityActing:
		return c, nil
	}
	return "", fmt.Errorf("unknown guardrail capability %q (want read_only, read_tools, acting or empty)", s)
}

// Effective is the capability the floors apply: an unattested (or unknown)
// value is acting.
func (c Capability) Effective() Capability {
	switch c {
	case CapabilityReadOnly, CapabilityReadTools:
		return c
	}
	return CapabilityActing
}

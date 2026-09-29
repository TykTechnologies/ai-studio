package scripting

import "github.com/TykTechnologies/midsommar/v2/scripting/engine"

// The script types live in package engine, so that the enterprise runtime can
// register itself without importing this package (see engine's doc).
type (
	// ScriptInput provides rich context to scripts including messages, metadata, and vendor info
	ScriptInput = engine.ScriptInput
	// ComplianceEventOutput represents a compliance event reported by a filter script.
	ComplianceEventOutput = engine.ComplianceEventOutput
	// ScriptOutput represents the result of script execution
	ScriptOutput = engine.ScriptOutput
)

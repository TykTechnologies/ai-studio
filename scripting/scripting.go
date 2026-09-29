package scripting

import (
	"log/slog"

	"github.com/TykTechnologies/midsommar/v2/scripting/engine"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// runScript runs the script with the registered runtime (the enterprise
// edition's Tengo engine). The Community Edition registers none, so scripts
// pass through unchanged.
func (sr *ScriptRunner) runScript(input *ScriptInput, serviceRef services.ServiceInterface) (*ScriptOutput, error) {
	if run := engine.Registered(); run != nil {
		return run(sr.source, input, serviceRef)
	}
	slog.Warn("⚠️ CE: Script execution skipped (enterprise feature)")
	return &ScriptOutput{
		Block:   false,
		Payload: input.RawInput, // Pass through unchanged
		Message: "",
	}, nil
}

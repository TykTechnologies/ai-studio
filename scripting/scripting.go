//go:build !enterprise

package scripting

import (
	"log/slog"

	"github.com/TykTechnologies/midsommar/v2/services"
)

// RunScript always passes through in CE (enterprise feature)
func (sr *ScriptRunner) RunScript(input *ScriptInput, serviceRef services.ServiceInterface) (*ScriptOutput, error) {
	slog.Warn("⚠️ CE: Script execution skipped (enterprise feature)")
	return &ScriptOutput{
		Block:   false,
		Payload: input.RawInput, // Pass through unchanged
		Message: "",
	}, nil
}

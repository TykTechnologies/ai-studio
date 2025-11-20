//go:build !enterprise

package scripting

import (
	"log/slog"

	"github.com/TykTechnologies/midsommar/v2/services"
)

// RunFilter always allows in CE (enterprise feature)
func (sr *ScriptRunner) RunFilter(payload string, serviceRef services.ServiceInterface) error {
	slog.Warn("⚠️ CE: Filter execution skipped (enterprise feature)")
	return nil // Always allow in CE
}

// RunMiddleware always returns original payload in CE (enterprise feature)
func (sr *ScriptRunner) RunMiddleware(payload string, serviceRef services.ServiceInterface) (string, error) {
	return payload, nil // Pass through unchanged in CE
}

// RunFilter is a convenience function that always allows in CE
func RunFilter(sourceCode string, payload string, svcRef services.ServiceInterface) error {
	return nil // Always allow in CE
}

// RunScript always passes through in CE (enterprise feature)
func (sr *ScriptRunner) RunScript(input *ScriptInput, serviceRef services.ServiceInterface) (*ScriptOutput, error) {
	slog.Warn("⚠️ CE: Script execution skipped (enterprise feature)")
	return &ScriptOutput{
		Block:   false,
		Payload: input.RawInput, // Pass through unchanged
		Message: "",
	}, nil
}

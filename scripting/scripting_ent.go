//go:build enterprise

package scripting

import (
	ent_scripting "github.com/TykTechnologies/midsommar/v2/enterprise/scripting"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// RunFilter executes a filter script using enterprise Tengo implementation
func (sr *ScriptRunner) RunFilter(payload string, serviceRef services.ServiceInterface) error {
	runner := ent_scripting.NewScriptRunner(sr.source)
	return runner.RunFilter(payload, serviceRef)
}

// RunMiddleware executes a middleware script using enterprise Tengo implementation
func (sr *ScriptRunner) RunMiddleware(payload string, serviceRef services.ServiceInterface) (string, error) {
	runner := ent_scripting.NewScriptRunner(sr.source)
	return runner.RunMiddleware(payload, serviceRef)
}

// RunFilter is a convenience function that uses enterprise Tengo implementation
func RunFilter(sourceCode string, payload string, svcRef services.ServiceInterface) error {
	return ent_scripting.RunFilter(sourceCode, payload, svcRef)
}

// RunScript executes a unified script using enterprise Tengo implementation
func (sr *ScriptRunner) RunScript(input *ScriptInput, serviceRef services.ServiceInterface) (*ScriptOutput, error) {
	runner := ent_scripting.NewScriptRunner(sr.source)

	// Convert base ScriptInput to enterprise ScriptInput
	entInput := &ent_scripting.ScriptInput{
		RawInput:   input.RawInput,
		Messages:   input.Messages,
		VendorName: input.VendorName,
		ModelName:  input.ModelName,
		Context:    input.Context,
		IsChat:     input.IsChat,
	}

	// Call enterprise RunScript
	entOutput, err := runner.RunScript(entInput, serviceRef)
	if err != nil {
		return nil, err
	}

	// Convert enterprise ScriptOutput back to base ScriptOutput
	output := &ScriptOutput{
		Block:   entOutput.Block,
		Payload: entOutput.Payload,
		Message: entOutput.Message,
	}

	return output, nil
}

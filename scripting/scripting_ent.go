//go:build enterprise

package scripting

import (
	ent_scripting "github.com/TykTechnologies/midsommar/v2/enterprise/scripting"
	"github.com/TykTechnologies/midsommar/v2/services"
)

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

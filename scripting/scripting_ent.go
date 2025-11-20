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

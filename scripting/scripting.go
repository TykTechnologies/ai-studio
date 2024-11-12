package scripting

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/scriptExtensions"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/d5/tengo/v2"
	"github.com/d5/tengo/v2/stdlib"
)

type ScriptRunner struct {
	mu     sync.Mutex
	source []byte
}

func NewScriptRunner(source []byte) *ScriptRunner {
	return &ScriptRunner{
		source: source,
	}
}

func (sr *ScriptRunner) RunFilter(payload string, serviceRef services.ServiceInterface) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	script := tengo.NewScript(sr.source)
	customModules := scriptExtensions.GetModules(serviceRef)
	stdModules := stdlib.GetModuleMap(stdlib.AllModuleNames()...)
	stdModules.AddBuiltinModule("tyk", customModules)
	script.SetImports(stdModules)

	// Add the payload to the script environment
	err := script.Add("payload", payload)
	if err != nil {
		return fmt.Errorf("error adding payload: %v", err)
	}

	// Compile the script

	compiled, err := script.Compile()
	if err != nil {
		return fmt.Errorf("compilation error: %v", err)
	}

	// Run the compiled script
	if err := compiled.Run(); err != nil {
		return fmt.Errorf("runtime error: %v", err)
	}

	// Get the result of the filter function
	result := compiled.Get("result")
	if result == nil {
		return fmt.Errorf("filter function result not found")
	}

	// Check if the result is truthy
	if result.Bool() == false {
		return fmt.Errorf("filter returned false")
	}

	return nil
}

func (sr *ScriptRunner) RunMiddleware(payload string, serviceRef services.ServiceInterface) (string, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	script := tengo.NewScript(sr.source)
	customModules := scriptExtensions.GetModules(serviceRef)
	stdModules := stdlib.GetModuleMap(stdlib.AllModuleNames()...)
	stdModules.AddBuiltinModule("tyk", customModules)
	script.SetImports(stdModules)

	// Add the payload to the script environment
	err := script.Add("payload", payload)
	if err != nil {
		return "", fmt.Errorf("error adding payload: %v", err)
	}

	// Compile the script

	compiled, err := script.Compile()
	if err != nil {
		return "", fmt.Errorf("compilation error: %v", err)
	}

	// Run the compiled script
	if err := compiled.Run(); err != nil {
		return "", fmt.Errorf("runtime error: %v", err)
	}

	// Get the result of the filter function
	result := compiled.Get("result")
	if result == nil {
		return "", fmt.Errorf("filter function result not found")
	}

	res, ok := result.Value().(string)
	if !ok {
		slog.Error("script returned non-string", "result", res)
		return "", fmt.Errorf("filter returned non-string result")
	}

	return res, nil
}

func RunFilter(sourceCode string, payload string, svcRef services.ServiceInterface) error {
	runner := NewScriptRunner([]byte(sourceCode))
	return runner.RunFilter(payload, svcRef)
}

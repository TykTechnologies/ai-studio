package scripting

import (
	"fmt"
	"sync"

	"github.com/d5/tengo/v2"
)

type ScriptRunner struct {
	mu sync.Mutex
}

func NewScriptRunner() *ScriptRunner {
	return &ScriptRunner{}
}

func (sr *ScriptRunner) runFilter(sourceCode string, payload string) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	script := tengo.NewScript([]byte(sourceCode))

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

func RunFilter(sourceCode string, payload string) error {
	runner := NewScriptRunner()
	return runner.runFilter(sourceCode, payload)
}

//go:build enterprise

package services

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/d5/tengo/v2"
	"github.com/d5/tengo/v2/stdlib"
)

// executeFilterScript executes a filter script using Tengo (enterprise feature)
func (s *FilterService) executeFilterScript(filter *database.Filter, payload map[string]interface{}) (bool, error) {
	slog.Info("🔥 MICROGATEWAY ENT: Executing filter script", "filter_name", filter.Name, "filter_id", filter.ID)

	// Convert payload to JSON string for script processing
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("failed to marshal payload: %w", err)
	}
	payloadString := string(payloadBytes)

	slog.Info("🔥 MICROGATEWAY ENT: Script content", "script", string(filter.Script))

	// Create Tengo script
	script := tengo.NewScript([]byte(filter.Script))

	// Add standard library modules
	script.SetImports(stdlib.GetModuleMap(stdlib.AllModuleNames()...))

	// Add payload variable
	if err := script.Add("payload", payloadString); err != nil {
		return false, fmt.Errorf("failed to add payload variable: %w", err)
	}

	// Compile script
	compiled, err := script.Compile()
	if err != nil {
		return false, fmt.Errorf("script compilation failed: %w", err)
	}

	// Run script
	if err := compiled.Run(); err != nil {
		return false, fmt.Errorf("script execution failed: %w", err)
	}

	// Get result variable
	resultVar := compiled.Get("result")
	if resultVar == nil {
		return false, fmt.Errorf("script must set a 'result' variable")
	}

	// Get the actual value and check if it's truthy
	resultValue := resultVar.Value()
	slog.Info("🔥 MICROGATEWAY ENT: Script result", "result", resultValue)

	if resultBool, ok := resultValue.(bool); ok {
		return resultBool, nil
	}

	// For other types, treat as truthy if not zero/nil/false
	switch v := resultValue.(type) {
	case int64:
		return v != 0, nil
	case string:
		return v != "", nil
	case nil:
		return false, nil
	default:
		return true, nil // Default to true for unknown types
	}
}

// internal/services/filter_service_panic_test.go
package services

import (
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
)

// Issue #535: the gateway runs filter scripts on the goroutine serving the
// request. Tengo panics rather than returning an error for faults it does not
// model (integer divide by zero), so one malformed script used to take the
// whole gateway process down along with every other tenant's traffic.
//
// This test is deliberately untagged: in CE the script is never executed and
// the call must still return cleanly, in enterprise builds the divide by zero
// panics inside the VM and must surface as an error.
func TestExecuteFilterScript_SurvivesScriptPanic(t *testing.T) {
	service := &FilterService{}

	panicScripts := map[string]string{
		"integer divide by zero": `result := 5 / 0`,
		"modulo by zero":         `result := 5 % 0`,
	}

	for name, script := range panicScripts {
		t.Run(name, func(t *testing.T) {
			filter := &database.Filter{Name: "panicking-filter", Script: script}

			// The assertion is the absence of a panic: if executeFilterScript
			// let one through, the test binary would die here rather than fail.
			result, err := service.executeFilterScript(filter, map[string]interface{}{"model": "gpt-4"})

			if err != nil {
				if !strings.Contains(err.Error(), "panic") {
					t.Errorf("executeFilterScript() error = %q, want it to name the panic", err)
				}
				// Fails closed: a filter that could not be evaluated does not
				// get to pass the request.
				if result {
					t.Error("executeFilterScript() result = true after a panic, want false")
				}
				return
			}

			// CE build: filters are disabled, so a pass is the only valid
			// non-error result.
			if !result {
				t.Error("executeFilterScript() result = false with no error, want the CE pass-through")
			}
		})
	}
}

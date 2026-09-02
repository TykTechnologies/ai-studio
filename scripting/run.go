package scripting

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/TykTechnologies/midsommar/v2/services"
)

// RunScript executes a filter script and converts any panic raised by the
// script engine into an ordinary error.
//
// Tengo panics instead of returning an error for faults it does not model —
// integer divide by zero being the common one. Filter scripts run on the
// caller's own goroutine, so an unrecovered panic unwinds through the HTTP
// handler and takes down the whole process: the AI Studio API server, or a
// gateway serving every other tenant's traffic. Recovering here means each
// call site's existing error handling applies instead, so a malformed script
// costs one failed request rather than the process.
//
// The build-tagged runScript implementations hold the actual behaviour: a
// pass-through in CE, the Tengo VM in enterprise builds.
func (sr *ScriptRunner) RunScript(input *ScriptInput, serviceRef services.ServiceInterface) (output *ScriptOutput, err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("filter script panicked, converting to error",
				"panic", r,
				"stack", string(debug.Stack()))
			output = nil
			err = fmt.Errorf("script panic: %v", r)
		}
	}()

	return sr.runScript(input, serviceRef)
}

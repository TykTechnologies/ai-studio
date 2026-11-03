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
		stripReservedEventKeys(output)
	}()

	return sr.runScript(input, serviceRef)
}

// ReservedEventKeys are the compliance event metadata keys only the
// guardrail runner sets: consumers trust them to describe a guardrail
// filter's own verdict (the filter to soften, the alert topic to publish,
// the exception or judge that changed it, whether App owners may see it).
// A script filter's events are free-form, so these keys are removed from
// them; a script cannot pass its event off as another filter's. segments is
// among them because the UI reads segments[].exception and
// segments[].template, and only top-level keys are removed.
var ReservedEventKeys = []string{FilterIDKey, "alert", "exception", "exceptions", "exception_id", "judge", "shadow", "segments", OwnerVisibleKey}

// stripReservedEventKeys removes ReservedEventKeys from a script filter's
// compliance events.
func stripReservedEventKeys(out *ScriptOutput) {
	if out == nil {
		return
	}
	for i := range out.ComplianceEvents {
		for _, k := range ReservedEventKeys {
			delete(out.ComplianceEvents[i].Metadata, k)
		}
	}
}

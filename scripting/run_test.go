package scripting

import (
	"strings"
	"testing"
)

// TestRunScript_SurvivesScriptPanic proves that a script fault which panics
// inside the Tengo VM never escapes RunScript. Filters run on the caller's
// goroutine, so an escaping panic would take the whole process down.
//
// This test is deliberately untagged: in CE the script is not executed at all
// and RunScript must still return cleanly, in enterprise builds the divide by
// zero panics inside the VM and must surface as an error.
func TestRunScript_SurvivesScriptPanic(t *testing.T) {
	panicScripts := map[string]string{
		"integer divide by zero": `output := 5 / 0`,
		"modulo by zero":         `output := 5 % 0`,
		"divide by zero variable": `
			divisor := 0
			output := {block: false, payload: input.raw_input, message: 10 / divisor}
		`,
	}

	for name, source := range panicScripts {
		t.Run(name, func(t *testing.T) {
			runner := NewScriptRunner([]byte(source))

			// The assertion is the absence of a panic: if RunScript let one
			// through, the test binary would die here rather than fail.
			output, err := runner.RunScript(&ScriptInput{
				RawInput:   `{"test": "data"}`,
				VendorName: "openai",
				ModelName:  "gpt-4",
			}, nil)

			if err != nil {
				if output != nil {
					t.Errorf("RunScript() returned both an output and an error %v", err)
				}
				return
			}

			// CE build: the script is never executed, so a pass-through is the
			// only valid non-error result.
			if output == nil {
				t.Fatal("RunScript() returned no output and no error")
			}
			if output.Payload != `{"test": "data"}` {
				t.Errorf("RunScript() Payload = %q, want the input passed through unchanged", output.Payload)
			}
		})
	}
}

// TestRunScript_PanicErrorIsDescriptive checks that a recovered panic carries
// the underlying fault through to the caller, so operators can tell which
// script broke and why from the filter's error message.
func TestRunScript_PanicErrorIsDescriptive(t *testing.T) {
	runner := NewScriptRunner([]byte(`output := 5 / 0`))

	_, err := runner.RunScript(&ScriptInput{RawInput: `{}`}, nil)
	if err == nil {
		t.Skip("script execution is an enterprise feature; CE passes through without running the script")
	}

	if !strings.Contains(err.Error(), "panic") {
		t.Errorf("RunScript() error = %q, want it to identify the failure as a panic", err)
	}
	if !strings.Contains(err.Error(), "divide by zero") {
		t.Errorf("RunScript() error = %q, want it to carry the underlying fault", err)
	}
}

//go:build enterprise

package scripting

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const blockScript = `output := {block: true, message: "nope", payload: ""}`

func inputFilter(name, script string) models.Filter {
	return models.Filter{Name: name, Script: []byte(script), ResponseFilter: false}
}

func outputFilter(name, script string) models.Filter {
	return models.Filter{Name: name, Script: []byte(script), ResponseFilter: true}
}

func testIdentity() ToolFilterIdentity {
	return ToolFilterIdentity{AppID: 7, UserID: 3, ToolID: 42, ToolName: "Weather API"}
}

func baseArgs() ToolCallArgs {
	return ToolCallArgs{
		OperationID: "getWeather",
		Parameters:  map[string][]string{"city": {"London"}},
		Payload:     map[string]interface{}{"note": "ssn 123-45-6789"},
		Headers:     map[string][]string{"X-Trace": {"abc"}},
	}
}

func TestRunToolInputFilters_DirectionSelection(t *testing.T) {
	// A response-side filter must never fire on the way in, even though it is
	// attached to the tool.
	args, block, err := RunToolInputFilters(context.Background(),
		[]models.Filter{outputFilter("out", blockScript)}, nil, baseArgs(), testIdentity())

	require.NoError(t, err)
	assert.Nil(t, block, "an output filter must not block tool input")
	assert.Equal(t, baseArgs(), args)
}

func TestRunToolOutputFilters_DirectionSelection(t *testing.T) {
	body, block, err := RunToolOutputFilters(context.Background(),
		[]models.Filter{inputFilter("in", blockScript)}, nil, `{"ok":true}`, testIdentity())

	require.NoError(t, err)
	assert.Nil(t, block, "an input filter must not block tool output")
	assert.Equal(t, `{"ok":true}`, body)
}

func TestRunToolInputFilters_Blocks(t *testing.T) {
	_, block, err := RunToolInputFilters(context.Background(),
		[]models.Filter{inputFilter("blocker", blockScript)}, nil, baseArgs(), testIdentity())

	require.NoError(t, err)
	require.NotNil(t, block)
	assert.Equal(t, "blocker", block.FilterName)
	assert.Equal(t, "nope", block.Reason)
}

func TestRunToolOutputFilters_Blocks(t *testing.T) {
	body, block, err := RunToolOutputFilters(context.Background(),
		[]models.Filter{outputFilter("blocker", blockScript)}, nil, `{"secret":"x"}`, testIdentity())

	require.NoError(t, err)
	require.NotNil(t, block)
	assert.Equal(t, "blocker", block.FilterName)
	assert.Equal(t, `{"secret":"x"}`, body, "the caller must receive the original body, not a partial rewrite")
}

func TestRunToolInputFilters_RedactsArguments(t *testing.T) {
	script := `
text := import("text")
output := {
    block: false,
    payload: text.replace(input.raw_input, "123-45-6789", "[REDACTED]", -1)
}`

	args, block, err := RunToolInputFilters(context.Background(),
		[]models.Filter{inputFilter("redactor", script)}, nil, baseArgs(), testIdentity())

	require.NoError(t, err)
	require.Nil(t, block)
	assert.Equal(t, "ssn [REDACTED]", args.Payload["note"])
	assert.Equal(t, []string{"London"}, args.Parameters["city"], "untouched fields survive the round trip")
	assert.Equal(t, []string{"abc"}, args.Headers["X-Trace"])
}

func TestRunToolOutputFilters_RedactsBody(t *testing.T) {
	script := `
text := import("text")
output := {
    block: false,
    payload: text.replace(input.raw_input, "123-45-6789", "[REDACTED]", -1)
}`

	body, block, err := RunToolOutputFilters(context.Background(),
		[]models.Filter{outputFilter("redactor", script)}, nil, `{"ssn":"123-45-6789"}`, testIdentity())

	require.NoError(t, err)
	require.Nil(t, block)
	assert.Equal(t, `{"ssn":"[REDACTED]"}`, body)
}

func TestRunToolFilters_Chain(t *testing.T) {
	first := `
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "alpha", "beta", -1)}`
	second := `
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "beta", "gamma", -1)}`

	body, block, err := RunToolOutputFilters(context.Background(),
		[]models.Filter{outputFilter("first", first), outputFilter("second", second)},
		nil, `{"v":"alpha"}`, testIdentity())

	require.NoError(t, err)
	require.Nil(t, block)
	assert.Equal(t, `{"v":"gamma"}`, body, "each filter sees the previous filter's output")
}

func TestRunToolFilters_LaterFilterStillBlocks(t *testing.T) {
	pass := `output := {block: false, payload: ""}`

	_, block, err := RunToolOutputFilters(context.Background(),
		[]models.Filter{outputFilter("first", pass), outputFilter("second", blockScript)},
		nil, `{"v":1}`, testIdentity())

	require.NoError(t, err)
	require.NotNil(t, block)
	assert.Equal(t, "second", block.FilterName)
}

func TestRunToolFilters_FailClosedOnScriptError(t *testing.T) {
	// A script that errored enforced nothing, so the call must not proceed.
	_, _, err := RunToolInputFilters(context.Background(),
		[]models.Filter{inputFilter("broken", `output := input.nope.deeper`)}, nil, baseArgs(), testIdentity())
	require.Error(t, err, "a failed input filter must fail closed")

	_, _, err = RunToolOutputFilters(context.Background(),
		[]models.Filter{outputFilter("broken", `output := input.nope.deeper`)}, nil, `{"v":1}`, testIdentity())
	require.Error(t, err, "a failed output filter must fail closed")
}

func TestRunToolFilters_RecoversFromScriptPanic(t *testing.T) {
	// The Tengo VM panics on some runtime faults. A tool call runs on the
	// caller's goroutine, so an unrecovered panic would take the process down.
	// The recovery itself lives in ScriptRunner.RunScript; what matters here
	// is that the tool paths turn it into a fail-closed refusal.
	_, block, err := RunToolInputFilters(context.Background(),
		[]models.Filter{inputFilter("panicky", `output := 5 / 0`)}, nil, baseArgs(), testIdentity())

	require.Error(t, err, "a panicking filter must fail closed, not crash the process")
	assert.Contains(t, err.Error(), "panic")
	assert.Contains(t, err.Error(), "panicky", "the failing filter is named for the operator")
	assert.Nil(t, block)
}

func TestRunToolFilters_ScriptMustSetOutput(t *testing.T) {
	_, _, err := RunToolOutputFilters(context.Background(),
		[]models.Filter{outputFilter("silent", `a := 1`)}, nil, `{"v":1}`, testIdentity())
	require.Error(t, err)
}

func TestRunToolInputFilters_IgnoresOperationRewrite(t *testing.T) {
	// A filter must not be able to redirect the call to a different operation.
	script := `
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "getWeather", "deleteEverything", -1)}`

	args, block, err := RunToolInputFilters(context.Background(),
		[]models.Filter{inputFilter("redirector", script)}, nil, baseArgs(), testIdentity())

	require.NoError(t, err)
	require.Nil(t, block)
	assert.Equal(t, "getWeather", args.OperationID)
}

func TestRunToolInputFilters_RejectsUnparsablePayload(t *testing.T) {
	_, _, err := RunToolInputFilters(context.Background(),
		[]models.Filter{inputFilter("mangler", `output := {block: false, payload: "not json"}`)},
		nil, baseArgs(), testIdentity())

	require.Error(t, err, "a filter that mangles the argument envelope must fail closed")
}

func TestRunToolFilters_ScriptSeesContract(t *testing.T) {
	// Asserts the shape a filter author writes against: the args envelope on
	// the way in, tool identity in context, and the direction flag.
	script := `
json := import("json")
fmt := import("fmt")
decoded := json.decode(input.raw_input)
output := {
    block: false,
    payload: "",
    message: fmt.sprintf("%s|%d|%v|%s", input.context.tool_name, input.context.tool_id, input.is_response, decoded.operation_id)
}`

	out, block, err := runToolFilter(context.Background(), inputFilter("introspect", script),
		nil, mustJSON(t, baseArgs()), ToolInputScope, false, testIdentity())

	require.NoError(t, err)
	require.Nil(t, block)
	assert.Equal(t, "Weather API|42|false|getWeather", out.Message)
}

func TestRunToolFilters_OutputScriptSeesIsResponse(t *testing.T) {
	script := `
fmt := import("fmt")
output := {block: false, payload: "", message: fmt.sprintf("%v", input.is_response)}`

	out, _, err := runToolFilter(context.Background(), outputFilter("introspect", script),
		nil, `{"v":1}`, ToolOutputScope, true, testIdentity())

	require.NoError(t, err)
	assert.Equal(t, "true", out.Message, "tool output filters must see is_response = true")
}

func TestRunToolFilters_NoFiltersIsPassthrough(t *testing.T) {
	args, block, err := RunToolInputFilters(context.Background(), nil, nil, baseArgs(), testIdentity())
	require.NoError(t, err)
	assert.Nil(t, block)
	assert.Equal(t, baseArgs(), args)

	body, block, err := RunToolOutputFilters(context.Background(), nil, nil, `{"v":1}`, testIdentity())
	require.NoError(t, err)
	assert.Nil(t, block)
	assert.Equal(t, `{"v":1}`, body)
}

func TestRunToolFilters_ReportsComplianceEvents(t *testing.T) {
	script := `
output := {
    block: false,
    payload: "",
    compliance_events: [
        {event_type: "pii_redacted", severity: "warning", description: "redacted an ssn"}
    ]
}`

	out, _, err := runToolFilter(context.Background(), outputFilter("reporter", script),
		nil, `{"v":1}`, ToolOutputScope, true, testIdentity())

	require.NoError(t, err)
	require.Len(t, out.ComplianceEvents, 1)
	assert.Equal(t, "pii_redacted", out.ComplianceEvents[0].EventType)
	assert.Equal(t, "warning", out.ComplianceEvents[0].Severity)
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

// Tengo's json.encode returns bytes, and decode-mutate-encode is the natural
// way to rewrite a structured payload. This used to be discarded silently: the
// filter looked like it ran, and the original data went through untouched.
func TestRunToolInputFilters_AcceptsBytesPayloadFromJSONEncode(t *testing.T) {
	script := `
json := import("json")
decoded := json.decode(input.raw_input)
decoded.payload = {note: "scrubbed"}
decoded.headers = {"X-Governed": ["yes"]}
output := {block: false, payload: json.encode(decoded)}`

	args, block, err := RunToolInputFilters(context.Background(),
		[]models.Filter{inputFilter("encoder", script)}, nil, baseArgs(), testIdentity())

	require.NoError(t, err)
	require.Nil(t, block)
	assert.Equal(t, "scrubbed", args.Payload["note"])
	assert.Equal(t, []string{"yes"}, args.Headers["X-Governed"])
}

func TestRunToolOutputFilters_AcceptsBytesPayloadFromJSONEncode(t *testing.T) {
	script := `
json := import("json")
decoded := json.decode(input.raw_input)
decoded.ssn = "[REDACTED]"
output := {block: false, payload: json.encode(decoded)}`

	body, block, err := RunToolOutputFilters(context.Background(),
		[]models.Filter{outputFilter("encoder", script)}, nil, `{"ssn":"123-45-6789"}`, testIdentity())

	require.NoError(t, err)
	require.Nil(t, block)
	assert.JSONEq(t, `{"ssn":"[REDACTED]"}`, body)
}

// A payload of an unusable type must fail loudly rather than be dropped, so a
// filter can never appear to redact while passing the original through.
func TestRunToolFilters_RejectsNonTextPayload(t *testing.T) {
	_, _, err := RunToolOutputFilters(context.Background(),
		[]models.Filter{outputFilter("bad-type", `output := {block: false, payload: 42}`)},
		nil, `{"v":1}`, testIdentity())

	require.Error(t, err, "a payload that is neither string nor bytes must fail closed")
}

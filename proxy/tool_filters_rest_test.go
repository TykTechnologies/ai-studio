//go:build enterprise

package proxy

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolRequest_InputFilterBlocksBeforeUpstream(t *testing.T) {
	f := newToolFilterFixture(t, `{"message":"should never be reached"}`)
	defer f.teardown()

	f.attachFilter(t, "deny-input", `output := {block: true, message: "no PII to tools"}`, false)

	rr := f.call(t, "submitTestData", map[string]interface{}{"ssn": "123-45-6789"})

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Empty(t, f.upstream.ReceivedRequests,
		"a blocked tool call must never reach the downstream tool")
	assert.NotContains(t, rr.Body.String(), "no PII to tools",
		"the filter's reason must not leak to the caller")
}

func TestToolRequest_InputFilterRedactsBeforeUpstream(t *testing.T) {
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	f.attachFilter(t, "redact-input", `
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "123-45-6789", "[REDACTED]", -1)}`, false)

	rr := f.call(t, "submitTestData", map[string]interface{}{"ssn": "123-45-6789"})

	require.Equal(t, http.StatusOK, rr.Code)
	require.Len(t, f.upstream.ReceivedRequests, 1)

	sent := string(f.upstream.ReceivedRequests[0].Body)
	assert.Contains(t, sent, "[REDACTED]", "the redaction must reach the downstream tool")
	assert.NotContains(t, sent, "123-45-6789", "the original value must not reach the downstream tool")
}

func TestToolRequest_OutputFilterBlocksResponse(t *testing.T) {
	f := newToolFilterFixture(t, `{"ssn":"123-45-6789"}`)
	defer f.teardown()

	f.attachFilter(t, "deny-output", `output := {block: true, message: "sensitive response"}`, true)

	rr := f.call(t, "getTestData", nil)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.NotContains(t, rr.Body.String(), "123-45-6789",
		"a blocked tool response must not reach the caller")
	assert.NotContains(t, rr.Body.String(), "sensitive response")
}

// The two block responses must be indistinguishable, so a caller cannot probe
// the filters to learn whether the downstream tool was actually reached.
func TestToolRequest_InputAndOutputBlocksAreIndistinguishable(t *testing.T) {
	inputBlocked := newToolFilterFixture(t, `{"ok":true}`)
	defer inputBlocked.teardown()
	inputBlocked.attachFilter(t, "deny-input", `output := {block: true, message: "in"}`, false)
	inputRR := inputBlocked.call(t, "getTestData", nil)

	outputBlocked := newToolFilterFixture(t, `{"ok":true}`)
	defer outputBlocked.teardown()
	outputBlocked.attachFilter(t, "deny-output", `output := {block: true, message: "out"}`, true)
	outputRR := outputBlocked.call(t, "getTestData", nil)

	assert.Equal(t, inputRR.Code, outputRR.Code)
	assert.Equal(t, inputRR.Body.String(), outputRR.Body.String())
}

func TestToolRequest_OutputFilterRedactsResponse(t *testing.T) {
	f := newToolFilterFixture(t, `{"ssn":"123-45-6789"}`)
	defer f.teardown()

	f.attachFilter(t, "redact-output", `
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "123-45-6789", "[REDACTED]", -1)}`, true)

	rr := f.call(t, "getTestData", nil)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "[REDACTED]")
	assert.NotContains(t, rr.Body.String(), "123-45-6789")
}

// An input filter must not fire on the way out, and vice versa - the direction
// is what the ResponseFilter flag on the filter means.
func TestToolRequest_DirectionIsHonoured(t *testing.T) {
	f := newToolFilterFixture(t, `{"ok":true}`)
	defer f.teardown()

	// Attached as an input filter, so it must not see the response.
	f.attachFilter(t, "input-only", `
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "ok", "tampered", -1)}`, false)

	rr := f.call(t, "getTestData", nil)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"ok"`, "an input filter must leave the response alone")
	assert.NotContains(t, rr.Body.String(), "tampered")
}

func TestToolRequest_FilterErrorFailsClosed(t *testing.T) {
	f := newToolFilterFixture(t, `{"ok":true}`)
	defer f.teardown()

	f.attachFilter(t, "broken", `output := input.nope.deeper`, false)

	rr := f.call(t, "getTestData", nil)

	assert.Equal(t, http.StatusForbidden, rr.Code,
		"a filter that failed enforced nothing, so the call must not proceed")
	assert.Empty(t, f.upstream.ReceivedRequests)
}

func TestToolRequest_UnfilteredToolIsUnaffected(t *testing.T) {
	f := newToolFilterFixture(t, `{"message":"GET success"}`)
	defer f.teardown()

	rr := f.call(t, "getTestData", nil)

	require.Equal(t, http.StatusOK, rr.Code)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.Equal(t, "GET success", response["message"])
	require.Len(t, f.upstream.ReceivedRequests, 1)
}

// Every block must leave an audit trail, because the caller is deliberately
// told nothing beyond "blocked by policy".
func TestToolRequest_BlockRecordsComplianceEvent(t *testing.T) {
	f := newToolFilterFixture(t, `{"ok":true}`)
	defer f.teardown()

	f.attachFilter(t, "deny-input", `
output := {
    block: true,
    message: "no PII to tools",
    compliance_events: [
        {event_type: "pii_detected", severity: "critical", description: "ssn in tool arguments"}
    ]
}`, false)

	require.Equal(t, http.StatusForbidden, f.call(t, "getTestData", nil).Code)

	events := f.complianceEvents(t, 1)
	require.Len(t, events, 1)
	assert.Equal(t, "deny-input", events[0].FilterName)
	assert.Equal(t, "tool_input", events[0].FilterScope)
	assert.Equal(t, "pii_detected", events[0].EventType)
	assert.Equal(t, "critical", events[0].Severity)
	assert.NotZero(t, events[0].AppID, "the event must be attributable to the calling app")
}

// A filter that blocks without reporting an event still has to be auditable,
// so the runner synthesises one.
func TestToolRequest_SilentBlockStillRecordsComplianceEvent(t *testing.T) {
	f := newToolFilterFixture(t, `{"ok":true}`)
	defer f.teardown()

	f.attachFilter(t, "quiet-denier", `output := {block: true, message: "just no"}`, true)

	require.Equal(t, http.StatusForbidden, f.call(t, "getTestData", nil).Code)

	events := f.complianceEvents(t, 1)
	require.Len(t, events, 1)
	assert.Equal(t, "tool_output_blocked", events[0].EventType)
	assert.Equal(t, "tool_output", events[0].FilterScope)
	assert.Equal(t, "critical", events[0].Severity)
	assert.Equal(t, "just no", events[0].Description, "the reason is kept for audit even though the caller never sees it")
}

// A filter that fails is also a block, and must be audited as one.
func TestToolRequest_FilterErrorRecordsComplianceEvent(t *testing.T) {
	f := newToolFilterFixture(t, `{"ok":true}`)
	defer f.teardown()

	f.attachFilter(t, "broken", `output := input.nope.deeper`, false)

	require.Equal(t, http.StatusForbidden, f.call(t, "getTestData", nil).Code)

	events := f.complianceEvents(t, 1)
	require.Len(t, events, 1)
	assert.Equal(t, "tool_input_blocked", events[0].EventType)
	assert.Contains(t, events[0].Description, "error")
}

// The realistic configuration: one filter guarding each direction on the same
// tool. Both must fire, on their own side only.
func TestToolRequest_BothDirectionsOnOneTool(t *testing.T) {
	f := newToolFilterFixture(t, `{"reply":"secret-out"}`)
	defer f.teardown()

	f.attachFilter(t, "redact-in", `
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "secret-in", "[IN]", -1)}`, false)
	f.attachFilter(t, "redact-out", `
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "secret-out", "[OUT]", -1)}`, true)

	rr := f.call(t, "submitTestData", map[string]interface{}{"v": "secret-in"})

	require.Equal(t, http.StatusOK, rr.Code)
	require.Len(t, f.upstream.ReceivedRequests, 1)

	sent := string(f.upstream.ReceivedRequests[0].Body)
	assert.Contains(t, sent, "[IN]", "the input filter must rewrite what the tool receives")
	assert.NotContains(t, sent, "secret-in")

	assert.Contains(t, rr.Body.String(), "[OUT]", "the output filter must rewrite what the caller receives")
	assert.NotContains(t, rr.Body.String(), "secret-out")
}

// Filters chain in order on the wire, not just in the unit tests.
func TestToolRequest_FiltersChainOverHTTP(t *testing.T) {
	f := newToolFilterFixture(t, `{"v":"alpha"}`)
	defer f.teardown()

	f.attachFilter(t, "first", `
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "alpha", "beta", -1)}`, true)
	f.attachFilter(t, "second", `
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "beta", "gamma", -1)}`, true)

	rr := f.call(t, "getTestData", nil)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "gamma")
}

// A later filter must still be able to block content an earlier one passed.
func TestToolRequest_SecondFilterBlocksOverHTTP(t *testing.T) {
	f := newToolFilterFixture(t, `{"v":1}`)
	defer f.teardown()

	f.attachFilter(t, "permissive", `output := {block: false, payload: ""}`, true)
	f.attachFilter(t, "strict", `output := {block: true, message: "denied"}`, true)

	assert.Equal(t, http.StatusForbidden, f.call(t, "getTestData", nil).Code)
}

// An input filter may rewrite headers and query parameters, not just the body.
func TestToolRequest_InputFilterRewritesHeadersAndParameters(t *testing.T) {
	f := newToolFilterFixture(t, `{"ok":true}`)
	defer f.teardown()

	f.attachFilter(t, "inject", `
json := import("json")
decoded := json.decode(input.raw_input)
decoded.headers = {"X-Governed": ["yes"]}
decoded.parameters = {"tag": ["filtered"]}
output := {block: false, payload: string(json.encode(decoded))}`, false)

	rr := f.call(t, "getTestData", nil)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Len(t, f.upstream.ReceivedRequests, 1)

	received := f.upstream.ReceivedRequests[0]
	assert.Equal(t, "yes", received.Headers.Get("X-Governed"))
}

// All three transports must refuse with the same generic text, and none may
// disclose the filter's name or its reason. This is the property the separate
// per-transport tests can each satisfy while still drifting apart, so it is
// asserted directly.
func TestToolRequest_BlockDisclosesNothingAboutTheFilter(t *testing.T) {
	f := newToolFilterFixture(t, `{"ok":true}`)
	defer f.teardown()

	f.attachFilter(t, "ssn-outbound-guard", `output := {block: true, message: "matched rule DLP-7"}`, false)

	rest := f.call(t, "getTestData", nil).Body.String()
	mcp, isError := newMCPSession(t, f).callTool(t, "getTestData", map[string]interface{}{})

	require.True(t, isError)

	for name, body := range map[string]string{"REST": rest, "MCP": mcp} {
		assert.Contains(t, body, toolBlockedMessage, name)
		assert.NotContains(t, body, "ssn-outbound-guard", "%s must not disclose the filter name", name)
		assert.NotContains(t, body, "DLP-7", "%s must not disclose the filter's reason", name)
	}
}

// typedResultService returns a tool result of a caller-chosen Go type,
// standing in for the microgateway's client, which - unlike Studio's - does
// not force a JSON string response and so yields float64, bool or a map for a
// tool whose spec declares a response schema.
type typedResultService struct {
	*services.Service
	result interface{}
}

func (s *typedResultService) CallToolOperation(uint, string, map[string][]string, map[string]interface{}, map[string][]string) (interface{}, error) {
	return s.result, nil
}

// Filtering must not change the JSON type a client receives. A no-op filter on
// a tool returning the number 123 must still yield 123, not the string "123".
func TestToolRequest_FilteringPreservesResultType(t *testing.T) {
	for _, tc := range []struct {
		name     string
		result   interface{}
		expected string
	}{
		{"number", float64(123), "123"},
		{"boolean", true, "true"},
		{"object", map[string]interface{}{"a": float64(1)}, `{"a":1}`},
		{"array", []interface{}{float64(1), float64(2)}, "[1,2]"},
		{"null", nil, "null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newToolFilterFixtureWithService(t, `unused`, func(svc *services.Service) services.ServiceInterface {
				return &typedResultService{Service: svc, result: tc.result}
			})
			defer f.teardown()

			f.attachFilter(t, "noop", `output := {block: false, payload: ""}`, true)

			rr := f.call(t, "getTestData", nil)

			require.Equal(t, http.StatusOK, rr.Code)
			assert.JSONEq(t, tc.expected, rr.Body.String(),
				"a filter that changed nothing must not change the response type")
		})
	}
}

// When a filter does rewrite the body, the rewritten JSON keeps its type too.
func TestToolRequest_RewriteKeepsJSONType(t *testing.T) {
	f := newToolFilterFixtureWithService(t, `unused`, func(svc *services.Service) services.ServiceInterface {
		return &typedResultService{Service: svc, result: map[string]interface{}{"ssn": "123-45-6789"}}
	})
	defer f.teardown()

	f.attachFilter(t, "redact", `
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "123-45-6789", "[REDACTED]", -1)}`, true)

	rr := f.call(t, "getTestData", nil)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `{"ssn":"[REDACTED]"}`, rr.Body.String())
}

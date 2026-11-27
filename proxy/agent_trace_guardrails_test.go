package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/requestid"
	"github.com/TykTechnologies/midsommar/v2/scripting"
)

// midsommar-clean-c8y: a guardrail's outcome is summarised from its detection
// events; judge verdicts and errors are not findings.
func TestGuardrailOutcome(t *testing.T) {
	finding := func(det, action string, kinds, tools []string) scripting.ComplianceEventOutput {
		return scripting.ComplianceEventOutput{EventType: "guardrail." + det, Metadata: map[string]interface{}{
			"detector": det, "action": action, "kinds": kinds, "tool_names": tools,
		}}
	}
	judge := scripting.ComplianceEventOutput{EventType: scripting.JudgeEventType, Metadata: map[string]interface{}{"verdict": "attack"}}

	cases := []struct {
		name            string
		out             scripting.ScriptOutput
		outcome, detail string
		ok              bool
	}{
		{"nothing found", scripting.ScriptOutput{ComplianceEvents: []scripting.ComplianceEventOutput{judge}}, "", "", false},
		{"quarantined tool result", scripting.ScriptOutput{ComplianceEvents: []scripting.ComplianceEventOutput{
			finding("injection.role_hijack", "quarantine", []string{"tool_result"}, []string{"search_kb"}),
			finding("injection.instruction_override", "quarantine", []string{"tool_result"}, []string{"search_kb"}),
			judge,
		}}, agenttrace.OutcomeModify, "injection.instruction_override, injection.role_hijack in tool_result (search_kb); quarantine", true},
		{"logged only", scripting.ScriptOutput{ComplianceEvents: []scripting.ComplianceEventOutput{
			finding("injection.instruction_override", "log", []string{"text"}, nil),
		}}, agenttrace.OutcomeAllow, "injection.instruction_override in text; log", true},
		{"block wins", scripting.ScriptOutput{Block: true, ComplianceEvents: []scripting.ComplianceEventOutput{
			finding("pii.email", "redact", []string{"text"}, nil),
			finding("secrets.aws", "block", []string{"text"}, nil),
		}}, agenttrace.OutcomeBlock, "pii.email, secrets.aws in text; block", true},
		{"fail-closed block", scripting.ScriptOutput{Block: true}, agenttrace.OutcomeBlock, "block", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outcome, detail, ok := guardrailOutcome(&tc.out)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.outcome, outcome)
			assert.Equal(t, tc.detail, detail)
		})
	}
}

// After a JSON round trip the metadata lists are []any.
func TestGuardrailOutcome_JSONMetadata(t *testing.T) {
	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(`{"detector":"secrets.test","action":"quarantine","kinds":["tool_result"],"tool_names":["read_file"]}`), &meta))
	outcome, detail, ok := guardrailOutcome(&scripting.ScriptOutput{ComplianceEvents: []scripting.ComplianceEventOutput{{Metadata: meta}}})
	require.True(t, ok)
	assert.Equal(t, agenttrace.OutcomeModify, outcome)
	assert.Equal(t, "secrets.test in tool_result (read_file); quarantine", detail)
}

const toolResultSecretBody = `{"model":"claude-sonnet-5","max_tokens":64,"messages":[{"role":"user","content":"read it"},` +
	`{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"read_file","input":{}}]},` +
	`{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"API_KEY=SECRET"}]}]}`

func anthropicGuardrailHarness(t *testing.T, filter *models.Filter) *failoverHarness {
	t.Helper()
	return newFailoverHarness(t, serveAnthropic(t), serveStatus(500), func(primary, _ *models.LLM) {
		primary.Vendor = models.ANTHROPIC
		primary.Failover = models.LLMFailover{}
		primary.Filters = []*models.Filter{filter}
	})
}

func blockingGuardrail(name string) *models.Filter {
	f := redactGuardrail(guardrails.ScopeAllUser)
	f.Name = name
	f.Config["on_detect"] = guardrails.ActionBlock
	return f
}

// A guardrail that quarantines a tool result records it on the trace, named,
// with where it matched and never the content; the request goes on.
func TestAgentTraceGuardrail_QuarantineIsRecorded(t *testing.T) {
	sink := withCapture(t)
	h := anthropicGuardrailHarness(t, blockingGuardrail("tool secrets"))
	resp, out := h.post("/llm/rest/primary/v1/messages", toolResultSecretBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

	ev := waitLLMEvent(t, sink)
	got := gatewayEvents(ev, agenttrace.GatewayEventGuardrail)
	require.Len(t, got, 1)
	assert.Equal(t, agenttrace.OutcomeModify, got[0].Outcome)
	assert.Equal(t, "tool secrets", got[0].Name)
	assert.Equal(t, "secrets.test in tool_result (read_file); quarantine", got[0].Detail)
	assert.NotContains(t, got[0].Detail, "SECRET")
	assert.NotEqual(t, agenttrace.StatusError, ev.Status, "a quarantine does not fail the call")
	// tkg.2: the request id its compliance events record, to link them.
	assert.Equal(t, resp.Header.Get(requestid.Header), ev.Attributes[agenttrace.AttrRequestID])
	assert.True(t, requestid.Valid(ev.Attributes[agenttrace.AttrRequestID]))
}

// A guardrail block is recorded once, under the guardrail's name, not again
// as the generic request_filter rule.
func TestAgentTraceGuardrail_BlockIsNamedOnce(t *testing.T) {
	sink := withCapture(t)
	h := anthropicGuardrailHarness(t, blockingGuardrail("no secrets"))
	resp, out := h.post("/llm/rest/primary/v1/messages",
		`{"model":"claude-sonnet-5","max_tokens":64,"messages":[{"role":"user","content":"my key is SECRET"}]}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
	assert.Empty(t, h.primaryVendor.calls())

	ev := waitLLMEvent(t, sink)
	assert.Equal(t, agenttrace.StatusError, ev.Status)
	got := gatewayEvents(ev, agenttrace.GatewayEventGuardrail)
	require.Len(t, got, 1)
	assert.Equal(t, agenttrace.OutcomeBlock, got[0].Outcome)
	assert.Equal(t, "no secrets", got[0].Name)
	assert.Equal(t, "secrets.test in text; block", got[0].Detail)
	assert.Empty(t, gatewayEvents(ev, agenttrace.GatewayEventFilter), "no second, unnamed block event")
	assert.Equal(t, resp.Header.Get(requestid.Header), ev.Attributes[agenttrace.AttrRequestID])
}

// tkg.2: the request id is the one the guardrail's compliance events carry,
// never a fresh one, and nothing is noted without a valid id.
func TestNoteRequestID(t *testing.T) {
	const id = "tyai_0123456789abcdef0123456789abcdef"
	out := &scripting.ScriptOutput{ComplianceEvents: []scripting.ComplianceEventOutput{
		{Metadata: map[string]interface{}{"request_id": "not-an-id"}},
		{Metadata: map[string]interface{}{"request_id": id}},
	}}
	withCapture(t)
	note := func(ctx context.Context, out *scripting.ScriptOutput) map[string]string {
		ctx, rec := agenttrace.Begin(ctx)
		require.NotNil(t, rec)
		noteRequestID(ctx, rec, out)
		var attrs map[string]string
		rec.Update(func(ev *agenttrace.Event) { attrs = ev.Attributes })
		return attrs
	}
	assert.Equal(t, id, note(context.Background(), out)[agenttrace.AttrRequestID])

	other := "tyai_ffffffffffffffffffffffffffffffff"
	ctx := requestid.WithID(context.Background(), other, "")
	assert.Equal(t, id, note(ctx, out)[agenttrace.AttrRequestID], "the events' id wins")
	assert.Equal(t, other, note(ctx, &scripting.ScriptOutput{})[agenttrace.AttrRequestID], "else the request's")
	assert.Empty(t, note(context.Background(), &scripting.ScriptOutput{})[agenttrace.AttrRequestID], "never minted")
}

// midsommar-clean-b66: an /ai/ call that a filter refused still records what
// was asked of which LLM and model, and the request itself.
func TestAgentTraceGuardrail_RefusedBridgeCallKeepsTheRequest(t *testing.T) {
	sink := withCapture(t)
	h := newFailoverHarness(t, serveOpenAIText("never"), serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Failover = models.LLMFailover{}
		primary.Filters = []*models.Filter{blockingGuardrail("no secrets")}
	})
	resp, out := h.post("/ai/primary/v1/chat/completions",
		`{"model":"gpt-4o","messages":[{"role":"user","content":"my key is SECRET"}]}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
	assert.Empty(t, h.primaryVendor.calls())

	var outer, inner *agenttrace.Event
	require.Eventually(t, func() bool {
		outer, inner = nil, nil
		for _, ev := range sink.llmEvents() {
			if ev.AttemptOf.IsZero() {
				outer = ev
			} else {
				inner = ev
			}
		}
		return outer != nil && inner != nil && outer.RequestModel != ""
	}, 5*time.Second, 10*time.Millisecond)

	assert.Equal(t, agenttrace.StatusError, outer.Status)
	assert.Equal(t, http.StatusBadRequest, outer.HTTPStatus)
	assert.Equal(t, "gpt-4o", outer.RequestModel)
	assert.Equal(t, agenttrace.ResourceLLM, outer.Resource.Type)
	assert.Equal(t, h.primary.Name, outer.Resource.Name)
	assert.NotEmpty(t, outer.Provider)
	assert.EqualValues(t, 1, outer.Attempt)
	assert.True(t, hasPart(outer, agenttrace.PartRequest), "the refused request is kept for the transcript")
	assert.False(t, hasPart(outer, agenttrace.PartResponse))

	// The guardrail's block is on the inner hop, which the assembler shows
	// as the attempt's child.
	got := gatewayEvents(inner, agenttrace.GatewayEventGuardrail)
	require.Len(t, got, 1)
	assert.Equal(t, "no secrets", got[0].Name)
}

// hasPart reports whether the capture kept part (raw, before the content
// pipeline the capture worker runs).
func hasPart(ev *agenttrace.Event, part string) bool {
	for _, p := range ev.Raw {
		if p.Part == part {
			return true
		}
	}
	return false
}

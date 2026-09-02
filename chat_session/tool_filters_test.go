//go:build enterprise

package chat_session

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// chatToolFilterFixture is a chat session holding one REST tool that points at
// a local upstream, so tool filtering can be asserted without network access.
type chatToolFilterFixture struct {
	session  *ChatSession
	upstream *httptest.Server
	received *int
	lastBody *string
}

func newChatToolFilterFixture(t *testing.T, upstreamBody string, filters []models.Filter) *chatToolFilterFixture {
	t.Helper()

	received := 0
	lastBody := ""
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received++
		if b, err := io.ReadAll(r.Body); err == nil {
			lastBody = string(b)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(upstreamBody))
	}))
	t.Cleanup(upstream.Close)

	spec := fmt.Sprintf(`{"openapi":"3.0.0","info":{"title":"t","version":"1"},"servers":[{"url":"%s"}],`+
		`"paths":{"/x":{"post":{"operationId":"sendData","responses":{"200":{"description":"OK"}}}}}}`, upstream.URL)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))

	uid := uint(1)
	sid := "tool-filter-session"
	chatRef := &models.Chat{
		ID:          1,
		Name:        "Test Chat",
		LLMSettings: &models.LLMSettings{ModelName: "dummy"},
		LLM:         &models.LLM{Name: "Dummy LLM", Vendor: models.MOCK_VENDOR},
	}

	cs, err := NewChatSession(chatRef, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
	require.NoError(t, err)
	require.NoError(t, cs.initSession())

	cs.tools = map[string]models.Tool{
		"test_tool": {
			ID:                  1,
			Name:                "Test Tool",
			ToolType:            models.ToolTypeREST,
			OASSpec:             spec,
			AvailableOperations: "sendData",
			Filters:             filters,
		},
	}

	return &chatToolFilterFixture{session: cs, upstream: upstream, received: &received, lastBody: &lastBody}
}

// call runs one tool call through the session and returns the parts handed to
// the model as the tool result.
func (f *chatToolFilterFixture) call(t *testing.T, arguments string) []llms.ToolCallResponse {
	t.Helper()

	choice := &llms.ContentChoice{ToolCalls: []llms.ToolCall{{
		ID:           "call_1",
		Type:         "function",
		FunctionCall: &llms.FunctionCall{Name: "sendData", Arguments: arguments},
	}}}
	toolCall := &llms.MessageContent{Role: llms.ChatMessageTypeAI}
	toolResult := &llms.MessageContent{Role: llms.ChatMessageTypeTool}

	f.session.handleToolCalls(choice, toolCall, toolResult)

	responses := make([]llms.ToolCallResponse, 0, len(toolResult.Parts))
	for _, part := range toolResult.Parts {
		resp, ok := part.(llms.ToolCallResponse)
		require.True(t, ok, "unexpected tool result part %T", part)
		responses = append(responses, resp)
	}
	return responses
}

func chatFilter(name, script string, responseFilter bool) models.Filter {
	return models.Filter{ID: 1, Name: name, Script: []byte(script), ResponseFilter: responseFilter}
}

// A blocked tool response used to be appended to the conversation alongside the
// error, so the model received the very payload the filter refused.
func TestChatToolCall_BlockedOutputIsNotAlsoDelivered(t *testing.T) {
	f := newChatToolFilterFixture(t, `{"ssn":"123-45-6789"}`,
		[]models.Filter{chatFilter("deny-output", `output := {block: true, message: "sensitive"}`, true)})

	responses := f.call(t, `{}`)

	require.Len(t, responses, 1, "a blocked tool call must produce exactly one result part")
	assert.Contains(t, responses[0].Content, scripting.ToolBlockedMessage)
	assert.NotContains(t, responses[0].Content, "123-45-6789",
		"the blocked payload must never reach the model")
	assert.NotContains(t, responses[0].Content, "deny-output",
		"the filter name must not be disclosed to the model")
	assert.NotContains(t, responses[0].Content, "sensitive",
		"the filter's own reason must not be disclosed to the model")
}

func TestChatToolCall_InputFilterBlocksBeforeUpstream(t *testing.T) {
	f := newChatToolFilterFixture(t, `{"ok":true}`,
		[]models.Filter{chatFilter("deny-input", `output := {block: true, message: "no PII"}`, false)})

	responses := f.call(t, `{"body":{"ssn":"123-45-6789"}}`)

	require.Len(t, responses, 1)
	assert.Contains(t, responses[0].Content, scripting.ToolBlockedMessage)
	assert.NotContains(t, responses[0].Content, "deny-input",
		"the filter name must not be disclosed to the model")
	assert.NotContains(t, responses[0].Content, "no PII",
		"the filter's own reason must not be disclosed to the model")
	assert.Equal(t, 0, *f.received, "a blocked tool call must never reach the downstream tool")
}

func TestChatToolCall_OutputFilterRedacts(t *testing.T) {
	f := newChatToolFilterFixture(t, `{"ssn":"123-45-6789"}`,
		[]models.Filter{chatFilter("redact-output", `
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "123-45-6789", "[REDACTED]", -1)}`, true)})

	responses := f.call(t, `{}`)

	require.Len(t, responses, 1)
	assert.Contains(t, responses[0].Content, "[REDACTED]")
	assert.NotContains(t, responses[0].Content, "123-45-6789")
}

// An input filter must not fire on the response, and vice versa.
func TestChatToolCall_DirectionIsHonoured(t *testing.T) {
	f := newChatToolFilterFixture(t, `{"value":"ok"}`,
		[]models.Filter{chatFilter("input-only", `
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "ok", "tampered", -1)}`, false)})

	responses := f.call(t, `{}`)

	require.Len(t, responses, 1)
	assert.Contains(t, responses[0].Content, `"ok"`)
	assert.NotContains(t, responses[0].Content, "tampered")
}

func TestChatToolCall_FilterErrorFailsClosed(t *testing.T) {
	f := newChatToolFilterFixture(t, `{"ok":true}`,
		[]models.Filter{chatFilter("broken", `output := input.nope.deeper`, false)})

	responses := f.call(t, `{}`)

	require.Len(t, responses, 1)
	assert.Contains(t, responses[0].Content, scripting.ToolBlockedMessage)
	assert.NotContains(t, responses[0].Content, "broken",
		"a failing filter must not name itself to the model")
	assert.NotContains(t, responses[0].Content, "Runtime Error",
		"Tengo diagnostics can quote the filter source and must not reach the model")
	assert.Equal(t, 0, *f.received)
}

func TestChatToolCall_UnfilteredToolIsUnaffected(t *testing.T) {
	f := newChatToolFilterFixture(t, `{"ok":true}`, nil)

	responses := f.call(t, `{}`)

	require.Len(t, responses, 1)
	assert.JSONEq(t, `{"ok":true}`, responses[0].Content)
	assert.Equal(t, 1, *f.received)
}

// The chat path's input rewrite must reach the downstream tool.
func TestChatToolCall_InputFilterRedactsBeforeUpstream(t *testing.T) {
	f := newChatToolFilterFixture(t, `{"ok":true}`,
		[]models.Filter{chatFilter("redact-input", `
json := import("json")
decoded := json.decode(input.raw_input)
decoded.payload = {note: "[REDACTED]"}
output := {block: false, payload: string(json.encode(decoded))}`, false)})

	responses := f.call(t, `{"body":{"ssn":"123-45-6789"}}`)

	require.Len(t, responses, 1)
	require.Equal(t, 1, *f.received)
	assert.Contains(t, *f.lastBody, "[REDACTED]")
	assert.NotContains(t, *f.lastBody, "123-45-6789",
		"the original value must not reach the downstream tool")
}

// Both directions on one tool, from a chat session.
func TestChatToolCall_BothDirectionsOnOneTool(t *testing.T) {
	f := newChatToolFilterFixture(t, `{"reply":"secret-out"}`, []models.Filter{
		{ID: 1, Name: "mark-in", ResponseFilter: false, Script: []byte(`
json := import("json")
decoded := json.decode(input.raw_input)
decoded.payload = {marker: "IN"}
output := {block: false, payload: string(json.encode(decoded))}`)},
		{ID: 2, Name: "redact-out", ResponseFilter: true, Script: []byte(`
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "secret-out", "[OUT]", -1)}`)},
	})

	responses := f.call(t, `{}`)

	require.Len(t, responses, 1)
	assert.Contains(t, *f.lastBody, "IN")
	assert.Contains(t, responses[0].Content, "[OUT]")
	assert.NotContains(t, responses[0].Content, "secret-out")
}

// Two tool calls in one model turn: blocking the first must not suppress or
// contaminate the second.
func TestChatToolCall_BlockOnOneCallDoesNotAffectAnother(t *testing.T) {
	f := newChatToolFilterFixture(t, `{"ok":true}`,
		[]models.Filter{chatFilter("deny-large", `
text := import("text")
blocked := text.contains(input.raw_input, "forbidden")
output := {block: blocked, message: "denied"}`, false)})

	choice := &llms.ContentChoice{ToolCalls: []llms.ToolCall{
		{ID: "call_1", Type: "function", FunctionCall: &llms.FunctionCall{Name: "sendData", Arguments: `{"body":{"v":"forbidden"}}`}},
		{ID: "call_2", Type: "function", FunctionCall: &llms.FunctionCall{Name: "sendData", Arguments: `{"body":{"v":"fine"}}`}},
	}}
	toolCall := &llms.MessageContent{Role: llms.ChatMessageTypeAI}
	toolResult := &llms.MessageContent{Role: llms.ChatMessageTypeTool}

	f.session.handleToolCalls(choice, toolCall, toolResult)

	require.Len(t, toolResult.Parts, 2, "each tool call contributes exactly one result part")

	first := toolResult.Parts[0].(llms.ToolCallResponse)
	second := toolResult.Parts[1].(llms.ToolCallResponse)

	assert.Equal(t, "call_1", first.ToolCallID)
	assert.Contains(t, first.Content, scripting.ToolBlockedMessage)

	assert.Equal(t, "call_2", second.ToolCallID)
	assert.JSONEq(t, `{"ok":true}`, second.Content, "the allowed call must still succeed")
	assert.Equal(t, 1, *f.received, "only the allowed call reaches the tool")
}

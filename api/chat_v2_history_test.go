package api

import (
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

func row(t *testing.T, id uint, mc llms.MessageContent) models.CMessage {
	t.Helper()
	b, err := json.Marshal(mc)
	require.NoError(t, err)
	return models.CMessage{ID: id, Content: b}
}

func TestMaterialiseHistory_FoldsAssistantTurnAndAttachesToolResults(t *testing.T) {
	rows := []models.CMessage{
		row(t, 1, llms.TextParts(llms.ChatMessageTypeSystem, "You are helpful")),
		row(t, 2, llms.TextParts(llms.ChatMessageTypeHuman, "[CONTEXT]\nContext for this message: \ndoc one\n[/CONTEXT]\nWhat is the weather?")),
		row(t, 3, llms.TextParts(llms.ChatMessageTypeAI, "Let me check.")),
		row(t, 4, llms.MessageContent{Role: llms.ChatMessageTypeAI, Parts: []llms.ContentPart{
			llms.ToolCall{ID: "call_1", Type: "function", FunctionCall: &llms.FunctionCall{Name: "getWeather", Arguments: `{"city":"Auckland"}`}},
		}}),
		row(t, 5, llms.MessageContent{Role: llms.ChatMessageTypeTool, Parts: []llms.ContentPart{
			llms.ToolCallResponse{ToolCallID: "call_1", Name: "getWeather", Content: `{"temp": 18}`},
		}}),
		row(t, 6, llms.TextParts(llms.ChatMessageTypeAI, "It is 18 degrees.")),
		row(t, 7, llms.TextParts(llms.ChatMessageTypeHuman, "Thanks")),
	}

	msgs := materialiseHistory(rows)
	require.Len(t, msgs, 3, "system row dropped, assistant rows folded: user, assistant, user")

	user := msgs[0]
	assert.Equal(t, "user", user.Role)
	assert.Equal(t, "2", user.ID)
	require.Len(t, user.Parts, 2)
	assert.Equal(t, "data", user.Parts[0].Type)
	assert.Equal(t, "context", user.Parts[0].Name)
	assert.JSONEq(t, `{"text":"doc one","source":"rag"}`, string(user.Parts[0].Data))
	assert.Equal(t, "What is the weather?", user.Parts[1].Text)

	ai := msgs[1]
	assert.Equal(t, "assistant", ai.Role)
	assert.Equal(t, "3", ai.ID, "folded message keeps the first row's id")
	require.Len(t, ai.Parts, 3)
	assert.Equal(t, "Let me check.", ai.Parts[0].Text)
	tc := ai.Parts[1]
	assert.Equal(t, "tool-call", tc.Type)
	assert.Equal(t, "call_1", tc.ToolCallID)
	assert.Equal(t, "getWeather", tc.ToolName)
	assert.JSONEq(t, `{"city":"Auckland"}`, string(tc.Args))
	assert.JSONEq(t, `{"temp": 18}`, string(tc.Result))
	assert.False(t, tc.IsError)
	assert.Equal(t, "It is 18 degrees.", ai.Parts[2].Text)

	assert.Equal(t, "Thanks", msgs[2].Parts[0].Text)
}

func TestMaterialiseHistory_ToolDocsContextAndErrors(t *testing.T) {
	rows := []models.CMessage{
		row(t, 1, llms.TextParts(llms.ChatMessageTypeHuman, "[CONTEXT]\nThe following additional documentation file 'a.md' has been provided for the tool 'T' to help you use it:\nhello\n[/CONTEXT]")),
		row(t, 2, llms.MessageContent{Role: llms.ChatMessageTypeTool, Parts: []llms.ContentPart{
			llms.ToolCallResponse{ToolCallID: "orphan", Name: "broken", Content: "ERROR: tool not found: broken"},
		}}),
	}
	msgs := materialiseHistory(rows)
	require.Len(t, msgs, 2)
	require.Len(t, msgs[0].Parts, 1, "a docs-only row has no text part")
	assert.JSONEq(t, `{"text":"The following additional documentation file 'a.md' has been provided for the tool 'T' to help you use it:\nhello","source":"tool_docs"}`, string(msgs[0].Parts[0].Data))

	tc := msgs[1].Parts[0]
	assert.Equal(t, "tool-call", tc.Type, "a result without its request still shows as a tool call")
	assert.True(t, tc.IsError)
	assert.Equal(t, "broken", tc.ToolName)
	assert.JSONEq(t, `"ERROR: tool not found: broken"`, string(tc.Result))
}

func TestJSONHelpers(t *testing.T) {
	assert.JSONEq(t, `{"a":1}`, string(jsonObjectOrWrap(`{"a":1}`)))
	assert.JSONEq(t, `{}`, string(jsonObjectOrWrap("")))
	assert.JSONEq(t, `{"raw":"not json"}`, string(jsonObjectOrWrap("not json")))
	assert.JSONEq(t, `[1,2]`, string(jsonValueOrString("[1,2]")))
	assert.JSONEq(t, `"plain"`, string(jsonValueOrString("plain")))
	assert.JSONEq(t, `"42"`, string(jsonValueOrString("42")), "scalars stay strings so the client renders them verbatim")
}

func TestMaterialiseHistory_UnwrapsClientAnswers(t *testing.T) {
	envelope := `{"source":"user","untrusted":true,"note":"n","tool":"ask-approval","kind":"approval","answer":{"approved":true,"comment":"ok"}}`
	rows := []models.CMessage{
		row(t, 1, llms.TextParts(llms.ChatMessageTypeHuman, "Delete it")),
		row(t, 2, llms.MessageContent{Role: llms.ChatMessageTypeAI, Parts: []llms.ContentPart{
			llms.ToolCall{ID: "call_1", Type: "function", FunctionCall: &llms.FunctionCall{Name: "ask-approval", Arguments: `{"action":"delete"}`}},
		}}),
		row(t, 3, llms.MessageContent{Role: llms.ChatMessageTypeTool, Parts: []llms.ContentPart{
			llms.ToolCallResponse{ToolCallID: "call_1", Name: "ask-approval", Content: envelope},
		}}),
	}
	msgs := materialiseHistory(rows)
	require.Len(t, msgs, 2)
	part := msgs[1].Parts[0]
	require.Equal(t, "tool-call", part.Type)
	assert.False(t, part.IsError)
	// The person sees their answer, not the envelope the model was given.
	assert.JSONEq(t, `{"approved":true,"comment":"ok"}`, string(part.Result))
}

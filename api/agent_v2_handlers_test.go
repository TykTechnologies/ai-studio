package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/agent_session"
	"github.com/TykTechnologies/midsommar/v2/chat_session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func kinds(evs []chat_session.ChatEvent) []string {
	out := make([]string, 0, len(evs))
	for _, e := range evs {
		out = append(out, e.Kind)
	}
	return out
}

func TestAgentChunkEvents(t *testing.T) {
	assert.Equal(t, []string{chat_session.EventTextDelta},
		kinds(agentChunkEvents(agent_session.AgentMessageChunk{Type: "CONTENT", Content: "hi"})))
	assert.Nil(t, agentChunkEvents(agent_session.AgentMessageChunk{Type: "CONTENT"}), "empty content emits nothing")
	assert.Equal(t, []string{chat_session.EventReasoningDelta},
		kinds(agentChunkEvents(agent_session.AgentMessageChunk{Type: "thinking", Content: "hmm"})))

	call := agentChunkEvents(agent_session.AgentMessageChunk{
		Type:     "TOOL_CALL",
		Metadata: map[string]any{"tool_name": "getWeather", "parameters": map[string]any{"city": "Auckland"}, agent_session.MetadataToolCallID: "call_7"},
	})
	require.Equal(t, []string{chat_session.EventToolCallStart, chat_session.EventToolCallDelta, chat_session.EventToolCallEnd}, kinds(call))
	var start chat_session.ToolCallStartData
	require.NoError(t, json.Unmarshal(call[0].Data, &start))
	assert.Equal(t, "call_7", start.ToolCallID)
	assert.Equal(t, "getWeather", start.ToolName)
	var delta chat_session.ToolCallDeltaData
	require.NoError(t, json.Unmarshal(call[1].Data, &delta))
	assert.JSONEq(t, `{"city":"Auckland"}`, delta.ArgsText)

	res := agentChunkEvents(agent_session.AgentMessageChunk{Type: "TOOL_RESULT", Content: `{"temp":18}`, Metadata: map[string]any{agent_session.MetadataToolCallID: "call_7", "is_error": true}})
	require.Equal(t, []string{chat_session.EventToolResult}, kinds(res))
	var r chat_session.ToolResultData
	require.NoError(t, json.Unmarshal(res[0].Data, &r))
	assert.Equal(t, "call_7", r.ToolCallID)
	assert.True(t, r.IsError)

	assert.Equal(t, []string{chat_session.EventError},
		kinds(agentChunkEvents(agent_session.AgentMessageChunk{Type: "ERROR", Content: "boom"})))
	assert.Nil(t, agentChunkEvents(agent_session.AgentMessageChunk{Type: "DONE"}))
}

func TestTranscriptToV2(t *testing.T) {
	now := time.Now()
	msgs := transcriptToV2([]agent_session.TranscriptMessage{
		{ID: "u1", Role: "user", CreatedAt: now, Parts: []agent_session.TranscriptPart{{Type: "text", Text: "hi"}}},
		{ID: "a2", Role: "assistant", CreatedAt: now, Parts: []agent_session.TranscriptPart{
			{Type: "reasoning", Text: "think"},
			{Type: "tool-call", ToolCallID: "call_1", ToolName: "t", Args: map[string]any{"a": 1}, Result: "ok", HasResult: true},
			{Type: "tool-call", ToolCallID: "call_2", ToolName: "pending"},
			{Type: "text", Text: "done"},
		}},
	})
	require.Len(t, msgs, 2)
	assert.Equal(t, "hi", msgs[0].Parts[0].Text)
	parts := msgs[1].Parts
	require.Len(t, parts, 4)
	assert.Equal(t, "data", parts[0].Type)
	assert.Equal(t, "reasoning", parts[0].Name)
	assert.JSONEq(t, `{"a":1}`, string(parts[1].Args))
	assert.JSONEq(t, `"ok"`, string(parts[1].Result))
	assert.Nil(t, parts[2].Result, "a call without a result has no result field")
	assert.JSONEq(t, `{}`, string(parts[2].Args))
	assert.Equal(t, "done", parts[3].Text)
}

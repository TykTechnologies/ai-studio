package vendorconformance

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sse(frames ...string) string {
	var b strings.Builder
	for _, f := range frames {
		b.WriteString("data: " + f + "\n\n")
	}
	return b.String()
}

const (
	chunkRole = `{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`
	chunkPong = `{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"pong"}}]}`
	chunkStop = `{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`
	chunkUse  = `{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`
)

func TestParseStreamHappyPath(t *testing.T) {
	s, err := ParseStream(strings.NewReader(sse(chunkRole, chunkPong, chunkStop, chunkUse, "[DONE]")))
	require.NoError(t, err)

	assert.Equal(t, "pong", s.Text)
	assert.Equal(t, "stop", s.FinishReason)
	assert.Equal(t, "m", s.Model)
	assert.True(t, s.SawDone)
	assert.True(t, s.HasUsage)
	assert.Equal(t, Usage{5, 1, 6}, s.Usage)
	assert.Empty(t, s.StreamInvariants(false))

	env := s.Envelope()
	assert.Equal(t, ContentText, env.ContentKind)
	assert.Equal(t, "assistant", env.Role)
	assert.Equal(t, "stop", env.FinishReason)
}

func TestStreamInvariantsCatchMissingDone(t *testing.T) {
	s, err := ParseStream(strings.NewReader(sse(chunkRole, chunkPong, chunkStop)))
	require.NoError(t, err)
	problems := strings.Join(s.StreamInvariants(false), "; ")
	assert.Contains(t, problems, "[DONE]")
}

func TestStreamInvariantsCatchFramesAfterDone(t *testing.T) {
	s, err := ParseStream(strings.NewReader(sse(chunkRole, chunkPong, chunkStop, "[DONE]", chunkPong)))
	require.NoError(t, err)
	assert.Equal(t, 1, s.FramesAfterDone)
	assert.Contains(t, strings.Join(s.StreamInvariants(false), "; "), "after [DONE]")
}

func TestStreamInvariantsCatchRepeatedRole(t *testing.T) {
	s, err := ParseStream(strings.NewReader(sse(chunkRole, chunkRole, chunkStop, "[DONE]")))
	require.NoError(t, err)
	assert.Contains(t, strings.Join(s.StreamInvariants(false), "; "), "repeated")
}

func TestStreamInvariantsCatchMissingRole(t *testing.T) {
	s, err := ParseStream(strings.NewReader(sse(chunkPong, chunkStop, "[DONE]")))
	require.NoError(t, err)
	assert.Contains(t, strings.Join(s.StreamInvariants(false), "; "), "delta.role")
}

func TestStreamReassemblesToolCallArguments(t *testing.T) {
	frames := []string{
		chunkRole,
		`{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
		`{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`,
		`{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Wellington\"}"}}]}}]}`,
		`{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		"[DONE]",
	}
	s, err := ParseStream(strings.NewReader(sse(frames...)))
	require.NoError(t, err)

	require.Len(t, s.ToolCalls, 1)
	call := s.ToolCalls[0]
	assert.Equal(t, "get_weather", call.Name)
	assert.Equal(t, "call_1", call.ID)
	assert.JSONEq(t, `{"city":"Wellington"}`, call.Arguments)
	assert.Empty(t, s.StreamInvariants(true))

	assert.Equal(t, ContentToolCalls, s.Envelope().ContentKind)
}

func TestStreamInvariantsCatchTruncatedToolArguments(t *testing.T) {
	frames := []string{
		chunkRole,
		`{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"f","arguments":"{\"city\":"}}]}}]}`,
		`{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		"[DONE]",
	}
	s, err := ParseStream(strings.NewReader(sse(frames...)))
	require.NoError(t, err)
	assert.Contains(t, strings.Join(s.StreamInvariants(true), "; "), "valid JSON")
}

// A tool-call argument or an inline image easily exceeds bufio.Scanner's 64KB
// default, and the failure looks like malformed vendor JSON rather than a
// harness bug. Pin the larger buffer.
func TestParseStreamHandlesVeryLargeFrames(t *testing.T) {
	big := strings.Repeat("x", 300_000)
	frame := `{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"` + big + `"}}]}`
	s, err := ParseStream(strings.NewReader(sse(frame, chunkStop, "[DONE]")))
	require.NoError(t, err)
	assert.Len(t, s.Text, len(big))
}

func TestParseStreamIgnoresCommentsAndEventLines(t *testing.T) {
	raw := ": keepalive\n\nevent: message\ndata: " + chunkRole + "\n\ndata: " + chunkStop + "\n\ndata: [DONE]\n\n"
	s, err := ParseStream(strings.NewReader(raw))
	require.NoError(t, err)
	assert.True(t, s.SawDone)
	assert.Len(t, s.DataFrames(), 2)
}

func TestNormalizeAcrossFormatsProducesComparableEnvelopes(t *testing.T) {
	openAI, err := Normalize(FormatOpenAICompletion, []byte(validCompletion))
	require.NoError(t, err)

	anthropic, err := Normalize(FormatAnthropicMessage, []byte(`{
      "id":"msg_1","type":"message","role":"assistant","model":"test-model",
      "stop_reason":"end_turn",
      "content":[{"type":"text","text":"pong"}],
      "usage":{"input_tokens":10,"output_tokens":2}
    }`))
	require.NoError(t, err)

	gemini, err := Normalize(FormatGeminiGenerate, []byte(`{
      "modelVersion":"test-model",
      "candidates":[{"finishReason":"STOP","content":{"role":"model","parts":[{"text":"pong"}]}}],
      "usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2,"totalTokenCount":12}
    }`))
	require.NoError(t, err)

	// The point of the Envelope: three different wire formats reduce to the
	// same shape, so a surface that mangles one is immediately visible.
	assert.Empty(t, openAI.Diff(anthropic, "openai", "anthropic"))
	assert.Empty(t, openAI.Diff(gemini, "openai", "gemini"))
	assert.Equal(t, Usage{10, 2, 12}, anthropic.Usage)
}

func TestEnvelopeDiffReportsDroppedUsage(t *testing.T) {
	withUsage, err := Normalize(FormatOpenAICompletion, []byte(validCompletion))
	require.NoError(t, err)

	noUsage, err := Normalize(FormatOpenAICompletion, []byte(`{
      "id":"1","object":"chat.completion","created":1,"model":"test-model",
      "choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"pong"}}]
    }`))
	require.NoError(t, err)

	diff := withUsage.Diff(noUsage, "shim", "universal")
	require.Len(t, diff, 1)
	assert.Contains(t, diff[0], "has_usage")
}

func TestEnvelopeDiffToleratesQualifiedModelNames(t *testing.T) {
	a := Envelope{Role: "assistant", ContentKind: ContentText, FinishReason: "stop", Model: "claude-sonnet-4-6"}
	b := Envelope{Role: "assistant", ContentKind: ContentText, FinishReason: "stop", Model: "claude-sonnet-4-6-20260101"}
	assert.Empty(t, a.Diff(b, "a", "b"))
}

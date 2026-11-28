package googleai

// Tyk: tests for thought_signature.go.

import (
	"strings"
	"testing"

	"github.com/google/generative-ai-go/genai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

func TestSignatureIDRoundTrip(t *testing.T) {
	sig := "c2ln+/=\x00binary"
	id := EncodeSignatureID(sig)
	assert.NotEqual(t, id, EncodeSignatureID(sig), "unique per call")
	got, ok := DecodeSignatureID(id)
	require.True(t, ok)
	assert.Equal(t, sig, got)

	oversize := EncodeSignatureID(strings.Repeat("x", MaxThoughtSignatureBytes+1))
	atCap := EncodeSignatureID(strings.Repeat("x", MaxThoughtSignatureBytes))
	_, ok = DecodeSignatureID(atCap)
	assert.True(t, ok, "a signature at the cap is kept")
	for _, other := range []string{"", "call_123", "call_gs1_", "call_gs1_abcd", "call_gs1_abcd_!!!", "toolu_01",
		"call_gs1_abcdefgh_c2ln", "call_gs1_0123456_c2ln", "call_gs1_01234567_c2ln=", "call_gs1_01234567_c2l+", oversize} {
		_, ok := DecodeSignatureID(other)
		assert.False(t, ok, other)
	}
}

// The args key the transport sets becomes the tool call's ID, and the ID
// becomes the args key again when the call is sent back.
func TestThoughtSignatureThroughTheDriver(t *testing.T) {
	resp, err := convertCandidates([]*genai.Candidate{{Content: &genai.Content{Role: "model", Parts: []genai.Part{
		genai.FunctionCall{Name: "f", Args: map[string]any{"x": 1.0, ThoughtSignatureArg: "SIG"}},
		genai.FunctionCall{Name: "g", Args: map[string]any{"y": 2.0}},
	}}}}, nil)
	require.NoError(t, err)
	calls := resp.Choices[0].ToolCalls
	require.Len(t, calls, 2)
	assert.JSONEq(t, `{"x":1}`, calls[0].FunctionCall.Arguments)
	sig, ok := DecodeSignatureID(calls[0].ID)
	require.True(t, ok)
	assert.Equal(t, "SIG", sig)
	assert.Empty(t, calls[1].ID)

	parts, err := convertParts([]llms.ContentPart{calls[0], calls[1]})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"x": 1.0, ThoughtSignatureArg: "SIG"}, parts[0].(genai.FunctionCall).Args)
	assert.Equal(t, map[string]any{"y": 2.0}, parts[1].(genai.FunctionCall).Args)
}

func TestShortSignatureID(t *testing.T) {
	id := EncodeSignatureID(strings.Repeat("s", 4096))
	short := ShortSignatureID(id)
	assert.Len(t, short, len("call_gs1_")+8)
	assert.True(t, strings.HasPrefix(id, short+"_"))
	assert.Equal(t, short, ShortSignatureID(id), "deterministic")
	for _, other := range []string{"call_123", "toolu_01", "call_gs1_abc_x", ""} {
		assert.Equal(t, other, ShortSignatureID(other))
	}
}

// A value a client put under the reserved key in a call's arguments is
// treated as a signature only if usable; an ID's signature wins over it.
func TestThoughtSignatureClientSuppliedKey(t *testing.T) {
	call := func(id, args string) llms.ContentPart {
		return llms.ToolCall{ID: id, FunctionCall: &llms.FunctionCall{Name: "f", Arguments: args}}
	}
	parts, err := convertParts([]llms.ContentPart{
		call("call_1", `{"x":1,"`+ThoughtSignatureArg+`":"CLIENT"}`),
		call("call_2", `{"`+ThoughtSignatureArg+`":42}`),
		call("call_3", `{"`+ThoughtSignatureArg+`":"`+strings.Repeat("s", MaxThoughtSignatureBytes+1)+`"}`),
		call(EncodeSignatureID("FROM_ID"), `{"`+ThoughtSignatureArg+`":"CLIENT"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, "CLIENT", parts[0].(genai.FunctionCall).Args[ThoughtSignatureArg])
	assert.NotContains(t, parts[1].(genai.FunctionCall).Args, ThoughtSignatureArg, "not a string")
	assert.NotContains(t, parts[2].(genai.FunctionCall).Args, ThoughtSignatureArg, "over the cap")
	assert.Equal(t, "FROM_ID", parts[3].(genai.FunctionCall).Args[ThoughtSignatureArg])
}

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

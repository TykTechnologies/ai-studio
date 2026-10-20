package agenttrace

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An edge event's attributes keep only the gateway's namespaces, bounded
// (jyx.160).
func TestFromProtoBoundsAttributes(t *testing.T) {
	attrs := map[string]string{
		"mcp.method.name":            "tools/call",
		"http.request.method":        "POST",
		"gen_ai.x":                   strings.Repeat("v", 5000),
		"evil.key":                   "x",
		strings.Repeat("tyk.k", 100): "long key",
	}
	for i := 0; i < 100; i++ {
		attrs["tyk.a"+strings.Repeat("0", i%3)+string(rune('a'+i%26))+string(rune('a'+i/26))] = "v"
	}
	ev := &Event{Kind: KindToolCall, SpanID: NewSpanID(), Attributes: attrs}
	got := FromProto(ToProto(ev)).Attributes
	assert.LessOrEqual(t, len(got), maxAttributes)
	assert.NotContains(t, got, "evil.key")
	assert.NotContains(t, got, strings.Repeat("tyk.k", 100))
	assert.Len(t, got["gen_ai.x"], maxAttributeValue)
	assert.Equal(t, "tools/call", got["mcp.method.name"])
	assert.Equal(t, "POST", got["http.request.method"])
	assert.Nil(t, boundAttributes(nil))
}

// An edge's extract lists are bounded again when the hub decodes them
// (jyx.159).
func TestFromProtoBoundsExtracts(t *testing.T) {
	long := strings.Repeat("n", 1000)
	ev := &Event{Kind: KindLLMInference, SpanID: NewSpanID(), Extracts: Extracts{
		ResponseID:    strings.Repeat("r", 300),
		FinishReasons: make([]string, 40),
	}}
	for i := 0; i < 2*maxExtractOffered; i++ {
		ev.Extracts.OfferedTools = append(ev.Extracts.OfferedTools, long)
	}
	for i := 0; i < maxExtractCalls+10; i++ {
		ev.Extracts.ToolCalls = append(ev.Extracts.ToolCalls, ToolCallRequest{ID: strings.Repeat("i", 300), Name: long})
		ev.Extracts.ToolResults = append(ev.Extracts.ToolResults, ToolResult{ID: "ok"})
	}
	got := FromProto(ToProto(ev))
	require.NotNil(t, got)
	x := got.Extracts
	assert.Empty(t, x.ResponseID, "an over-long id is dropped")
	assert.Len(t, x.OfferedTools, maxExtractOffered)
	assert.Len(t, x.OfferedTools[0], maxExtractNameBytes)
	assert.Len(t, x.ToolCalls, maxExtractCalls)
	assert.Empty(t, x.ToolCalls[0].ID)
	assert.Len(t, x.ToolCalls[0].Name, maxExtractNameBytes)
	assert.Len(t, x.ToolResults, maxExtractCalls)
	assert.Len(t, x.FinishReasons, maxExtractReasons)
}

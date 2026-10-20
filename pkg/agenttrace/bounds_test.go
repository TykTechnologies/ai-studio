package agenttrace

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The hub keeps exactly the attributes a capture writes, with valid
// values: an edge cannot preset the hub's own statements (attribution, the
// offered tool set) or put content in attributes (jyx.160, jyx.167).
func TestFromProtoAllowlistsAttributes(t *testing.T) {
	assert.Equal(t, []string{
		"http.request.method", "mcp.method.name", "mcp.protocol.version",
		"tyk.content.withheld", "tyk.server_tool_calls",
	}, CaptureAttributeKeys(), "a new capture attribute must be added here and to the allowlist on purpose")

	attrs := map[string]string{
		"mcp.method.name":            "tools/call",
		"mcp.protocol.version":       "2026-07-28",
		"http.request.method":        "POST",
		"tyk.server_tool_calls":      "2",
		"tyk.content.withheld":       WithheldEdgeLinkUnverifiedTLS,
		"tyk.attribution.rule":       "catalogue_unique",
		"tyk.attribution.confidence": "exact",
		"tyk.tools.offered":          `["x"]`,
		"tyk.tool_calls.not_offered": `["y"]`,
		"tyk.tool.in_catalogue":      "true",
		"gen_ai.input.messages":      "secret prompt",
		"evil.key":                   "x",
	}
	ev := &Event{Kind: KindToolCall, SpanID: NewSpanID(), Attributes: attrs}
	got := FromProto(ToProto(ev)).Attributes
	assert.Equal(t, map[string]string{
		"mcp.method.name":       "tools/call",
		"mcp.protocol.version":  "2026-07-28",
		"http.request.method":   "POST",
		"tyk.server_tool_calls": "2",
		"tyk.content.withheld":  WithheldEdgeLinkUnverifiedTLS,
	}, got)

	bad := map[string]string{
		"mcp.method.name":       strings.Repeat("m", 200),
		"http.request.method":   "post; drop",
		"tyk.server_tool_calls": "many",
		"tyk.content.withheld":  "because I said so",
	}
	assert.Nil(t, boundAttributes(bad), "invalid values are dropped")
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

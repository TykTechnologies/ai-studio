package agenttrace

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

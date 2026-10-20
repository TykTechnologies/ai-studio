package openaiVendor

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A streamed response's tool calls are counted once each, whichever chunks
// their fragments arrive in (bead jyx.15: the count used to look only at the
// finish chunk, whose delta is empty, and recorded 0).
func TestAnalyzeStreamingResponse_ToolCount(t *testing.T) {
	stream, err := os.ReadFile(filepath.Join("..", "..", "testdata", "agent_tracing", "streams", "openai_chat_parallel_stream.sse"))
	require.NoError(t, err)
	r := httptest.NewRequest("POST", "/llm/stream/x/v1/chat/completions", nil)
	_, _, resp, err := (&OpenAI{}).AnalyzeStreamingResponse(&models.LLM{}, &models.App{}, 200, stream, r, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, resp.GetToolCount())

	legacy := []byte(`data: {"choices":[{"index":0,"delta":{"function_call":{"name":"get_time","arguments":""}}}]}

data: {"choices":[{"index":0,"delta":{"function_call":{"arguments":"{}"}}}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"function_call"}]}

data: [DONE]
`)
	_, _, resp, err = (&OpenAI{}).AnalyzeStreamingResponse(&models.LLM{}, &models.App{}, 200, legacy, r, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, resp.GetToolCount())
}

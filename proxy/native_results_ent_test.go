//go:build enterprise

package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// A response filter runs on every result's message. A blocked result is
// replaced by an errored result for the same custom_id; the other lines are
// untouched, and the blocked request's usage is still billed (the vendor
// charged it).
func TestNativeResults_ResponseFilterReplacesBlockedLine(t *testing.T) {
	const script = `
text := import("text")
output := {block: text.contains(input.raw_input, "answer b"), message: "withheld by policy"}
`
	h := newNativeHarness(t, nativeHarnessOpts{mutateLLM: func(l *models.LLM) {
		l.Filters = []*models.Filter{{Name: "withhold-b", Script: []byte(script), ResponseFilter: true}}
	}})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	h.vendor.addBatch("msgbatch_F1", "ended")
	lines := []string{succeededLine("a", "claude-luna", 10, 1), succeededLine("b", "claude-luna", 10, 1), erroredLine, succeededLine("c", "claude-luna", 10, 1)}
	h.vendor.setResults("msgbatch_F1", lines...)
	h.seedBatchRow(h.llm, h.appA, "msgbatch_F1")

	resp, body := h.do(h.keyA, http.MethodGet, resultsPath(h.wrap(h.llm, h.appA, "msgbatch_F1")), "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	got := strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
	require.Len(t, got, 4)
	assert.Equal(t, lines[0], got[0])
	assert.Equal(t, lines[2], got[2])
	assert.Equal(t, lines[3], got[3])
	assert.NotContains(t, got[1], "answer b")
	var blocked struct {
		CustomID string `json:"custom_id"`
		Result   struct {
			Type  string `json:"type"`
			Error struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"error"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(got[1]), &blocked), got[1])
	assert.Equal(t, "b", blocked.CustomID)
	assert.Equal(t, "errored", blocked.Result.Type)
	assert.Contains(t, blocked.Result.Error.Error.Message, "withheld by policy")

	require.Eventually(t, func() bool {
		recs := h.chatRecords(h.appA)
		return len(recs) == 1 && recs[0].PromptTokens == 30
	}, 5*time.Second, 50*time.Millisecond, "the blocked request is still billed")
}

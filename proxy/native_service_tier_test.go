package proxy

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Batch usage is recorded with its service tier on the chat record and the
// proxy log; list-price traffic has none.
func TestNative_ServiceTierRecorded(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	h.vendor.addBatch("msgbatch_T1", "ended")
	h.vendor.setResults("msgbatch_T1", succeededLine("a", "claude-luna", 1000, 200))
	row := h.seedBatchRow(h.llm, h.appA, "msgbatch_T1")
	out, err := h.proxy.accountNativeRow(context.Background(), row)
	require.NoError(t, err)
	require.Equal(t, nativeAccountAccounted, out)

	// appB: a synchronous Messages call at list price.
	resp, body := h.do(h.keyB, http.MethodPost, "/llm/rest/claude/v1/messages",
		`{"model":"claude-luna","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	var batchRecs, syncRecs []models.LLMChatRecord
	require.Eventually(t, func() bool {
		batchRecs, syncRecs = h.chatRecords(h.appA), h.chatRecords(h.appB)
		return len(batchRecs) == 1 && len(syncRecs) == 1
	}, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, "batch", batchRecs[0].ServiceTier)
	assert.Equal(t, "", syncRecs[0].ServiceTier)

	assert.Equal(t, "batch", h.proxyLogs(h.appA, 1)[0].ServiceTier)
	assert.Equal(t, "", h.proxyLogs(h.appB, 1)[0].ServiceTier)
}

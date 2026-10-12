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

// A pinned workspace reaches Anthropic on every call made for the LLM:
// synchronous Messages calls (rest, call, stream), native batch operations
// and the reconciler's own polls and downloads. A client's workspace and
// user-profile headers never do.
func TestNative_WorkspacePinnedOnEveryUpstreamCall(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{mutateLLM: func(l *models.LLM) {
		l.VendorWorkspaceID = "wrkspc_pinned"
	}})
	h.vendor.addBatch("msgbatch_A1", "ended")
	h.vendor.setResults("msgbatch_A1", batchLines()...)
	// Seeded before the Messages calls, whose analytics writes would hold
	// the SQLite table lock.
	row := h.seedBatchRow(h.llm, h.appA, "msgbatch_A1")
	clientHeaders := []string{"anthropic-workspace-id", "wrkspc_client", "anthropic-user-profile-id", "prof_client"}
	msg := `{"model":"claude-luna","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`
	streamMsg := `{"model":"claude-luna","max_tokens":10,"stream":true,"messages":[{"role":"user","content":"hi"}]}`

	for _, mode := range []string{"rest", "call"} {
		resp, body := h.do(h.keyA, http.MethodPost, "/llm/"+mode+"/claude/v1/messages", msg, clientHeaders...)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	}
	resp, body := h.do(h.keyA, http.MethodPost, "/llm/stream/claude/v1/messages", streamMsg, clientHeaders...)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	id := h.wrap(h.llm, h.appA, "msgbatch_A1")
	resp, body = h.do(h.keyA, http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+id, "", clientHeaders...)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	// Let the Messages calls' analytics land before the accounting writes.
	require.Eventually(t, func() bool { return len(h.chatRecords(h.appA)) == 3 }, 5*time.Second, 20*time.Millisecond)
	_, err := h.proxy.accountNativeRow(context.Background(), row)
	require.NoError(t, err)

	calls := h.vendor.recorded()
	require.Len(t, h.vendor.callsTo(http.MethodPost, "/v1/messages"), 3)
	require.NotEmpty(t, h.vendor.callsTo(http.MethodGet, "/v1/messages/batches/msgbatch_A1/results"), "the reconciler downloaded the results")
	for _, c := range calls {
		assert.Equal(t, []string{"wrkspc_pinned"}, c.Headers.Values("anthropic-workspace-id"), "%s %s", c.Method, c.Path)
		assert.Empty(t, c.Headers.Values("anthropic-user-profile-id"), "%s %s", c.Method, c.Path)
	}
}

// Without a pin no workspace header reaches Anthropic, whatever the client
// sends: calls run in the API key's workspace.
func TestNative_ClientWorkspaceDroppedWithoutPin(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_A1", "ended")
	clientHeaders := []string{"anthropic-workspace-id", "wrkspc_client", "anthropic-user-profile-id", "prof_client"}

	resp, body := h.do(h.keyA, http.MethodPost, "/llm/rest/claude/v1/messages",
		`{"model":"claude-luna","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`, clientHeaders...)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	resp, body = h.do(h.keyA, http.MethodPost, "/llm/stream/claude/v1/messages",
		`{"model":"claude-luna","max_tokens":10,"stream":true,"messages":[{"role":"user","content":"hi"}]}`, clientHeaders...)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	resp, body = h.do(h.keyA, http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+h.wrap(h.llm, h.appA, "msgbatch_A1"), "", clientHeaders...)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	calls := h.vendor.recorded()
	require.Len(t, calls, 3)
	for _, c := range calls {
		assert.Empty(t, c.Headers.Values("anthropic-workspace-id"), "%s %s", c.Method, c.Path)
		assert.Empty(t, c.Headers.Values("anthropic-user-profile-id"), "%s %s", c.Method, c.Path)
	}
}

package proxy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

const countTokensPath = "/llm/call/claude/v1/messages/count_tokens"

// count_tokens is admitted like a synchronous call (model allow-list) and
// logged without a chat record.
func TestNativeCountTokens_AllowListAndLogOnly(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{mutateLLM: func(l *models.LLM) { l.AllowedModels = []string{"^claude-luna$"} }})

	body := `{"model":"claude-luna","messages":[{"role":"user","content":"hi"}]}`
	resp, out := h.do(h.keyA, http.MethodPost, countTokensPath+"?beta=true", body,
		"anthropic-version", "2023-06-01", "anthropic-beta", "token-counting-2024-11-01",
		"anthropic-workspace-id", "wrkspc_other")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	assert.JSONEq(t, `{"input_tokens":42}`, string(out))
	calls := h.vendor.callsTo(http.MethodPost, "/v1/messages/count_tokens")
	require.Len(t, calls, 1)
	assert.Equal(t, body, string(calls[0].Body), "an unfiltered body is forwarded byte for byte")
	assert.Equal(t, "beta=true", calls[0].RawQuery)
	assert.Equal(t, "token-counting-2024-11-01", calls[0].Headers.Get("anthropic-beta"))
	assert.Empty(t, calls[0].Headers.Get("anthropic-workspace-id"))
	assert.Equal(t, "anthropic-vendor-key", calls[0].Headers.Get("x-api-key"))

	// A model off the allow-list never reaches the vendor.
	resp, out = h.do(h.keyA, http.MethodPost, countTokensPath, `{"model":"claude-sol","messages":[]}`)
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "%s", out)
	typ, msg := nativeErr(t, out)
	assert.Equal(t, "permission_error", typ)
	assert.Equal(t, "model 'claude-sol' is not allowed", msg)

	// Malformed bodies, duplicate keys included, are refused.
	for _, b := range []string{`{"messages":[]}`, `nope`, `{"model":"claude-luna","model":"claude-sol"}`,
		`{"model":"claude-luna","messages":[{"role":"user","content":"a","Content":"b"}]}`} {
		resp, out = h.do(h.keyA, http.MethodPost, countTokensPath, b)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", b, out)
		_, msg := nativeErr(t, out)
		assert.Contains(t, msg, "invalid count_tokens body")
	}
	assert.Len(t, h.vendor.callsTo(http.MethodPost, "/v1/messages/count_tokens"), 1)

	assert.Empty(t, h.chatRecords(h.appA), "counting tokens costs nothing")
	logs := h.proxyLogs(h.appA, 6)
	require.Len(t, logs, 6, "one proxy log per call")
	assert.Equal(t, http.StatusOK, logs[0].ResponseCode)
	assert.Equal(t, body, logs[0].RequestBody)
	assert.Equal(t, `{"input_tokens":42}`, logs[0].ResponseBody)
	assert.Equal(t, http.StatusForbidden, logs[1].ResponseCode)
}

// count_tokens touches no vendor resource: it needs no vendor credential for
// an ID codec.
func TestNativeCountTokens_NoResourcePrerequisites(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{vendorKey: "-"})
	resp, out := h.do(h.keyA, http.MethodPost, countTokensPath, `{"model":"claude-luna","messages":[]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)

	// Another App without the LLM is refused by the ACL as before.
	resp, _ = h.do(h.keyC, http.MethodPost, countTokensPath, `{"model":"claude-luna","messages":[]}`)
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
}

// Only the exact method and path are native; a trailing slash stays on the
// legacy chain, and GET is the vendor-shaped 405.
func TestNativeCountTokens_ExactMatch(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	resp, out := h.do(h.keyA, http.MethodGet, countTokensPath, "")
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, "%s", out)
	assert.Equal(t, "POST", resp.Header.Get("Allow"))
}

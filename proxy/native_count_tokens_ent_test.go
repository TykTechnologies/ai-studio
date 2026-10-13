//go:build enterprise

package proxy

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Blocks a request mentioning "forbidden"; the script context has no batch
// fields for a single-body operation.
const countTokensBlockScript = `
text := import("text")
last := input.messages[len(input.messages)-1]
output := {block: text.contains(last.content, "forbidden"), message: "blocked; custom_id=" + string(is_undefined(input.context.custom_id))}
`

// The request filters run on a count_tokens body as on a synchronous call:
// a redaction is forwarded, a block refuses the call before the vendor.
func TestNativeCountTokens_Filters(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{mutateLLM: withNativeFilters(nativeRedactScript, countTokensBlockScript)})

	resp, out := h.do(h.keyA, http.MethodPost, countTokensPath,
		`{"model":"claude-luna","messages":[{"role":"user","content":"my SECRET"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	calls := h.vendor.callsTo(http.MethodPost, "/v1/messages/count_tokens")
	require.Len(t, calls, 1)
	assert.Contains(t, string(calls[0].Body), "[REDACTED]")
	assert.NotContains(t, string(calls[0].Body), "SECRET")

	resp, out = h.do(h.keyA, http.MethodPost, countTokensPath,
		`{"model":"claude-luna","messages":[{"role":"user","content":"something forbidden"}]}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", out)
	typ, msg := nativeErr(t, out)
	assert.Equal(t, "invalid_request_error", typ)
	assert.True(t, strings.HasPrefix(msg, "request refused: "), msg)
	assert.Contains(t, msg, "custom_id=true", "no batch fields in the script context")
	assert.Len(t, h.vendor.callsTo(http.MethodPost, "/v1/messages/count_tokens"), 1)
}

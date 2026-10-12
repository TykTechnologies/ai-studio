//go:build enterprise

package proxy

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Redacts SECRET in every message.
const nativeRedactScript = `
text := import("text")
msgs := []
for m in input.messages {
    msgs = append(msgs, {role: m.role, content: text.replace(m.content, "SECRET", "[REDACTED]", -1)})
}
output := {block: false, messages: msgs}
`

// Blocks the request whose last message mentions "forbidden", naming its
// custom_id from the script context.
const nativeBlockScript = `
text := import("text")
last := input.messages[len(input.messages)-1]
output := {block: text.contains(last.content, "forbidden"), message: "no forbidden in " + input.context.custom_id}
`

func withNativeFilters(scripts ...string) func(*models.LLM) {
	return func(l *models.LLM) {
		for i, s := range scripts {
			l.Filters = append(l.Filters, &models.Filter{Name: "native-filter-" + string(rune('a'+i)), Script: []byte(s)})
		}
	}
}

// A filter that blocks one request refuses the batch, naming the request;
// nothing reaches the vendor.
func TestNativeCreate_FilterBlocksOneItem(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{mutateLLM: withNativeFilters(nativeBlockScript)})
	body := `{"requests":[` +
		`{"custom_id":"fine","params":{"model":"claude-luna","max_tokens":5,"messages":[{"role":"user","content":"hello"}]}},` +
		`{"custom_id":"bad-one","params":{"model":"claude-luna","max_tokens":5,"messages":[{"role":"user","content":"something forbidden"}]}}]}`
	resp, out := h.do(h.keyA, http.MethodPost, batchesPath, body)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
	typ, msg := nativeErr(t, out)
	assert.Equal(t, "invalid_request_error", typ)
	assert.Contains(t, msg, `"bad-one"`)
	assert.Contains(t, msg, "no forbidden in bad-one", "the script saw the item's custom_id")
	assert.Empty(t, h.vendor.recorded())
}

// A redacting filter rewrites only the request it changed; the rest of the
// forwarded body is byte for byte what the client sent.
func TestNativeCreate_RedactionRewritesOnlyChangedItems(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{mutateLLM: withNativeFilters(nativeRedactScript, nativeBlockScript)})
	untouched := `{"model": "claude-luna",  "max_tokens": 5, "messages": [ {"role":"user", "content":"plain"} ]}`
	body := `{"requests":[` +
		`{"custom_id":"keep","params":` + untouched + `},` +
		`{"custom_id":"redact","params":{"model":"claude-luna","max_tokens":5,"messages":[{"role":"user","content":"my SECRET plan"}]}}],` +
		` "extra" : true }`
	resp, out := h.do(h.keyA, http.MethodPost, batchesPath, body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

	calls := h.vendor.callsTo(http.MethodPost, "/v1/messages/batches")
	require.Len(t, calls, 1)
	sent := string(calls[0].Body)
	assert.NotContains(t, sent, "SECRET")
	assert.Contains(t, sent, "[REDACTED]")
	assert.True(t, strings.HasPrefix(sent, `{"requests":[{"custom_id":"keep","params":`+untouched+`},{"custom_id":"redact","params":`), sent)
	assert.True(t, strings.HasSuffix(sent, `}], "extra" : true }`), sent)

	// The proxy log holds the body the filters let through.
	logs := h.proxyLogs(h.appA)
	require.Len(t, logs, 1)
	assert.NotContains(t, logs[0].RequestBody, "SECRET")
}

// With request filters configured, a batch over the item cap is refused
// with a message that says why.
func TestNativeCreate_FilteredItemCap(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{
		config:    &Config{NativeMaxFilteredItems: 2},
		mutateLLM: withNativeFilters(nativeBlockScript),
	})
	resp, out := h.do(h.keyA, http.MethodPost, batchesPath, batchCreateBody("claude-luna", "claude-luna", "claude-luna"))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
	_, msg := nativeErr(t, out)
	assert.Contains(t, msg, "at most 2 requests")
	assert.Empty(t, h.vendor.recorded())

	resp, out = h.do(h.keyA, http.MethodPost, batchesPath, batchCreateBody("claude-luna", "claude-luna"))
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
}

// Many items run through the filter pool; every one is screened.
func TestNativeCreate_ManyItemsAllScreened(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{mutateLLM: withNativeFilters(nativeBlockScript)})
	var reqs []string
	for i := 0; i < 60; i++ {
		content := "ok"
		if i == 47 {
			content = "forbidden"
		}
		reqs = append(reqs, `{"custom_id":"r`+strings.Repeat("x", i%3)+string(rune('A'+i%26))+`-`+strconv.Itoa(i)+`","params":{"model":"claude-luna","max_tokens":1,"messages":[{"role":"user","content":"`+content+`"}]}}`)
	}
	resp, out := h.do(h.keyA, http.MethodPost, batchesPath, `{"requests":[`+strings.Join(reqs, ",")+`]}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
	assert.Contains(t, string(out), "-47")
	assert.Empty(t, h.vendor.recorded())
}

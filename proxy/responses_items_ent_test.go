//go:build enterprise

package proxy

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Text in built-in input items (MCP results, code interpreter logs,
// reasoning summaries) is screened like message text, on the synchronous,
// streamed and batch paths; an unknown item type is refused when filters are
// attached.
func TestResponsesFilters_BuiltInItemsBlocked(t *testing.T) {
	t.Parallel()
	items := map[string]string{
		"mcp output":       `{"type":"mcp_call","id":"mcp_1","server_label":"s","name":"t","arguments":"{}","output":"forbidden result"}`,
		"code logs":        `{"type":"code_interpreter_call","id":"ci_1","code":"x","outputs":[{"type":"logs","logs":"forbidden logs"}]}`,
		"reasoning":        `{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"forbidden thought"}]}`,
		"unknown item":     `{"type":"brand_new_item","text":"harmless"}`,
		"unknown item 2":   `{"type":"brand_new_item","text":"forbidden"}`,
		"file search text": `{"type":"file_search_call","id":"fs_1","queries":["q"],"results":[{"file_id":"f","text":"forbidden"}]}`,
	}

	h := newResponsesHarness(t)
	// Without filters every item is the vendor's to judge.
	for name, item := range items {
		resp, out := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":[`+item+`]}`)
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", name, out)
	}

	withPrimaryFilters(t, h, responsesBlockScript)
	before := len(h.api.recorded())
	for name, item := range items {
		for _, p := range []struct{ path, extra string }{
			{"/llm/rest/primary/v1/responses", ""},
			{"/llm/call/primary/v1/responses", `"stream":true,`},
		} {
			resp, out := h.do(h.keyC, http.MethodPost, p.path, `{`+p.extra+`"model":"gpt-4o","input":[`+item+`]}`)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s %s: %s", name, p.path, out)
			if strings.HasPrefix(name, "unknown item") {
				assert.Contains(t, string(out), "could not be read for screening", name)
			} else {
				assert.Contains(t, string(out), "forbidden content", name)
			}
		}
	}
	assert.Len(t, h.api.recorded(), before, "nothing reached the vendor")
}

// Batch lines carry the same items.
func TestResponsesFilters_BuiltInItemsInBatchLines(t *testing.T) {
	t.Parallel()
	h := newBatchesHarness(t)
	withPrimaryFilters(t, h, responsesBlockScript)
	for name, item := range map[string]string{
		"mcp output": `{"type":"mcp_call","id":"m","arguments":"{}","output":"forbidden result"}`,
		"unknown":    `{"type":"brand_new_item","text":"x"}`,
	} {
		line := `{"custom_id":"r1","method":"POST","url":"/v1/responses","body":{"model":"gpt-4o","input":[` + item + `]}}`
		resp, out := h.uploadFile(h.keyC, line)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", name, out)
		_, msg := oaiErr(t, out)
		assert.Contains(t, msg, `"r1"`, name)
	}
	assert.Empty(t, h.api.callsTo(http.MethodPost, "/v1/files"))
}

// A redacting filter rewrites built-in item text in place.
func TestResponsesFilters_BuiltInItemsRedacted(t *testing.T) {
	t.Parallel()
	h := newResponsesHarness(t)
	withPrimaryFilters(t, h, nativeRedactScript)
	resp, out := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses",
		`{"model":"gpt-4o","input":[{"type":"mcp_call","id":"mcp_1","arguments":"{}","output":"SECRET result"},`+
			`{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"my SECRET"}]}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	creates := h.api.callsTo(http.MethodPost, "/v1/responses")
	require.Len(t, creates, 1)
	in := decodeObj(t, creates[0].Body)["input"].([]any)
	assert.Equal(t, "[REDACTED] result", in[0].(map[string]any)["output"])
	assert.Equal(t, "mcp_1", in[0].(map[string]any)["id"])
	assert.Equal(t, "my [REDACTED]", in[1].(map[string]any)["summary"].([]any)[0].(map[string]any)["text"])
}

//go:build enterprise

package proxy

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Blocks a request with "forbidden" in any message.
const responsesBlockScript = `
text := import("text")
hit := false
for m in input.messages {
    if text.contains(m.content, "forbidden") { hit = true }
}
output := {block: hit, message: "forbidden content"}
`

func withPrimaryFilters(t *testing.T, h *responsesHarness, scripts ...string) {
	t.Helper()
	h.settle() // the shared-cache SQLite refuses a write that overlaps the analytics writer
	for i, s := range scripts {
		h.primary.Filters = append(h.primary.Filters, &models.Filter{Name: "responses-filter-" + string(rune('a'+i)), Script: []byte(s)})
	}
	require.NoError(t, h.db.Save(h.primary).Error)
	require.NoError(t, h.proxy.loadResources())
}

// The request filters see the Responses API prompt: instructions, a string
// input, message items, tool outputs and prompt variables, on every path
// that carries one.
func TestResponsesFilters_BlockOnEveryPath(t *testing.T) {
	t.Parallel()
	h := newResponsesHarness(t)
	withPrimaryFilters(t, h, responsesBlockScript)

	bodies := map[string]string{
		"string input":  `{"model":"gpt-4o","input":"something forbidden"}`,
		"instructions":  `{"model":"gpt-4o","instructions":"forbidden rules","input":"hi"}`,
		"message parts": `{"model":"gpt-4o","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"ok"},{"type":"input_text","text":"forbidden"}]}]}`,
		"tool output":   `{"model":"gpt-4o","input":[{"type":"function_call_output","call_id":"c","output":"forbidden data"}]}`,
		"prompt vars":   `{"model":"gpt-4o","prompt":{"id":"pmpt_1","variables":{"q":"forbidden"}}}`,
	}
	paths := []struct{ path, extra string }{
		{"/llm/rest/primary/v1/responses", ""},
		{"/llm/call/primary/v1/responses", `"stream":true,`},
		{"/llm/rest/primary/v1/responses/input_tokens", ""},
		{"/llm/rest/primary/v1/responses/compact", ""},
	}
	for name, body := range bodies {
		for _, p := range paths {
			b := strings.Replace(body, `{"model"`, `{`+p.extra+`"model"`, 1)
			resp, out := h.do(h.keyC, http.MethodPost, p.path, b)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s %s: %s", name, p.path, out)
			assert.Contains(t, string(out), "forbidden content", "%s %s", name, p.path)
		}
	}
	assert.Empty(t, h.api.recorded(), "nothing blocked reached the vendor")

	resp, out := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":[{"role":"user","content":"fine"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
}

// A redacting filter's edits are written back into the Responses fields.
func TestResponsesFilters_RedactWritesBack(t *testing.T) {
	t.Parallel()
	h := newResponsesHarness(t)
	withPrimaryFilters(t, h, nativeRedactScript)
	resp, out := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses",
		`{"model":"gpt-4o","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"my SECRET"}]},`+
			`{"type":"function_call_output","call_id":"c1","output":"SECRET result"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	creates := h.api.callsTo(http.MethodPost, "/v1/responses")
	require.Len(t, creates, 1)
	sent := decodeObj(t, creates[0].Body)
	in := sent["input"].([]any)
	assert.Equal(t, "my [REDACTED]", in[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"])
	assert.Equal(t, "[REDACTED] result", in[1].(map[string]any)["output"])
	assert.Equal(t, "c1", in[1].(map[string]any)["call_id"])
}

// With filters attached, a prompt the gateway cannot read is refused rather
// than screened as empty; without filters it is the vendor's to judge.
func TestResponsesFilters_UnreadableInputFailsClosed(t *testing.T) {
	t.Parallel()
	h := newResponsesHarness(t)
	unreadable := `{"model":"gpt-4o","input":[{"role":"user","content":[{"type":"input_text","text":{"nested":"forbidden"}}]}]}`
	resp, _ := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", unreadable)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "no filters: forwarded")

	withPrimaryFilters(t, h, responsesBlockScript)
	before := len(h.api.recorded())
	resp, out := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", unreadable)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", out)
	assert.Contains(t, string(out), "could not be read for screening")
	assert.Len(t, h.api.recorded(), before)
}

// Batch lines with url /v1/responses are screened as Responses requests.
func TestResponsesFilters_BatchLines(t *testing.T) {
	t.Parallel()
	h := newBatchesHarness(t)
	withPrimaryFilters(t, h, responsesBlockScript)
	line := `{"custom_id":"r1","method":"POST","url":"/v1/responses","body":{"model":"gpt-4o","input":"something forbidden"}}`
	resp, out := h.uploadFile(h.keyC, chatLine("ok", "gpt-4o", "hello"), line)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", out)
	_, msg := oaiErr(t, out)
	assert.Contains(t, msg, `"r1"`)
	assert.Empty(t, h.api.callsTo(http.MethodPost, "/v1/files"))

	ok := `{"custom_id":"r2","method":"POST","url":"/v1/responses","body":{"model":"gpt-4o","input":"fine"}}`
	resp, out = h.uploadFile(h.keyC, ok)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
}

// Items stored in a conversation reach the model through every response
// that names it, so the filters screen them when they are stored.
func TestResponsesFilters_ConversationItems(t *testing.T) {
	t.Parallel()
	h := newResponsesHarness(t)
	h.primary.AllowedModels = []string{"^gpt-4o$"} // conversation items name no model
	withPrimaryFilters(t, h, nativeRedactScript, responsesBlockScript)

	resp, out := h.do(h.keyC, http.MethodPost, "/llm/call/primary/v1/conversations",
		`{"items":[{"type":"message","role":"user","content":"something forbidden"}]}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", out)
	assert.Contains(t, string(out), "forbidden content")

	resp, out = h.do(h.keyC, http.MethodPost, "/llm/call/primary/v1/conversations",
		`{"items":[{"type":"message","role":"user","content":"my SECRET"}],"metadata":{"k":"v"}}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	conv := decodeObj(t, out)["id"].(string)
	creates := h.api.callsTo(http.MethodPost, "/v1/conversations")
	require.Len(t, creates, 1)
	sent := decodeObj(t, creates[0].Body)
	assert.Equal(t, "my [REDACTED]", sent["items"].([]any)[0].(map[string]any)["content"])
	assert.Equal(t, map[string]any{"k": "v"}, sent["metadata"])

	resp, out = h.do(h.keyC, http.MethodPost, "/llm/call/primary/v1/conversations/"+conv+"/items",
		`{"items":[{"type":"function_call_output","call_id":"c","output":"forbidden data"}]}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", out)
	assert.Empty(t, h.api.callsTo(http.MethodPost, "/v1/conversations/"+vendorPart(conv)+"/items"))

	// Items the gateway cannot read are refused while filters are attached.
	resp, out = h.do(h.keyC, http.MethodPost, "/llm/call/primary/v1/conversations",
		`{"items":[{"role":"user","content":[{"type":"input_text","text":{"x":1}}]}]}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", out)
}

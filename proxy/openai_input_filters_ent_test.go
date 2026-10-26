//go:build enterprise

package proxy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The filters see the text of embeddings, completions and moderations
// requests, sync and as batch lines; token arrays are refused while filters
// are attached.
func TestOpenAIInputFilters(t *testing.T) {
	t.Parallel()
	h := newBatchesHarness(t)
	h.primary.AllowedModels = nil
	withPrimaryFilters(t, h, nativeRedactScript, responsesBlockScript)

	for _, c := range []struct{ path, body string }{
		{"/llm/rest/primary/v1/embeddings", `{"model":"text-embedding-3-small","input":["fine","forbidden"]}`},
		{"/llm/call/primary/v1/embeddings", `{"model":"text-embedding-3-small","input":"forbidden"}`},
		{"/llm/rest/primary/v1/completions", `{"model":"gpt-3.5-turbo-instruct","prompt":"something forbidden"}`},
		{"/llm/rest/primary/v1/completions", `{"model":"gpt-3.5-turbo-instruct","prompt":"a","suffix":"forbidden"}`},
		{"/llm/rest/primary/v1/moderations", `{"model":"omni-moderation-latest","input":[{"type":"text","text":"forbidden"}]}`},
	} {
		resp, out := h.do(h.keyC, http.MethodPost, c.path, c.body)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s %s: %s", c.path, c.body, out)
		assert.Contains(t, string(out), "forbidden content", c.path)
	}
	resp, out := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/embeddings", `{"model":"text-embedding-3-small","input":[1,2,3]}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", out)
	assert.Contains(t, string(out), "could not be read for screening")
	assert.Empty(t, h.api.recorded(), "nothing refused reached the vendor")

	resp, out = h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/embeddings", `{"model":"text-embedding-3-small","input":["my SECRET","ok"]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	calls := h.api.callsTo(http.MethodPost, "/v1/embeddings")
	require.Len(t, calls, 1)
	assert.Equal(t, []any{"my [REDACTED]", "ok"}, decodeObj(t, calls[0].Body)["input"])

	for url, body := range map[string]string{
		"/v1/embeddings":  `{"model":"gpt-4o","input":"forbidden"}`,
		"/v1/completions": `{"model":"gpt-4o","prompt":["ok","forbidden"]}`,
		"/v1/moderations": `{"model":"gpt-4o","input":"forbidden"}`,
	} {
		line := `{"custom_id":"bad","method":"POST","url":"` + url + `","body":` + body + `}`
		resp, out := h.uploadFile(h.keyC, line)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", url, out)
	}
	assert.Empty(t, h.api.callsTo(http.MethodPost, "/v1/files"))
}

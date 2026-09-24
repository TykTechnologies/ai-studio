package proxy

import (
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reasoning_effort reaches an OpenAI-wire upstream verbatim when the client
// sent it, and is absent upstream when it did not: values and defaults differ
// per model, so the gateway never supplies one. Anthropic has no such field
// and never receives it.
func TestBridge_ReasoningEffortPassThrough(t *testing.T) {
	post := func(h *endpointShapeHarness, model, extra string, stream bool) {
		t.Helper()
		streamField := ""
		if stream {
			streamField = `"stream":true,`
		}
		resp, body := h.post("/v1/chat/completions",
			`{"model":"shape-route/`+model+`",`+streamField+extra+`"messages":[{"role":"user","content":"hi"}]}`)
		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	}

	t.Run("openai", func(t *testing.T) {
		rec := &bodyRecorder{}
		h := newEndpointShapeHarness(t, models.OPENAI, "", rec.wrap(serveOpenAI))
		for _, stream := range []bool{false, true} {
			for _, effort := range []string{"none", "minimal", "xhigh"} {
				post(h, "gpt-5", `"reasoning_effort":"`+effort+`",`, stream)
				assert.Equal(t, effort, rec.body()["reasoning_effort"], "stream=%v", stream)
			}
			post(h, "gpt-5", "", stream)
			assert.NotContains(t, rec.body(), "reasoning_effort", "stream=%v: absent when not sent", stream)
			post(h, "gpt-5", `"reasoning_effort":null,`, stream)
			assert.NotContains(t, rec.body(), "reasoning_effort", "stream=%v: null means absent", stream)
			assert.NotContains(t, rec.body(), "metadata", "stream=%v: never leaks into metadata", stream)
		}
	})

	t.Run("anthropic", func(t *testing.T) {
		rec := &bodyRecorder{}
		h := newEndpointShapeHarness(t, models.ANTHROPIC, "", rec.wrap(serveAnthropic(t)))
		post(h, "claude-sonnet-5", `"reasoning_effort":"high",`, false)
		assert.NotContains(t, rec.body(), "reasoning_effort")
		assert.NotContains(t, rec.body(), "metadata")
	})
}

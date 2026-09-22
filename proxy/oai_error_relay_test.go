package proxy

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The inner hop's refusal reaches the outer /ai/ handler as a driver error
// string with the whole OpenAI envelope quoted inside it. Whatever the
// envelope says (a policy block from the loopback hop, or the vendor's own
// refusal passed through it) the caller must get that envelope's message,
// type and code, not our wrapper around the driver's wrapper around it.
func TestUpstreamEnvelope_RelayedVerbatim(t *testing.T) {
	const envelope = `{"error":{"message":"prompt has 3 images, max is 2","type":"invalid_request_error","code":"too_many_images"}}`
	refuse := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(envelope))
	}

	for name, body := range map[string]string{"non-streaming": failoverChatBody, "streaming": failoverStreamBody} {
		t.Run(name, func(t *testing.T) {
			h := newFailoverHarness(t, refuse, serveOpenAIText("never"), nil)
			resp, got := h.post("/ai/primary/v1/chat/completions", body)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", got)

			var out OAIErrorResponse
			require.NoError(t, json.Unmarshal(got, &out), "body: %s", got)
			require.NotNil(t, out.Error)
			assert.Equal(t, "prompt has 3 images, max is 2", out.Error.Message)
			// The drivers keep only the envelope's message, so type and
			// code are the ones the status implies.
			assert.Equal(t, oaiErrorType(http.StatusBadRequest), out.Error.Type)
			assert.Equal(t, oaiErrorCode(http.StatusBadRequest), out.Error.Code)
			assert.Empty(t, h.fallbackVendor.calls(), "a 400 must not fail over")
		})
	}
}

// An error with no envelope in it keeps the existing wrapper, so a vendor
// that answers with plain text is still reported with its status.
func TestUpstreamPlainTextError_KeepsTheWrapper(t *testing.T) {
	refuse := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("nope"))
	}
	h := newFailoverHarness(t, refuse, serveOpenAIText("never"), nil)
	resp, got := h.post("/ai/primary/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", got)

	var out OAIErrorResponse
	require.NoError(t, json.Unmarshal(got, &out), "body: %s", got)
	require.NotNil(t, out.Error)
	assert.Contains(t, out.Error.Message, "failed to generate content")
	assert.Equal(t, oaiErrorType(http.StatusBadRequest), out.Error.Type)
}

func TestInnerOAIError_ParsesTheDriverErrorString(t *testing.T) {
	t.Run("langchaingo prefix around an envelope", func(t *testing.T) {
		err := errors.New(`API returned unexpected status code: 400: {"error":{"message":"[ERROR] msg: policy_violation err: Policy error: f - m","type":"invalid_request_error","code":"invalid_request_error"}}` + "\n")
		inner, ok := innerOAIError(err)
		require.True(t, ok)
		assert.Equal(t, "[ERROR] msg: policy_violation err: Policy error: f - m", inner.Message)
		assert.Equal(t, "invalid_request_error", inner.Type)
		assert.Equal(t, "invalid_request_error", inner.Code)
	})

	t.Run("langchaingo keeps only the envelope message", func(t *testing.T) {
		inner, ok := innerOAIError(errors.New("API returned unexpected status code: 400: [ERROR] msg: policy_violation err: Policy error: f - m\n"))
		require.True(t, ok)
		assert.Equal(t, "[ERROR] msg: policy_violation err: Policy error: f - m", inner.Message)
		assert.Empty(t, inner.Type, "the driver dropped it; the caller derives it from the status")
	})

	t.Run("status with no message falls back", func(t *testing.T) {
		_, ok := innerOAIError(errors.New("API returned unexpected status code: 503"))
		assert.False(t, ok)
		_, ok = innerOAIError(errors.New("API returned unexpected status code: 503: "))
		assert.False(t, ok)
	})

	t.Run("other driver text falls back", func(t *testing.T) {
		_, ok := innerOAIError(errors.New("googleapi: Error 429: quota"))
		assert.False(t, ok)
	})

	t.Run("json that is not an envelope falls back", func(t *testing.T) {
		_, ok := innerOAIError(errors.New(`status 500: {"detail":"boom"}`))
		assert.False(t, ok)
	})

	t.Run("nil error falls back", func(t *testing.T) {
		_, ok := innerOAIError(nil)
		assert.False(t, ok)
	})
}

// The analytics body for a block is what the Compliance service matches on,
// and it was built with Sprintf around the raw message: a filter message
// containing a double quote produced invalid JSON in proxy_logs.response_body.
func TestPolicyViolationBody_QuotesProduceValidJSON(t *testing.T) {
	err := errors.New(`Policy error: dlp - found "secret" in prompt` + "\n" + `and a backslash \`)
	body := policyViolationBody(err)

	var decoded map[string]string
	require.NoError(t, json.Unmarshal(body, &decoded), "body: %s", body)
	assert.Equal(t, "policy_violation", decoded["error"])
	assert.Equal(t, err.Error(), decoded["detail"])
	assert.Contains(t, string(body), `"error":"policy_violation"`)
}

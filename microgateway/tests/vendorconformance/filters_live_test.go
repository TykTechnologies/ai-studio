//go:build vendorlive && enterprise

package vendorconformance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	mgwdb "github.com/TykTechnologies/midsommar/microgateway/internal/database"
	vc "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/vendorconformance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBedrockRequestFilters proves, against real Bedrock, that request filters
// run on the two paths that call the AWS SDK directly and never take the
// /llm/call/ loopback hop where every other vendor gets them: the /ai/ shim
// (per waterfall rung) and the /anthropic/ bridge. For each it checks that a
// blocking filter is a 400 before any vendor call, that a redacting filter's
// rewrite is what the model sees, and that the proxy log stores the body as
// the filter left it rather than what the caller sent.
//
// Needs the enterprise tag: the community scripting runner does not execute
// filter scripts, so under a CE build every filter is a no-op and this test
// would report the product as broken.
//
//	make test-vendors-filters
func TestBedrockRequestFilters(t *testing.T) {
	h := setup(t)
	if h.filteredBedrock == nil {
		t.Skip("bedrock has no credentials; nothing to filter")
	}
	bedrock, _ := h.cfg.Vendor("bedrock")
	model, _ := bedrock.Model(vc.ModelLatest)
	shimURL := fmt.Sprintf("%s/ai/%s/v1/chat/completions", h.baseURL, filteredBedrockSlug)
	bridgeURL := fmt.Sprintf("%s/anthropic/%s/v1/messages", h.baseURL, filteredBedrockSlug)

	t.Run("shim/blocked", func(t *testing.T) {
		nonce := newNonce()
		body := shimBody(model, "Reply with the word ok. "+filterBlockMarker+" "+nonce, false, h.cfg.MaxTokens)
		resp := mustPost(t, h, shimURL, body)

		require.Equal(t, http.StatusBadRequest, resp.Status, bodyExcerpt(resp.Body))
		assert.Contains(t, string(resp.Body), filterBlockMessage, "the filter's own message reaches the caller")
		assert.True(t, looksLikeErrorEnvelope(resp.Body), "a block is an OpenAI error envelope:\n%s", bodyExcerpt(resp.Body))

		ev := waitForLoggedRequest(t, h, nonce)
		assert.Equal(t, http.StatusBadRequest, ev.StatusCode)
		assert.Contains(t, ev.RequestBody, filterBlockMarker, "a blocked request is logged with the body that was refused")
	})

	t.Run("shim/redacted", func(t *testing.T) {
		nonce := newNonce()
		body := shimBody(model, "Reply with exactly the text after the colon and nothing else: "+filterSecret+" "+nonce, false, h.cfg.MaxTokens)
		resp := mustPost(t, h, shimURL, body)
		if skipIfThrottled(t, resp) {
			return
		}
		require.Equal(t, http.StatusOK, resp.Status, bodyExcerpt(resp.Body))
		requireJSON(t, resp.Body)
		assert.NotContains(t, string(resp.Body), filterSecret, "the model must never have seen the secret")

		ev := waitForLoggedRequest(t, h, nonce)
		assert.Equal(t, http.StatusOK, ev.StatusCode)
		assert.Contains(t, ev.RequestBody, filterRedacted, "the proxy log stores the post-filter prompt")
		assert.NotContains(t, ev.RequestBody, filterSecret, "the pre-filter prompt must not reach the database")
	})

	t.Run("shim/streaming blocked before headers", func(t *testing.T) {
		nonce := newNonce()
		body := shimBody(model, "Reply with the word ok. "+filterBlockMarker+" "+nonce, true, h.cfg.MaxTokens)
		ctx, cancel := h.CallContext(context.Background())
		defer cancel()
		stream, meta, err := h.postStream(ctx, shimURL, body, nil)
		require.NoError(t, err)

		assert.Nil(t, stream, "a block must not open a stream")
		require.Equal(t, http.StatusBadRequest, meta.Status, bodyExcerpt(meta.Body))
		assert.Contains(t, string(meta.Body), filterBlockMessage)
	})

	t.Run("shim/streaming redacted", func(t *testing.T) {
		nonce := newNonce()
		body := shimBody(model, "Reply with exactly the text after the colon and nothing else: "+filterSecret+" "+nonce, true, h.cfg.MaxTokens)
		ctx, cancel := h.CallContext(context.Background())
		defer cancel()
		stream, meta, err := h.postStream(ctx, shimURL, body, nil)
		require.NoError(t, err)
		if meta.Status == http.StatusTooManyRequests || meta.Status == http.StatusServiceUnavailable {
			t.Skipf("upstream returned %d", meta.Status)
		}
		require.Equal(t, http.StatusOK, meta.Status, bodyExcerpt(meta.Body))
		require.NotNil(t, stream, "expected an event stream, got:\n%s", bodyExcerpt(meta.Body))
		if raw, failed := streamCarriedUpstreamError(stream); failed {
			t.Fatalf("stream carried an upstream error:\n%s", bodyExcerpt(raw))
		}
		for _, f := range stream.DataFrames() {
			assert.NotContains(t, string(f.Raw), filterSecret, "the model must never have seen the secret")
		}

		ev := waitForLoggedRequest(t, h, nonce)
		assert.Contains(t, ev.RequestBody, filterRedacted, "the streaming proxy log stores the post-filter prompt")
		assert.NotContains(t, ev.RequestBody, filterSecret)
	})

	bridgeModel := bedrock.Extra["anthropic_bridge_model"]
	if bridgeModel == "" {
		t.Log("VT_BEDROCK_ANTHROPIC_MODEL not set; skipping the /anthropic/ bridge cases")
		return
	}

	t.Run("bridge/blocked", func(t *testing.T) {
		nonce := newNonce()
		body := bridgeBody("Reply with the word ok. "+filterBlockMarker+" "+nonce, false, h.cfg.MaxTokens)
		resp := mustPost(t, h, bridgeURL, body)

		require.Equal(t, http.StatusBadRequest, resp.Status, bodyExcerpt(resp.Body))
		assert.Equal(t, "invalid_request_error", anthropicErrorType(t, resp.Body))
		assert.Contains(t, string(resp.Body), filterBlockMessage)

		ev := waitForLoggedRequest(t, h, nonce)
		assert.Equal(t, http.StatusBadRequest, ev.StatusCode)
	})

	t.Run("bridge/redacted", func(t *testing.T) {
		nonce := newNonce()
		body := bridgeBody("Reply with exactly the text after the colon and nothing else: "+filterSecret+" "+nonce, false, h.cfg.MaxTokens)
		resp := mustPost(t, h, bridgeURL, body)
		if skipIfThrottled(t, resp) {
			return
		}
		require.Equal(t, http.StatusOK, resp.Status, bodyExcerpt(resp.Body))
		requireJSON(t, resp.Body)
		assert.NotContains(t, string(resp.Body), filterSecret, "the model must never have seen the secret")

		ev := waitForLoggedRequest(t, h, nonce)
		assert.Equal(t, http.StatusOK, ev.StatusCode)
		assert.Contains(t, ev.RequestBody, filterRedacted, "the bridge proxy log stores the post-filter prompt")
		assert.NotContains(t, ev.RequestBody, filterSecret)
	})

	t.Run("bridge/streaming blocked before headers", func(t *testing.T) {
		nonce := newNonce()
		body := bridgeBody("Reply with the word ok. "+filterBlockMarker+" "+nonce, true, h.cfg.MaxTokens)
		ctx, cancel := h.CallContext(context.Background())
		defer cancel()
		stream, meta, err := h.postStream(ctx, bridgeURL, body, nil)
		require.NoError(t, err)

		assert.Nil(t, stream, "a block must not open a stream")
		require.Equal(t, http.StatusBadRequest, meta.Status, bodyExcerpt(meta.Body))
		assert.Equal(t, "invalid_request_error", anthropicErrorType(t, meta.Body))
	})
}

// shimBody is an OpenAI-format chat request for the /ai/ shim.
func shimBody(model, prompt string, stream bool, maxTokens int) []byte {
	req := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"messages":   []any{map[string]any{"role": "user", "content": prompt}},
	}
	if stream {
		req["stream"] = true
	}
	b, _ := json.Marshal(req)
	return b
}

// bridgeBody is an Anthropic Messages request for the /anthropic/ bridge. The
// model field is echoed, not used: the bridge calls the route's default model.
func bridgeBody(prompt string, stream bool, maxTokens int) []byte {
	req := map[string]any{
		"model":      "claude-conformance",
		"max_tokens": maxTokens,
		"messages":   []any{map[string]any{"role": "user", "content": prompt}},
	}
	if stream {
		req["stream"] = true
	}
	b, _ := json.Marshal(req)
	return b
}

func mustPost(t *testing.T, h *harness, url string, body []byte) *response {
	t.Helper()
	ctx, cancel := h.CallContext(context.Background())
	defer cancel()
	resp, err := h.post(ctx, url, body, nil)
	require.NoError(t, err)
	return resp
}

// newNonce tags one request so its analytics event can be found among the
// others the shared harness accumulates.
func newNonce() string {
	return fmt.Sprintf("nonce-%d", time.Now().UnixNano())
}

// waitForLoggedRequest returns the analytics event whose logged request body
// carries nonce. The proxy records the event on a background goroutine after
// the response is written, so it is polled rather than read once.
func waitForLoggedRequest(t *testing.T, h *harness, nonce string) mgwdb.AnalyticsEvent {
	t.Helper()
	return waitForLoggedRequestOn(t, h, h.filteredBedrock.ID, nonce)
}

// waitForLoggedRequestOn is waitForLoggedRequest for any route.
func waitForLoggedRequestOn(t *testing.T, h *harness, llmID uint, nonce string) mgwdb.AnalyticsEvent {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var events []mgwdb.AnalyticsEvent
		err := h.db.Where("llm_id = ? AND request_body LIKE ?", llmID, "%"+nonce+"%").
			Order("id desc").Limit(1).Find(&events).Error
		// The gateway writes the event on another connection of the same
		// shared-cache SQLite; a lock while it does so is a reason to poll
		// again, not a failure.
		if err != nil && !strings.Contains(err.Error(), "locked") {
			require.NoError(t, err)
		}
		if err == nil && len(events) == 1 {
			return events[0]
		}
		if time.Now().After(deadline) {
			t.Fatalf("no analytics event logged for request %s on llm %d", nonce, llmID)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// anthropicErrorType decodes an Anthropic error envelope and returns error.type.
func anthropicErrorType(t *testing.T, body []byte) string {
	t.Helper()
	var parsed struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoErrorf(t, json.Unmarshal(body, &parsed), "not an Anthropic error envelope:\n%s", bodyExcerpt(body))
	assert.Equal(t, "error", parsed.Type)
	return parsed.Error.Type
}

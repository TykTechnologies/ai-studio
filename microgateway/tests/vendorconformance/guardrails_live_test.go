//go:build vendorlive && enterprise

package vendorconformance

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	vc "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/vendorconformance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The guardrailed route proves guardrail filters through the data plane: the
// edge Filter row with kind/config, the gateway adapter's conversion onto the
// shared filter chain, the runner, and the real provider, all on the /ai/
// shim of a real vendor. It complements tests/guardrailconformance, which
// calls the providers directly. Seeding is in guardrails_seed_test.go.
//
// Needs the enterprise tag: under a community build every guardrail is a
// no-op and this test would report the product as broken.
//
//	make test-guardrails

func guardrailURL(h *harness) string {
	return fmt.Sprintf("%s/ai/%s/v1/chat/completions", h.baseURL, h.guardrailed.Slug)
}

// TestGuardrailFilters drives the four probes through the guardrailed route.
func TestGuardrailFilters(t *testing.T) {
	h := setup(t)
	if h.guardrailed == nil {
		t.Skip("no vendor configured to carry the guardrailed route")
	}
	t.Log(h.guardrails.Summary())
	url := guardrailURL(h)
	model := h.guardrailedModel

	t.Run("benign passes", func(t *testing.T) {
		nonce := newNonce()
		resp := mustPost(t, h, url, shimBody(model, "Reply with the word ok. "+nonce, false, h.cfg.MaxTokens))
		if skipIfThrottled(t, resp) {
			return
		}
		require.Equal(t, http.StatusOK, resp.Status, bodyExcerpt(resp.Body))
		requireJSON(t, resp.Body)
	})

	t.Run("credential blocked by the built-in library", func(t *testing.T) {
		nonce := newNonce()
		resp := mustPost(t, h, url, shimBody(model, "Reply with the word ok. "+vc.ProbeSecret+" "+nonce, false, h.cfg.MaxTokens))
		require.Equal(t, http.StatusBadRequest, resp.Status, bodyExcerpt(resp.Body))
		assert.Contains(t, string(resp.Body), guardrailBlockPrefix+"builtin")
		assert.True(t, looksLikeErrorEnvelope(resp.Body), "a block is an OpenAI error envelope:\n%s", bodyExcerpt(resp.Body))
		assert.NotContains(t, string(resp.Body), vc.ProbeSecretKey, "the block must not echo the credential")

		ev := waitForLoggedRequestOn(t, h, h.guardrailed.ID, nonce)
		assert.Equal(t, http.StatusBadRequest, ev.StatusCode)
	})

	t.Run("credential blocked before a stream opens", func(t *testing.T) {
		nonce := newNonce()
		ctx, cancel := h.CallContext(context.Background())
		defer cancel()
		stream, meta, err := h.postStream(ctx, url, shimBody(model, "Reply with the word ok. "+vc.ProbeSecret+" "+nonce, true, h.cfg.MaxTokens), nil)
		require.NoError(t, err)
		assert.Nil(t, stream, "a block must not open a stream")
		require.Equal(t, http.StatusBadRequest, meta.Status, bodyExcerpt(meta.Body))
	})

	t.Run("pii redacted before the vendor", func(t *testing.T) {
		nonce := newNonce()
		resp := mustPost(t, h, url, shimBody(model, "Reply with exactly the text after the colon and nothing else: "+vc.ProbePII+" "+nonce, false, h.cfg.MaxTokens))
		if skipIfThrottled(t, resp) {
			return
		}
		require.Equal(t, http.StatusOK, resp.Status, bodyExcerpt(resp.Body))
		assert.NotContains(t, string(resp.Body), vc.ProbePIIEmail, "the model must never have seen the email")

		ev := waitForLoggedRequestOn(t, h, h.guardrailed.ID, nonce)
		assert.NotContains(t, ev.RequestBody, vc.ProbePIIEmail, "the pre-filter prompt must not reach the database")
		assert.Contains(t, ev.RequestBody, "[REDACTED:EMAIL]", "the proxy log stores the redacted prompt")
	})

	t.Run("injection blocked", func(t *testing.T) {
		nonce := newNonce()
		resp := mustPost(t, h, url, shimBody(model, vc.ProbeInjection+" "+nonce, false, h.cfg.MaxTokens))
		require.Equal(t, http.StatusBadRequest, resp.Status, bodyExcerpt(resp.Body))
		assert.Contains(t, string(resp.Body), guardrailBlockPrefix)

		var remote []string
		for _, p := range h.guardrails.Remote() {
			if p.Expect[vc.GuardrailInjection] {
				remote = append(remote, p.Key)
			}
		}
		if len(remote) > 0 && !strings.Contains(string(resp.Body), guardrailBlockPrefix+remote[0]) {
			// The chain runs the remote providers first, in configured order,
			// so anything other than the first one blocking means it let the
			// probe through.
			t.Errorf("expected %s to block the injection probe first, got: %s", remote[0], bodyExcerpt(resp.Body))
		}
		t.Logf("injection blocked by: %s", bodyExcerpt(resp.Body))
	})
}

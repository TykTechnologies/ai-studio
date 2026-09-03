//go:build vendorlive

package vendorconformance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	vc "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/vendorconformance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPreflightConfig proves the harness booted and every vendor is routable
// before any money is spent on completions. When this fails, nothing downstream
// is worth reading.
func TestPreflightConfig(t *testing.T) {
	h := setup(t)

	require.NotEmpty(t, h.cfg.Vendors, "no vendors configured")
	for _, v := range h.cfg.Vendors {
		t.Run(v.Key, func(t *testing.T) {
			model, ok := v.Model(vc.ModelLatest)
			require.True(t, ok, "%s must be set", v.ModelEnvVar(vc.ModelLatest))
			require.NotEmpty(t, v.Endpoint, "%s must be set", v.EnvVar("ENDPOINT"))
			assert.Regexp(t, `^[A-Za-z0-9_-]+$`, v.Slug(),
				"slug must satisfy the unified router's slug pattern, or the universal surface cannot route to it")
			t.Logf("%s: vendor=%s endpoint=%s model=%s", v.Key, v.Vendor, v.Endpoint, model)
		})
	}
}

// TestPreflightModelsCallable is the check that stops "latest-1" rotting into
// "some model from two years ago".
//
// It sends a one-token completion for every configured model rather than
// scraping a model-list endpoint. That was the first design and it was wrong:
// the gateway's /v1/models advertises each LLM's configured DefaultModel, not
// the vendor's catalogue, so it can say nothing about MODEL_PREV. Vendors' own
// list endpoints are no better — they disagree on path and shape, and Bedrock's
// runtime API has none at all.
//
// A one-token call is uniform across every vendor, costs a fraction of a cent,
// and proves more: that the model exists, the credentials work, the route
// resolves and auth passes. When this is green, a later failure is a
// translation bug rather than a configuration one — which is the whole point of
// running it first.
func TestPreflightModelsCallable(t *testing.T) {
	h := setup(t)

	for _, v := range h.cfg.Vendors {
		for _, slot := range v.ModelSlots(h.cfg.IncludePrevModel) {
			v, slot := v, slot
			model, _ := v.Model(slot)

			t.Run(fmt.Sprintf("%s/%s", v.Key, slot), func(t *testing.T) {
				t.Parallel()

				ctx, cancel := h.CallContext(context.Background())
				defer cancel()

				body := vc.ChatRequest{
					"model":      model,
					"max_tokens": 1,
					"messages":   []any{map[string]any{"role": "user", "content": "hi"}},
				}.JSON()

				resp, err := h.post(ctx, h.URLFor(vc.SurfaceShim, v), body, nil)
				require.NoError(t, err, "calling %s", model)

				if resp.Status == http.StatusOK {
					return
				}

				t.Errorf("model %q is not callable (HTTP %d).\n"+
					"Check %s in test-secrets/vendors.env.\n\nResponse:\n%s",
					model, resp.Status, v.ModelEnvVar(slot), bodyExcerpt(resp.Body))
			})
		}
	}
}

// TestPreflightModelListingSchema checks the gateway's own /v1/models contract.
// Clients use it for discovery, so its shape matters even though it cannot tell
// us anything about which upstream models exist.
func TestPreflightModelListingSchema(t *testing.T) {
	h := setup(t)

	ctx, cancel := h.CallContext(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+"/v1/models", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+testAPIToken)

	resp, err := h.client.Do(req)
	require.NoError(t, err, "listing models")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "the gateway must serve /v1/models to an authenticated app")
	body := readAll(t, resp.Body)

	res, err := vc.Validate(vc.FormatOpenAIModelsList, body)
	require.NoError(t, err)
	if !res.OK() {
		h.recorder.RecordFailure("gateway-v1-models", res.Error(), body)
		t.Errorf("model listing failed its schema: %s\n%s", res.Error(), bodyExcerpt(body))
	}
	h.recorder.RecordDrift("gateway/v1-models", res.Drift)

	var listing struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &listing))

	// Every configured vendor must be discoverable, and only through the
	// unified "slug/model" form: a bare model ID there would be ambiguous once
	// two vendors offer the same model name.
	advertised := map[string]bool{}
	for _, m := range listing.Data {
		advertised[m.ID] = true
	}
	for _, v := range h.cfg.Vendors {
		model, _ := v.Model(vc.ModelLatest)
		want := v.Slug() + "/" + model
		assert.Truef(t, advertised[want],
			"%s is associated with the app but %q is not advertised by /v1/models", v.Key, want)
	}

	// A model the app is NOT associated with must never appear.
	for id := range advertised {
		route, _, err := splitUnifiedModel(id)
		if err != nil {
			t.Errorf("advertised model %q is not in vendor/model form; the unified router cannot route it", id)
			continue
		}
		assert.Truef(t, h.slugIsConfigured(route),
			"/v1/models advertised %q for an app that has no such LLM association", id)
	}
}

// TestPreflightAuthRejectsAnonymous confirms the gateway is actually enforcing
// auth. Without it, a misconfigured harness could pass every other test while
// proving nothing about the authenticated path.
func TestPreflightAuthRejectsAnonymous(t *testing.T) {
	h := setup(t)
	v := h.cfg.Vendors[0]
	model, _ := v.Model(vc.ModelLatest)

	body := vc.ChatRequest{
		"model":      model,
		"max_tokens": 16,
		"messages":   []any{map[string]any{"role": "user", "content": "ping"}},
	}.JSON()

	ctx, cancel := h.CallContext(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		h.URLFor(vc.SurfaceShim, v), strings.NewReader(string(body)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"an unauthenticated request must be rejected before it reaches a vendor")
}

// slugIsConfigured reports whether a route slug belongs to a configured vendor.
func (h *harness) slugIsConfigured(slug string) bool {
	for _, v := range h.cfg.Vendors {
		if v.Slug() == slug {
			return true
		}
	}
	return false
}

// splitUnifiedModel mirrors proxy.ParseUnifiedModel: split on the FIRST slash so
// model names containing slashes survive.
func splitUnifiedModel(model string) (route, bare string, err error) {
	idx := strings.Index(model, "/")
	if idx <= 0 || idx == len(model)-1 {
		return "", "", fmt.Errorf("not in vendor/model form")
	}
	return model[:idx], model[idx+1:], nil
}

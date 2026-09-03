//go:build vendorlive

package vendorconformance

import (
	"context"
	"testing"

	vc "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/vendorconformance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestShimUniversalEquivalence is the strongest check in the suite.
//
// The unified router is a pure pre-auth rewrite: it strips the vendor prefix
// from the model string and forwards to the same /ai/{slug}/ handler chain. Two
// responses that are each individually schema-valid can still disagree — a field
// dropped on one path only — and nothing else in the suite would notice.
//
// Content text is not compared: LLMs are non-deterministic, so only shape is
// meaningful. Shape is also where the bugs are.
func TestShimUniversalEquivalence(t *testing.T) {
	h := setup(t)

	if !h.cfg.HasSurface(vc.SurfaceShim) || !h.cfg.HasSurface(vc.SurfaceUniversal) {
		t.Skip("equivalence needs both the shim and universal surfaces enabled")
	}

	for _, v := range h.cfg.Vendors {
		if vc.SurfaceSupport(v.Key, vc.SurfaceUniversal) != vc.Supported {
			continue
		}
		v := v
		t.Run(v.Key, func(t *testing.T) {
			t.Parallel()

			model, _ := v.Model(vc.ModelLatest)
			c := vc.Case{
				Vendor: v, Surface: vc.SurfaceShim,
				Scenario: vc.ScenarioBasic, ModelSlot: vc.ModelLatest, Model: model,
			}
			req, ok := vc.BuildRequest(c, h.cfg)
			require.True(t, ok)

			ctx, cancel := h.CallContext(context.Background())
			defer cancel()

			shimResp, err := h.post(ctx, h.URLFor(vc.SurfaceShim, v), req.WithModel(model).JSON(), nil)
			require.NoError(t, err)
			require.Equalf(t, 200, shimResp.Status, "shim returned %d:\n%s", shimResp.Status, bodyExcerpt(shimResp.Body))

			uniResp, err := h.post(ctx, h.URLFor(vc.SurfaceUniversal, v), req.WithModel(v.Slug()+"/"+model).JSON(), nil)
			require.NoError(t, err)
			require.Equalf(t, 200, uniResp.Status, "universal returned %d:\n%s", uniResp.Status, bodyExcerpt(uniResp.Body))

			shimEnv, err := vc.Normalize(vc.FormatOpenAICompletion, shimResp.Body)
			require.NoError(t, err)
			uniEnv, err := vc.Normalize(vc.FormatOpenAICompletion, uniResp.Body)
			require.NoError(t, err)

			diffs := shimEnv.Diff(uniEnv, "shim", "universal")
			for _, d := range diffs {
				t.Errorf("shim and universal disagree: %s", d)
			}
			if len(diffs) > 0 {
				h.recorder.RecordFailure(v.Key+"-equivalence-shim", "diverged from universal", shimResp.Body)
				h.recorder.RecordFailure(v.Key+"-equivalence-universal", "diverged from shim", uniResp.Body)
			}

			// The model string is the one field that legitimately differs on the
			// wire: the client sent "slug/model" but must get the bare model
			// back, or SDKs that key a cache on the response model will miss.
			assert.NotContains(t, uniEnv.Model, v.Slug()+"/",
				"the universal surface echoed the routing prefix back in the response model")
		})
	}
}

// TestStreamingMatchesBuffered asserts that the streaming and non-streaming code
// paths agree. They are separate implementations for every vendor, and the
// streaming one is the one that gets less attention.
func TestStreamingMatchesBuffered(t *testing.T) {
	h := setup(t)

	if !h.cfg.HasSurface(vc.SurfaceShim) {
		t.Skip("needs the shim surface")
	}
	if !h.cfg.HasScenario(vc.ScenarioBasic) || !h.cfg.HasScenario(vc.ScenarioUsage) {
		t.Skip("needs the basic and usage scenarios")
	}

	for _, v := range h.cfg.Vendors {
		v := v
		t.Run(v.Key, func(t *testing.T) {
			t.Parallel()

			model, _ := v.Model(vc.ModelLatest)
			url := h.URLFor(vc.SurfaceShim, v)

			ctx, cancel := h.CallContext(context.Background())
			defer cancel()

			buffered, ok := vc.BuildRequest(vc.Case{
				Vendor: v, Surface: vc.SurfaceShim, Scenario: vc.ScenarioBasic,
				ModelSlot: vc.ModelLatest, Model: model,
			}, h.cfg)
			require.True(t, ok)

			bufResp, err := h.post(ctx, url, buffered.WithModel(model).JSON(), nil)
			require.NoError(t, err)
			require.Equalf(t, 200, bufResp.Status, "buffered returned %d:\n%s", bufResp.Status, bodyExcerpt(bufResp.Body))

			streaming, ok := vc.BuildRequest(vc.Case{
				Vendor: v, Surface: vc.SurfaceShim, Scenario: vc.ScenarioUsage,
				ModelSlot: vc.ModelLatest, Model: model,
			}, h.cfg)
			require.True(t, ok)

			stream, meta, err := h.postStream(ctx, url, streaming.WithModel(model).JSON(), nil)
			require.NoError(t, err)
			require.Equalf(t, 200, meta.Status, "streaming returned %d:\n%s", meta.Status, bodyExcerpt(meta.Body))
			require.NotNil(t, stream)

			bufEnv, err := vc.Normalize(vc.FormatOpenAICompletion, bufResp.Body)
			require.NoError(t, err)
			streamEnv := stream.Envelope()

			// Compared on shape only, and finish_reason is allowed to differ:
			// two independent generations can legitimately end differently (one
			// hits max_tokens, the other stops).
			assert.Equal(t, bufEnv.Role, streamEnv.Role, "role differs between buffered and streaming")
			assert.Equal(t, bufEnv.ContentKind, streamEnv.ContentKind, "content kind differs between buffered and streaming")

			if bufEnv.HasUsage && !streamEnv.HasUsage {
				t.Error("buffered response carried usage but the stream did not, " +
					"even with stream_options.include_usage set")
			}
		})
	}
}

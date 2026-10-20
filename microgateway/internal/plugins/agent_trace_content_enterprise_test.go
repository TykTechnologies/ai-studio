//go:build enterprise
// +build enterprise

package plugins

import (
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	// The Enterprise content pipeline, as the Enterprise edge imports it.
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/agenttracing/content"
)

// Acceptance (.20): edge-captured content is redacted before it enters the
// pulse.
func TestCapture_EdgeContentRedactedBeforeThePulse(t *testing.T) {
	proc := agenttrace.NewContentProcessor(agenttrace.ContentGlobals{
		DefaultMode: "redacted", MaxRequestOverride: "metadata", StoreCap: "full",
		Detectors: []string{"email"},
	}, func(uint, agenttrace.Resource) agenttrace.ContentOverrides { return agenttrace.ContentOverrides{} })
	require.NotNil(t, proc, "the Enterprise pipeline registers its factory")
	agenttrace.SetContentProcessor(proc)
	t.Cleanup(func() { agenttrace.SetContentProcessor(nil) })

	secret := "ghp_1A2b3C4d5E6f7G8h9I0jKlMnOpQrStUvWxYz"
	client := &fakeSyncClient{}
	p := newCapturePlugin(t, client, CaptureSettings{MaxEvents: 10, MaxBytes: 1 << 20, FlushMaxEvents: 500, ChunkMaxBytes: 4 << 20})
	ev := rawEdgeEvent("mail jane.doe@example.com with key " + secret)
	ev.ErrorMessage = "failed for jane.doe@example.com"
	p.BufferCaptureEvents([]*agenttrace.Event{ev})
	p.flushCapture(p.capture.Load())

	pulses := client.capturePulses()
	require.Len(t, pulses, 1)
	wire, err := proto.Marshal(pulses[0])
	require.NoError(t, err)
	for _, leaked := range []string{"jane.doe@example.com", secret} {
		assert.False(t, strings.Contains(string(wire), leaked), "%q in the pulse", leaked)
	}
	pe := pulses[0].CaptureEvents[0]
	require.Len(t, pe.Parts, 1)
	assert.Equal(t, agenttrace.StateRedacted, pe.Parts[0].ContentState)
	assert.Contains(t, string(pe.Parts[0].Data), "[REDACTED:EMAIL]")
	assert.Contains(t, string(pe.Parts[0].Data), "[REDACTED:GITHUB_PAT]")
	assert.Equal(t, "redacted", pe.ContentMode)
	assert.Equal(t, "failed for [REDACTED:EMAIL]", pe.ErrorMessage)
}

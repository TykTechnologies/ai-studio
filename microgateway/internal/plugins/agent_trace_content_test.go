package plugins

import (
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

const edgeContentCanary = "edge-raw-content-canary"

func rawEdgeEvent(text string) *agenttrace.Event {
	return &agenttrace.Event{
		Kind:     agenttrace.KindLLMInference,
		SpanID:   agenttrace.NewSpanID(),
		Identity: agenttrace.Identity{AppID: 9},
		Raw: []agenttrace.RawPart{{
			Part: agenttrace.PartRequest, MediaType: "application/json",
			Data: []byte(`{"messages":[{"role":"user","content":"` + text + `"}]}`), Bytes: int64(len(text) + 40),
		}},
	}
}

// Over a hub connection that is not verified TLS the edge sends metadata
// only: processed content is dropped before the pulse (jyx.127).
func TestCapture_MetadataOnlyDropsProcessedContent(t *testing.T) {
	for _, metadataOnly := range []bool{false, true} {
		client := &fakeSyncClient{}
		p := newCapturePlugin(t, client, CaptureSettings{MaxEvents: 10, MaxBytes: 1 << 20, FlushMaxEvents: 500,
			ChunkMaxBytes: 4 << 20, MetadataOnly: metadataOnly})
		p.BufferCaptureEvents([]*agenttrace.Event{edgeEvent(edgeContentCanary)})
		p.flushCapture(p.capture.Load())

		pulses := client.capturePulses()
		require.Len(t, pulses, 1)
		wire, err := proto.Marshal(pulses[0])
		require.NoError(t, err)
		ev := pulses[0].CaptureEvents[0]
		require.Len(t, ev.Parts, 1)
		assert.Equal(t, "search", ev.ToolName, "metadata still flows")
		if metadataOnly {
			assert.False(t, strings.Contains(string(wire), edgeContentCanary), "content crossed an unverified channel")
			assert.Equal(t, agenttrace.StateDropped, ev.Parts[0].ContentState)
		} else {
			assert.True(t, strings.Contains(string(wire), edgeContentCanary), "verified TLS carries processed content")
		}
	}
}

// Without a content pipeline (Community build, or tracing off on the hub)
// raw content never enters the pulse: the part arrives as dropped.
func TestCapture_RawContentWithoutPipelineIsDropped(t *testing.T) {
	agenttrace.SetContentProcessor(nil)
	client := &fakeSyncClient{}
	p := newCapturePlugin(t, client, CaptureSettings{MaxEvents: 10, MaxBytes: 1 << 20, FlushMaxEvents: 500, ChunkMaxBytes: 4 << 20})
	p.BufferCaptureEvents([]*agenttrace.Event{rawEdgeEvent(edgeContentCanary)})
	p.flushCapture(p.capture.Load())

	pulses := client.capturePulses()
	require.Len(t, pulses, 1)
	wire, err := proto.Marshal(pulses[0])
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(wire), edgeContentCanary), "raw content in the pulse")
	require.Len(t, pulses[0].CaptureEvents[0].Parts, 1)
	assert.Equal(t, agenttrace.StateDropped, pulses[0].CaptureEvents[0].Parts[0].ContentState)
}

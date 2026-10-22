package plugins

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func newCapturePlugin(t *testing.T, client *fakeSyncClient, s CaptureSettings) *AnalyticsPulsePlugin {
	t.Helper()
	p := newRetryPlugin(client, PulsePluginConfig{MaxRetries: 1})
	p.ctx, p.cancel = context.WithCancel(context.Background())
	t.Cleanup(p.cancel)
	if s.FlushInterval == 0 {
		s.FlushInterval = time.Hour // tests flush by hand unless they say otherwise
	}
	p.EnableCapture(s)
	return p
}

func edgeEvent(data string) *agenttrace.Event {
	return &agenttrace.Event{
		Kind:     agenttrace.KindToolCall,
		SpanID:   agenttrace.NewSpanID(),
		Identity: agenttrace.Identity{AppID: 9},
		ToolName: "search",
		Parts: []agenttrace.PayloadPart{{
			Part: agenttrace.PartToolArgs, ContentState: agenttrace.StateFull, Data: []byte(data),
		}},
	}
}

func (f *fakeSyncClient) capturePulses() []*pb.AnalyticsPulse {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*pb.AnalyticsPulse
	for _, p := range f.pulses {
		if len(p.CaptureEvents) > 0 || len(p.CaptureDropped) > 0 {
			out = append(out, p)
		}
	}
	return out
}

func TestCapture_DisabledIsNoop(t *testing.T) {
	client := &fakeSyncClient{}
	p := newRetryPlugin(client, PulsePluginConfig{MaxRetries: 1})
	assert.False(t, p.CaptureEnabled())
	p.BufferCaptureEvents([]*agenttrace.Event{edgeEvent("x")})
	p.stopCapture()
	assert.Zero(t, client.callCount())
}

func TestCapture_FlushSendsCaptureOnlyPulses(t *testing.T) {
	client := &fakeSyncClient{}
	p := newCapturePlugin(t, client, CaptureSettings{MaxEvents: 100, MaxBytes: 1 << 20, FlushMaxEvents: 500, ChunkMaxBytes: 4 << 20})
	a, b := edgeEvent("a"), edgeEvent("b")
	p.BufferCaptureEvents([]*agenttrace.Event{a, b})
	c := p.capture.Load()
	p.flushCapture(c)

	pulses := client.capturePulses()
	require.Len(t, pulses, 1)
	pl := pulses[0]
	assert.Equal(t, "edge-1", pl.EdgeId)
	assert.Empty(t, pl.AnalyticsEvents, "capture travels in pulses of its own")
	require.Len(t, pl.CaptureEvents, 2)
	assert.Equal(t, uint32(2), pl.TotalRecords)
	assert.Equal(t, "edge-1:"+a.SpanID.String()+":0", pl.CaptureEvents[0].IdempotencyKey)
	assert.Equal(t, "search", pl.CaptureEvents[1].ToolName)
	assert.Zero(t, c.queue.Depth(), "delivered events are released")
	assert.Equal(t, uint64(2), c.sent.Load())
}

func TestCapture_ChunksUnderTheSizeCap(t *testing.T) {
	client := &fakeSyncClient{}
	p := newCapturePlugin(t, client, CaptureSettings{MaxEvents: 100, MaxBytes: 1 << 20, FlushMaxEvents: 500, ChunkMaxBytes: 600})
	var events []*agenttrace.Event
	for i := 0; i < 6; i++ {
		events = append(events, edgeEvent(strings.Repeat("p", 150)))
	}
	oversize := edgeEvent(strings.Repeat("z", 5000)) // alone over the cap: loses its payload
	events = append(events, oversize)
	p.BufferCaptureEvents(events)
	c := p.capture.Load()
	p.flushCapture(c)

	pulses := client.capturePulses()
	require.Greater(t, len(pulses), 1, "the batch is split across pulses")
	total := 0
	for _, pl := range pulses {
		size := 0
		for _, e := range pl.CaptureEvents {
			size += proto.Size(e) + 8
		}
		assert.LessOrEqual(t, size, 600, "a pulse's capture events stay under the chunk cap")
		total += len(pl.CaptureEvents)
	}
	assert.Equal(t, 7, total, "nothing lost to chunking")
	assert.Equal(t, uint64(1), c.queue.Dropped(agenttrace.DropPayload))
	assert.Nil(t, oversize.Parts[0].Data)
	// The payload drop is reported to the hub on the next flush.
	p.BufferCaptureEvents([]*agenttrace.Event{edgeEvent("n")})
	p.flushCapture(c)
	last := client.capturePulses()[len(client.capturePulses())-1]
	assert.Equal(t, uint64(1), last.CaptureDropped[agenttrace.DropPayload])
}

func TestCapture_FailedSendKeepsEventsForTheNextFlush(t *testing.T) {
	client := &fakeSyncClient{failures: 2} // one attempt + one retry fail
	p := newCapturePlugin(t, client, CaptureSettings{MaxEvents: 100, MaxBytes: 1 << 20, FlushMaxEvents: 500, ChunkMaxBytes: 4 << 20})
	p.BufferCaptureEvents([]*agenttrace.Event{edgeEvent("a")})
	c := p.capture.Load()
	p.flushCapture(c)
	assert.Equal(t, int64(1), c.queue.Depth(), "undelivered events stay queued")
	assert.Zero(t, c.sent.Load())

	p.flushCapture(c)
	assert.Zero(t, c.queue.Depth())
	assert.Equal(t, uint64(1), c.sent.Load())
	pulses := client.capturePulses()
	assert.Equal(t, pulses[0].CaptureEvents[0].IdempotencyKey, pulses[len(pulses)-1].CaptureEvents[0].IdempotencyKey,
		"a resend carries the same idempotency key, so the hub drops the duplicate")
}

func TestCapture_FullBufferDropsAndReportsUp(t *testing.T) {
	client := &fakeSyncClient{}
	p := newCapturePlugin(t, client, CaptureSettings{MaxEvents: 2, MaxBytes: 1 << 20, FlushMaxEvents: 500, ChunkMaxBytes: 4 << 20})
	p.BufferCaptureEvents([]*agenttrace.Event{edgeEvent("a"), edgeEvent("b"), edgeEvent("c"), edgeEvent("d")})
	c := p.capture.Load()
	assert.Equal(t, uint64(2), c.queue.Dropped(agenttrace.DropSpan))
	p.flushCapture(c)
	pulses := client.capturePulses()
	require.Len(t, pulses, 1)
	assert.Len(t, pulses[0].CaptureEvents, 2)
	assert.Equal(t, uint64(2), pulses[0].CaptureDropped[agenttrace.DropSpan])
}

func TestCapture_FlushesOnItsOwnCadence(t *testing.T) {
	client := &fakeSyncClient{}
	p := newCapturePlugin(t, client, CaptureSettings{MaxEvents: 100, MaxBytes: 1 << 20, FlushInterval: 20 * time.Millisecond,
		FlushMaxEvents: 500, ChunkMaxBytes: 4 << 20})
	p.BufferCaptureEvents([]*agenttrace.Event{edgeEvent("a")})
	require.Eventually(t, func() bool { return len(client.capturePulses()) == 1 }, 2*time.Second, 10*time.Millisecond)
}

func TestCapture_FullBatchFlushesAtOnce(t *testing.T) {
	client := &fakeSyncClient{}
	p := newCapturePlugin(t, client, CaptureSettings{MaxEvents: 100, MaxBytes: 1 << 20, FlushMaxEvents: 3, ChunkMaxBytes: 4 << 20})
	p.BufferCaptureEvents([]*agenttrace.Event{edgeEvent("a"), edgeEvent("b"), edgeEvent("c")})
	require.Eventually(t, func() bool { return len(client.capturePulses()) == 1 }, 2*time.Second, 10*time.Millisecond)
}

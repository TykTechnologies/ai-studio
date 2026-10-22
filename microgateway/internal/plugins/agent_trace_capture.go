package plugins

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CaptureSettings is the edge side of agent tracing transport (decision D3).
type CaptureSettings struct {
	MaxEvents      int           // queue cap, events
	MaxBytes       int64         // queue cap, bytes
	FlushInterval  time.Duration // send at least this often (D3: 10 s)
	FlushMaxEvents int           // or as soon as this many are queued (D3: 500)
	ChunkMaxBytes  int           // capture events per pulse (D3: 4 MiB)
	// MetadataOnly drops captured content before it is sent: the edge's
	// connection to the hub is not verified TLS (TLS off, or certificate
	// verification skipped), and content must not cross it (D4).
	MetadataOnly bool
}

// captureOutbox holds capture events between requests and their pulses. It
// has its own caps and drop order (agenttrace.Queue), separate from the
// shared buffer for billing records, so capture traffic can never push
// usage records out, and its own cadence, so traces settle in seconds while
// the analytics pulse keeps its interval.
type captureOutbox struct {
	settings CaptureSettings
	queue    *agenttrace.Queue
	kick     chan struct{}
	flushMu  sync.Mutex // one flush at a time (the loop, or Stop)
	sent     atomic.Uint64
}

// EnableCapture starts agent tracing capture on this edge. Only the
// Enterprise microgateway calls it, once its start gate has passed. Calling
// it again does nothing.
func (p *AnalyticsPulsePlugin) EnableCapture(s CaptureSettings) {
	if s.FlushInterval <= 0 {
		s.FlushInterval = 10 * time.Second
	}
	if s.FlushMaxEvents <= 0 {
		s.FlushMaxEvents = 500
	}
	if s.ChunkMaxBytes <= 0 {
		s.ChunkMaxBytes = 4 << 20
	}
	c := &captureOutbox{
		settings: s,
		queue:    agenttrace.NewQueue(s.MaxEvents, s.MaxBytes, s.FlushMaxEvents, nil),
		kick:     make(chan struct{}, 1),
	}
	if !p.capture.CompareAndSwap(nil, c) {
		return
	}
	go p.runCapture(c)
	log.Info().Dur("flush_interval", s.FlushInterval).Int("flush_max_events", s.FlushMaxEvents).
		Msg("Agent tracing capture enabled on the analytics pulse")
}

// CaptureEnabled reports whether EnableCapture was called.
func (p *AnalyticsPulsePlugin) CaptureEnabled() bool { return p.capture.Load() != nil }

// BufferCaptureEvents queues capture events for the next capture pulse. It
// never blocks; over the caps events are degraded or dropped (spec §10) and
// the drops are reported to the hub with the next pulse.
func (p *AnalyticsPulsePlugin) BufferCaptureEvents(events []*agenttrace.Event) {
	c := p.capture.Load()
	if c == nil || len(events) == 0 {
		return
	}
	if c.queue.Add(events...) {
		select {
		case c.kick <- struct{}{}:
		default:
		}
	}
}

func (p *AnalyticsPulsePlugin) runCapture(c *captureOutbox) {
	t := time.NewTicker(c.settings.FlushInterval)
	defer t.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-t.C:
		case <-c.kick:
		}
		p.flushCapture(c)
	}
}

// flushCapture sends everything queued, a batch at a time. A batch that
// cannot be delivered goes back to the front of the queue (still within the
// caps) for the next flush; chunks of it the hub already staged are
// deduplicated there by idempotency key.
func (p *AnalyticsPulsePlugin) flushCapture(c *captureOutbox) {
	c.flushMu.Lock()
	defer c.flushMu.Unlock()
	for {
		b := c.queue.Take()
		dropped := c.queue.TakeUnreported()
		if len(b.Events) == 0 && len(dropped) == 0 {
			return
		}
		chunks := p.chunkCapture(c, b.Events)
		if len(chunks) == 0 {
			chunks = [][]*pb.CaptureEventProto{nil} // drops to report, nothing to send
		}
		for i, chunk := range chunks {
			pulse := p.capturePulse(chunk)
			if i == 0 {
				pulse.CaptureDropped = dropped
			}
			if _, err := p.sendWithRetry(pulse, pulse.SequenceNumber); err != nil {
				c.queue.Restore(b)
				if i == 0 {
					c.queue.ReturnUnreported(dropped)
				}
				log.Warn().Err(err).Int("capture_events", len(b.Events)).
					Msg("Agent tracing capture pulse not delivered; kept for the next flush")
				return
			}
			c.sent.Add(uint64(len(chunk)))
		}
		c.queue.Release(b)
		if len(b.Events) < c.settings.FlushMaxEvents {
			return
		}
	}
}

// chunkCapture converts events to proto and splits them into chunks of at
// most ChunkMaxBytes. An event that alone exceeds the chunk loses its
// payload; if it still does not fit, it is dropped (spec §10 order).
func (p *AnalyticsPulsePlugin) chunkCapture(c *captureOutbox, events []*agenttrace.Event) [][]*pb.CaptureEventProto {
	const perEventOverhead = 8 // field tag and length prefix
	limit := c.settings.ChunkMaxBytes
	var chunks [][]*pb.CaptureEventProto
	var cur []*pb.CaptureEventProto
	curSize := 0
	for _, ev := range events {
		// The content pipeline runs on the edge, off the request path; raw
		// content never travels (ToProto ignores it).
		agenttrace.ProcessContent(ev)
		if c.settings.MetadataOnly && agenttrace.StripPayloadData(ev) {
			// Say why the content is missing (jyx.164).
			if ev.Attributes == nil {
				ev.Attributes = map[string]string{}
			}
			ev.Attributes["tyk.content.withheld"] = agenttrace.WithheldEdgeLinkUnverifiedTLS
		}
		ev.SetIdempotencyKey(p.edgeID)
		pe := agenttrace.ToProto(ev)
		size := proto.Size(pe) + perEventOverhead
		if size > limit && agenttrace.StripPayloadData(ev) {
			c.queue.Drop(agenttrace.DropPayload, 1)
			pe = agenttrace.ToProto(ev)
			size = proto.Size(pe) + perEventOverhead
		}
		if size > limit && agenttrace.StripGatewayEventDetail(ev) {
			c.queue.Drop(agenttrace.DropGatewayEventDetail, 1)
			pe = agenttrace.ToProto(ev)
			size = proto.Size(pe) + perEventOverhead
		}
		if size > limit {
			c.queue.Drop(agenttrace.DropSpan, 1)
			continue
		}
		if curSize+size > limit && len(cur) > 0 {
			chunks = append(chunks, cur)
			cur, curSize = nil, 0
		}
		cur = append(cur, pe)
		curSize += size
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}

func (p *AnalyticsPulsePlugin) capturePulse(events []*pb.CaptureEventProto) *pb.AnalyticsPulse {
	p.bufferMutex.Lock()
	seq := p.sequenceNumber
	p.sequenceNumber++
	p.bufferMutex.Unlock()
	now := timestamppb.Now()
	return &pb.AnalyticsPulse{
		EdgeId:         p.edgeID,
		EdgeNamespace:  p.edgeNamespace,
		PulseTimestamp: now,
		DataFrom:       now,
		DataTo:         now,
		SequenceNumber: seq,
		CaptureEvents:  events,
		TotalRecords:   uint32(len(events)),
	}
}

// stopCapture makes one last attempt to send what is queued. What is still
// queued after it is lost (decision D3 accepts loss on edge restart).
func (p *AnalyticsPulsePlugin) stopCapture() {
	if c := p.capture.Load(); c != nil {
		p.flushCapture(c)
	}
}

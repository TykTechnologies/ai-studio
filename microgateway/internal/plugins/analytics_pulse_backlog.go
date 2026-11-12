package plugins

import (
	"slices"
	"sync/atomic"
	"time"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"
)

// The backlog (bead midsommar-clean-4x5y.36, option B): what an edge keeps
// for the control plane while analytics pulses fail.
//
// Without it, a pulse that is given up is held as it was, and the held
// batch and the buffers are trimmed to max_buffer_size records at every
// failed pulse, so an outage longer than the retries (15 s as packaged)
// loses all but the newest ~3,300 requests' records. With it, a failed
// batch and everything buffered behind it are kept as serialized pulse
// messages, each under its own sequence number, up to max_backlog_bytes:
// about 94 bytes per record (282 per request) instead of the ~600 a record
// takes as Go structs, so 256 MiB keeps about 900,000 requests. When a
// control plane answers again the backlog is sent first, oldest first, one
// message at a time, then the buffers. The hub stores a batch once by
// (edge ID, boot ID, sequence number), so a batch sent again after a lost
// acknowledgement is not stored twice.
//
// max_buffer_size keeps its meaning: the size at which a filling buffer is
// flushed and the most records one pulse carries. The backlog is in memory:
// it is lost if the edge restarts during the outage.

// defaultMaxBacklogBytes is max_backlog_bytes when the pulse file does not
// set it.
const defaultMaxBacklogBytes = 256 << 20

// backlogBatch is one undelivered pulse, serialized.
type backlogBatch struct {
	seq     uint64
	msg     []byte
	records int
	// newest is the time of its newest record: the batch expires, whole,
	// once that is older than edge_retention_hours.
	newest time.Time
}

// The backlog as metrics read it (microgateway_analytics_pulse_backlog_bytes
// and _batches), set whenever it changes.
var (
	pulseBacklogBytes   atomic.Int64
	pulseBacklogBatches atomic.Int64
	// pulseDroppedOverBacklog: the oldest batches discarded to keep the
	// backlog within max_backlog_bytes.
	pulseDroppedOverBacklog atomic.Uint64
)

// PulseBacklog returns the size of the analytics backlog: the bytes of
// serialized pulses waiting for a control plane, and how many pulses.
func PulseBacklog() (bytes, batches int64) {
	return pulseBacklogBytes.Load(), pulseBacklogBatches.Load()
}

// backlogEnabled reports whether failed pulses go to the backlog rather
// than being trimmed to max_buffer_size.
func (p *AnalyticsPulsePlugin) backlogEnabled() bool {
	return p.config.MaxBacklogBytes > 0
}

// newest is the time of the snapshot's newest record.
func (s *pulseSnapshot) newest() time.Time {
	var t time.Time
	later := func(ts time.Time) {
		if ts.After(t) {
			t = ts
		}
	}
	for _, ev := range s.analytics {
		later(ev.TimeStamp)
	}
	for _, b := range s.budget {
		later(b.Timestamp)
	}
	for _, pr := range s.proxy {
		later(pr.LastRequest)
	}
	for _, ce := range s.compliance {
		later(ce.Timestamp)
	}
	for _, tc := range s.toolCalls {
		later(tc.Timestamp)
	}
	return t
}

// pulseRecords is how many records a pulse message carries.
func pulseRecords(m *pb.AnalyticsPulse) int {
	return len(m.AnalyticsEvents) + len(m.BudgetEvents) + len(m.ProxySummaries) + len(m.ComplianceEvents) + len(m.ToolCalls)
}

// splitPulse splits a pulse message in two: the first gets half of the
// records (rounded down, at least one), the oldest of each kind, kinds in a
// fixed order, and keeps the sequence number; the second gets the rest
// under seq2. A message of one record cannot be split: second is nil.
func splitPulse(m *pb.AnalyticsPulse, seq2 uint64) (first, second *pb.AnalyticsPulse) {
	n := pulseRecords(m)
	if n <= 1 {
		return m, nil
	}
	left := n / 2
	take := func(k int) int {
		if k > left {
			k = left
		}
		left -= k
		return k
	}
	// Shallow: the halves share m's records (sub-slices) and header values,
	// which are only read (marshalled) from here on; nothing is copied.
	header := func() *pb.AnalyticsPulse {
		return &pb.AnalyticsPulse{
			EdgeId:         m.EdgeId,
			EdgeNamespace:  m.EdgeNamespace,
			PulseTimestamp: m.PulseTimestamp,
			DataFrom:       m.DataFrom,
			DataTo:         m.DataTo,
			SequenceNumber: m.SequenceNumber,
			BootId:         m.BootId,
			IsCompressed:   m.IsCompressed,
			DataSizeBytes:  m.DataSizeBytes,
		}
	}
	first, second = header(), header()
	// Capture events, if a pulse ever carries them, stay with the first
	// half only, so they are not sent twice.
	first.CaptureEvents, first.CaptureDropped = m.CaptureEvents, m.CaptureDropped
	a := take(len(m.AnalyticsEvents))
	first.AnalyticsEvents, second.AnalyticsEvents = m.AnalyticsEvents[:a:a], m.AnalyticsEvents[a:]
	b := take(len(m.BudgetEvents))
	first.BudgetEvents, second.BudgetEvents = m.BudgetEvents[:b:b], m.BudgetEvents[b:]
	c := take(len(m.ProxySummaries))
	first.ProxySummaries, second.ProxySummaries = m.ProxySummaries[:c:c], m.ProxySummaries[c:]
	d := take(len(m.ComplianceEvents))
	first.ComplianceEvents, second.ComplianceEvents = m.ComplianceEvents[:d:d], m.ComplianceEvents[d:]
	e := take(len(m.ToolCalls))
	first.ToolCalls, second.ToolCalls = m.ToolCalls[:e:e], m.ToolCalls[e:]
	first.TotalRecords = uint32(pulseRecords(first))
	second.TotalRecords = uint32(pulseRecords(second))
	second.SequenceNumber = seq2
	return first, second
}

// nextSequence takes the next sequence number, for a part split off a
// batch. It takes bufferMutex itself, briefly, so that serializing a batch
// never holds the lock the request path records analytics under.
func (p *AnalyticsPulsePlugin) nextSequence() uint64 {
	p.bufferMutex.Lock()
	defer p.bufferMutex.Unlock()
	seq := p.sequenceNumber
	p.sequenceNumber++
	return seq
}

// encodeBacklog serializes a pulse into backlog batches, split until each
// is at most maxPulseBytes, so every batch can be delivered as it is. A
// record larger than a pulse may be by itself is dropped, as a fresh
// batch's is. Called without bufferMutex: it takes the lock only for a
// split's sequence number.
func (p *AnalyticsPulsePlugin) encodeBacklog(m *pb.AnalyticsPulse, newest time.Time) []backlogBatch {
	if pulseRecords(m) == 0 {
		return nil
	}
	if proto.Size(m) > maxPulseBytes {
		if pulseRecords(m) <= 1 {
			p.oversizedDropped.Add(1)
			pulseDroppedOversized.Add(1)
			log.Error().
				Uint64("sequence", m.SequenceNumber).
				Int("message_bytes", proto.Size(m)).
				Int("limit_bytes", maxPulseBytes).
				Msg("Analytics pulse: one record is larger than a pulse may be and cannot be delivered; it is dropped (stored request and response bodies are the usual reason: see ANALYTICS_STORE_REQUESTS / ANALYTICS_STORE_RESPONSES)")
			return nil
		}
		first, second := splitPulse(m, p.nextSequence())
		return append(p.encodeBacklog(first, newest), p.encodeBacklog(second, newest)...)
	}
	msg, err := proto.Marshal(m)
	if err != nil {
		// Cannot happen for a message proto.Size measured; nothing to keep.
		log.Error().Err(err).Uint64("sequence", m.SequenceNumber).Msg("Analytics pulse: could not serialize a batch for the backlog; it is dropped")
		return nil
	}
	return []backlogBatch{{seq: m.SequenceNumber, msg: msg, records: pulseRecords(m), newest: newest}}
}

// setBacklogGaugesLocked publishes the backlog's size. Caller holds
// bufferMutex.
func (p *AnalyticsPulsePlugin) setBacklogGaugesLocked() {
	pulseBacklogBytes.Store(p.backlogBytes)
	pulseBacklogBatches.Store(int64(len(p.backlog)))
}

// backlogRecordsLocked is how many records the backlog holds. Caller holds
// bufferMutex.
func (p *AnalyticsPulsePlugin) backlogRecordsLocked() int {
	n := 0
	for _, b := range p.backlog {
		n += b.records
	}
	return n
}

// pushBacklogLocked appends batches to the backlog. Caller holds bufferMutex.
func (p *AnalyticsPulsePlugin) pushBacklogLocked(batches []backlogBatch) {
	for _, b := range batches {
		p.backlog = append(p.backlog, b)
		p.backlogBytes += int64(len(b.msg))
	}
	p.setBacklogGaugesLocked()
}

// spillToBacklog moves everything buffered (the records returned from a
// split batch first, then the buffers) into the backlog as pulses of at
// most max_buffer_size records, each under a new sequence number, then
// applies the backlog's bounds. Called by the sender after a failed pulse:
// the records are taken under the lock, and serialized outside it, so
// requests recording analytics do not wait for the encoding.
func (p *AnalyticsPulsePlugin) spillToBacklog(now time.Time) {
	type taken struct {
		snap pulseSnapshot
		seq  uint64
	}
	var batches []taken
	p.bufferMutex.Lock()
	p.unreturnLocked()
	for {
		snap := p.takeBatchLocked(p.batchRecords())
		if snap.empty() {
			break
		}
		batches = append(batches, taken{snap: snap, seq: p.sequenceNumber})
		p.sequenceNumber++
	}
	p.bufferMutex.Unlock()

	var encoded []backlogBatch
	for _, b := range batches {
		m := p.buildPulseMessage(b.snap.analytics, b.snap.metadata, b.snap.budget, b.snap.proxy, b.snap.compliance, b.snap.toolCalls, b.seq)
		makeValidUTF8(m.ProtoReflect())
		encoded = append(encoded, p.encodeBacklog(m, b.snap.newest())...)
	}

	p.bufferMutex.Lock()
	p.pushBacklogLocked(encoded)
	p.trimBacklogLocked(now)
	p.bufferMutex.Unlock()
}

// trimBacklogLocked applies the backlog's bounds: batches whose newest
// record is older than edge_retention_hours are dropped, then the oldest
// batches until the backlog is within max_backlog_bytes. Both are counted
// and logged. Caller holds bufferMutex.
func (p *AnalyticsPulsePlugin) trimBacklogLocked(now time.Time) {
	expired, overBound := 0, 0
	if hours := p.config.EdgeRetentionHours; hours > 0 {
		cutoff := now.Add(-time.Duration(hours) * time.Hour)
		kept := p.backlog[:0]
		for _, b := range p.backlog {
			if b.newest.Before(cutoff) {
				expired += b.records
				p.backlogBytes -= int64(len(b.msg))
				continue
			}
			kept = append(kept, b)
		}
		p.backlog = kept
	}
	for p.backlogBytes > p.config.MaxBacklogBytes && len(p.backlog) > 0 {
		b := p.backlog[0]
		p.backlog = p.backlog[1:]
		p.backlogBytes -= int64(len(b.msg))
		overBound += b.records
	}
	p.setBacklogGaugesLocked()
	pulseDroppedExpired.Add(uint64(expired))
	pulseDroppedOverBacklog.Add(uint64(overBound))
	if expired > 0 || overBound > 0 {
		log.Warn().
			Int("expired", expired).
			Int("dropped_over_backlog", overBound).
			Int64("backlog_bytes", p.backlogBytes).
			Int64("max_backlog_bytes", p.config.MaxBacklogBytes).
			Str("pulses_fail_because", pulseFailureReason(p.lastError)).
			AnErr("last_error", p.lastError).
			Msg("Analytics records discarded: pulses are failing and the backlog for the control plane is full (max_backlog_bytes); the oldest go first")
	}
}

// deliverBacklogHead sends the oldest batch of the backlog, as it was
// serialized, under its own sequence number, and reports what became of it.
// Delivered, it leaves the backlog; failed, it stays at the head, and what
// was buffered meanwhile is moved behind it.
func (p *AnalyticsPulsePlugin) deliverBacklogHead() pulseOutcome {
	p.bufferMutex.Lock()
	head := p.backlog[0]
	p.bufferMutex.Unlock()

	pulse := &pb.AnalyticsPulse{}
	if err := proto.Unmarshal(head.msg, pulse); err != nil {
		// Cannot happen for bytes this process marshalled; drop it rather
		// than block the backlog behind it.
		log.Error().Err(err).Uint64("sequence", head.seq).Msg("Analytics pulse: a backlog batch could not be read back; it is dropped")
		p.popBacklogHead()
		return pulseDelivered
	}

	resp, err := p.sendWithRetry(pulse, head.seq)
	if err != nil && isMessageTooLarge(err) {
		// The control plane's limit is below this edge's: split the batch
		// in place, the older half keeping its sequence number. Serialized
		// before the lock is taken.
		first, second := splitPulse(pulse, p.nextSequence())
		if second == nil {
			p.oversizedDropped.Add(1)
			pulseDroppedOversized.Add(1)
			log.Error().Uint64("sequence", head.seq).Str("cause", err.Error()).Msg("Analytics pulse: one record is larger than the control plane accepts and cannot be delivered; it is dropped")
			p.popBacklogHead()
			return pulseDelivered
		}
		var parts []backlogBatch
		for _, m := range []*pb.AnalyticsPulse{first, second} {
			msg, merr := proto.Marshal(m)
			if merr != nil {
				continue
			}
			parts = append(parts, backlogBatch{seq: m.SequenceNumber, msg: msg, records: pulseRecords(m), newest: head.newest})
		}
		// Only the sender changes the backlog, so the head is still head.
		p.bufferMutex.Lock()
		p.backlogBytes -= int64(len(head.msg))
		p.backlog = slices.Replace(p.backlog, 0, 1, parts...) // entries are small headers: a copy of the slice, not of the messages
		for _, b := range parts {
			p.backlogBytes += int64(len(b.msg))
		}
		p.setBacklogGaugesLocked()
		p.bufferMutex.Unlock()
		log.Warn().Uint64("sequence", head.seq).Str("cause", err.Error()).Msg("Analytics pulse too large for the control plane - sending the backlog batch in parts")
		return pulseSplit
	}
	if err != nil {
		now := time.Now()
		p.bufferMutex.Lock()
		p.lastError = err
		p.lastErrorTime = now
		p.bufferMutex.Unlock()
		p.spillToBacklog(now)
		p.bufferMutex.Lock()
		bytes, batches := p.backlogBytes, len(p.backlog)
		p.bufferMutex.Unlock()
		log.Error().
			Err(err).
			Uint64("sequence", head.seq).
			Int64("backlog_bytes", bytes).
			Int("backlog_batches", batches).
			Msg("Analytics pulse not delivered - data kept in the backlog for the next pulse")
		return pulseFailed
	}

	p.popBacklogHead()
	p.bufferMutex.Lock()
	p.lastError = nil
	p.bufferMutex.Unlock()
	p.totalPulsesSent++
	p.totalRecordsSent += uint64(pulse.TotalRecords)
	p.lastPulseTime = time.Now()
	if !resp.Success {
		log.Error().Str("message", resp.Message).Uint64("sequence", head.seq).Msg("Analytics pulse rejected by control server")
	}
	return pulseDelivered
}

// popBacklogHead removes the oldest batch of the backlog.
func (p *AnalyticsPulsePlugin) popBacklogHead() {
	p.bufferMutex.Lock()
	defer p.bufferMutex.Unlock()
	if len(p.backlog) == 0 {
		return
	}
	p.backlogBytes -= int64(len(p.backlog[0].msg))
	p.backlog[0] = backlogBatch{}
	p.backlog = p.backlog[1:]
	p.setBacklogGaugesLocked()
}

// failToBacklog keeps a fresh batch that was not delivered: it goes to the
// backlog (serialized, under its sequence number) with everything buffered
// behind it. Called by deliver instead of holdSnapshotLocked when the
// backlog is on.
func (p *AnalyticsPulsePlugin) failToBacklog(pulse *pb.AnalyticsPulse, snap pulseSnapshot, now time.Time) {
	encoded := p.encodeBacklog(pulse, snap.newest())
	p.bufferMutex.Lock()
	p.pushBacklogLocked(encoded)
	p.bufferMutex.Unlock()
	p.spillToBacklog(now)
}

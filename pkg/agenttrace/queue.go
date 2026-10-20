package agenttrace

import "sync"

// Capture drop reasons (spec §10; tyk_tracing_capture_dropped{reason}).
const (
	DropPayload            = "payload"              // payload bytes removed, span kept (§10 step 1)
	DropGatewayEventDetail = "gateway_event_detail" // gateway event details removed (§10 step 2)
	DropSpan               = "span"                 // whole span dropped (§10 step 3)
	DropNoSealer           = "no_sealer"            // payload bytes removed: no key to seal them
	DropWriteError         = "write_error"          // the staging write or the send failed for good
)

// Queue is the bounded capture queue between the request path and a
// transport (the hub's staging writer, the edge's pulse). Add never blocks:
// when the caps are reached the incoming event is degraded in the §10 order
// (payload bytes, then gateway event details) until it fits, or dropped.
// Events count against the caps from Add until Release, so a transport that
// is stalled or retrying holds memory within the caps.
type Queue struct {
	mu        sync.Mutex
	pending   []*Event
	sizes     []int64
	held      int64
	heldBytes int64
	maxEvents int64
	maxBytes  int64
	batchSize int
	onDrop    func(reason string, n int64)

	dropped    map[string]uint64
	unreported map[string]uint64
}

// Batch is a run of events taken from a Queue.
type Batch struct {
	Events []*Event
	Bytes  int64
	sizes  []int64
}

// NewQueue builds a queue. onDrop (optional) is called for every drop, for
// metrics; it must not call back into the queue.
func NewQueue(maxEvents int, maxBytes int64, batchSize int, onDrop func(reason string, n int64)) *Queue {
	if maxEvents <= 0 {
		maxEvents = 10000
	}
	if maxBytes <= 0 {
		maxBytes = 64 << 20
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	return &Queue{
		maxEvents:  int64(maxEvents),
		maxBytes:   maxBytes,
		batchSize:  batchSize,
		onDrop:     onDrop,
		dropped:    map[string]uint64{},
		unreported: map[string]uint64{},
	}
}

func (q *Queue) dropLocked(reason string, n int64) {
	q.dropped[reason] += uint64(n)
	q.unreported[reason] += uint64(n)
	if q.onDrop != nil {
		q.onDrop(reason, n)
	}
}

// Drop counts n drops for reason (for losses the transport sees itself).
func (q *Queue) Drop(reason string, n int64) {
	if n <= 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.dropLocked(reason, n)
}

// Add admits events, degrading or dropping them to fit the caps. The queue
// owns the events afterwards. It reports whether a full batch is pending.
func (q *Queue) Add(events ...*Event) (batchReady bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, ev := range events {
		if ev == nil {
			continue
		}
		if q.held+1 > q.maxEvents {
			q.dropLocked(DropSpan, 1)
			continue
		}
		size := int64(ev.ApproxSize())
		if q.heldBytes+size > q.maxBytes && StripPayloadData(ev) {
			q.dropLocked(DropPayload, 1)
			size = int64(ev.ApproxSize())
		}
		if q.heldBytes+size > q.maxBytes && StripGatewayEventDetail(ev) {
			q.dropLocked(DropGatewayEventDetail, 1)
			size = int64(ev.ApproxSize())
		}
		if q.heldBytes+size > q.maxBytes {
			q.dropLocked(DropSpan, 1)
			continue
		}
		q.pending = append(q.pending, ev)
		q.sizes = append(q.sizes, size)
		q.held++
		q.heldBytes += size
	}
	return len(q.pending) >= q.batchSize
}

// Take removes up to the batch size of pending events, oldest first. They
// stay counted against the caps until Release or Restore.
func (q *Queue) Take() Batch {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := len(q.pending)
	if n > q.batchSize {
		n = q.batchSize
	}
	b := Batch{Events: make([]*Event, n), sizes: make([]int64, n)}
	copy(b.Events, q.pending[:n])
	copy(b.sizes, q.sizes[:n])
	for _, s := range b.sizes {
		b.Bytes += s
	}
	q.pending = append(q.pending[:0], q.pending[n:]...)
	q.sizes = append(q.sizes[:0], q.sizes[n:]...)
	return b
}

// Release frees a taken batch's room once it was delivered (or given up on).
func (q *Queue) Release(b Batch) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.held -= int64(len(b.Events))
	q.heldBytes -= b.Bytes
}

// Restore puts a taken batch back in front of the pending events, for a
// transport that will retry it. It stays counted against the caps.
func (q *Queue) Restore(b Batch) {
	if len(b.Events) == 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pending = append(append([]*Event(nil), b.Events...), q.pending...)
	q.sizes = append(append([]int64(nil), b.sizes...), q.sizes...)
}

// Depth is the number of events held (pending or taken, not yet released).
func (q *Queue) Depth() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.held
}

// Pending is the number of events not yet taken.
func (q *Queue) Pending() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

// Dropped returns the total drops counted for reason.
func (q *Queue) Dropped(reason string) uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.dropped[reason]
}

// TakeUnreported returns the drops counted since the last call and resets
// them, for a transport that reports drops upstream. Give them back with
// ReturnUnreported if the report was not delivered.
func (q *Queue) TakeUnreported() map[string]uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.unreported) == 0 {
		return nil
	}
	out := q.unreported
	q.unreported = map[string]uint64{}
	return out
}

// ReturnUnreported adds undelivered drop counts back.
func (q *Queue) ReturnUnreported(m map[string]uint64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for k, v := range m {
		q.unreported[k] += v
	}
}

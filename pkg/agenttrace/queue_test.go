package agenttrace

import (
	"strings"
	"sync"
	"testing"
)

func queueEvent(data string) *Event {
	return &Event{
		Kind:          KindLLMInference,
		SpanID:        NewSpanID(),
		Parts:         []PayloadPart{{Part: PartRequest, ContentState: StateFull, Data: []byte(data)}},
		GatewayEvents: []GatewayEvent{{Type: GatewayEventAttempt, Detail: "attempt=1"}},
	}
}

func TestQueueDropOrder(t *testing.T) {
	var mu sync.Mutex
	reported := map[string]int64{}
	q := NewQueue(3, 1<<20, 500, func(reason string, n int64) {
		mu.Lock()
		reported[reason] += n
		mu.Unlock()
	})

	// Plenty of room: nothing is degraded.
	q.Add(queueEvent("small"))
	if q.Dropped(DropPayload) != 0 {
		t.Fatal("degraded with room to spare")
	}

	// A payload that does not fit loses its bytes first; the span stays.
	big := queueEvent(strings.Repeat("x", 2<<20))
	q.Add(big)
	if q.Dropped(DropPayload) != 1 || big.Parts[0].Data != nil || big.Parts[0].ContentState != StateDropped {
		t.Fatalf("payload not stripped first: %+v", big.Parts[0])
	}
	if big.GatewayEvents[0].Detail != "attempt=1" {
		t.Fatal("details go only when the payload alone is not enough")
	}

	// Then gateway event details.
	q.maxBytes = q.heldBytes + 300
	detail := queueEvent("")
	detail.Parts = nil
	detail.GatewayEvents[0].Detail = strings.Repeat("d", 400)
	q.Add(detail)
	if q.Dropped(DropGatewayEventDetail) != 1 || detail.GatewayEvents[0].Detail != "" || q.Depth() != 3 {
		t.Fatalf("details not stripped: depth %d", q.Depth())
	}

	// Then whole spans: by bytes...
	q.maxEvents = 10
	huge := queueEvent("")
	huge.Parts = nil
	huge.GatewayEvents = nil
	huge.Attributes = map[string]string{"k": strings.Repeat("a", 1000)}
	q.Add(huge)
	if q.Dropped(DropSpan) != 1 || q.Depth() != 3 {
		t.Fatalf("oversized bare span not dropped: depth %d", q.Depth())
	}
	// ...and by count.
	q.maxEvents, q.maxBytes = 3, 1<<30
	q.Add(queueEvent("a"), queueEvent("b"))
	if q.Dropped(DropSpan) != 3 || q.Depth() != 3 {
		t.Fatalf("event cap: dropped %d depth %d", q.Dropped(DropSpan), q.Depth())
	}

	mu.Lock()
	if reported[DropPayload] != 1 || reported[DropGatewayEventDetail] != 1 || reported[DropSpan] != 3 {
		t.Fatalf("onDrop counts: %v", reported)
	}
	mu.Unlock()
}

func TestQueueTakeReleaseRestore(t *testing.T) {
	q := NewQueue(10, 1<<20, 2, nil)
	a, b, c := queueEvent("a"), queueEvent("b"), queueEvent("c")
	if q.Add(a) {
		t.Fatal("batch ready after one event with batch size 2")
	}
	if !q.Add(b, c) {
		t.Fatal("batch not ready")
	}
	first := q.Take()
	if len(first.Events) != 2 || first.Events[0] != a || first.Events[1] != b || q.Pending() != 1 || q.Depth() != 3 {
		t.Fatalf("take: %d events, pending %d, depth %d", len(first.Events), q.Pending(), q.Depth())
	}
	// A failed send puts the batch back in front, still counted.
	bytesBefore := q.heldBytes
	q.Restore(first)
	if q.Pending() != 3 || q.Depth() != 3 || q.heldBytes != bytesBefore {
		t.Fatal("restore changed the accounting")
	}
	again := q.Take()
	if again.Events[0] != a || again.Bytes != first.Bytes {
		t.Fatal("restored batch not first")
	}
	q.Release(again)
	q.Release(q.Take())
	if q.Depth() != 0 || q.heldBytes != 0 {
		t.Fatalf("released queue not empty: depth %d bytes %d", q.Depth(), q.heldBytes)
	}
}

func TestQueueUnreportedDrops(t *testing.T) {
	q := NewQueue(1, 1<<20, 10, nil)
	q.Add(queueEvent("a"), queueEvent("b"), queueEvent("c"))
	got := q.TakeUnreported()
	if got[DropSpan] != 2 || q.TakeUnreported() != nil {
		t.Fatalf("unreported: %v", got)
	}
	q.ReturnUnreported(got)
	q.Drop(DropWriteError, 1)
	again := q.TakeUnreported()
	if again[DropSpan] != 2 || again[DropWriteError] != 1 || q.Dropped(DropSpan) != 2 {
		t.Fatalf("returned drops: %v", again)
	}
}

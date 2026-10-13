package chat_session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// pgNotifyLimit is Postgres's limit: a NOTIFY payload must be shorter.
const pgNotifyLimit = 8000

func TestNotificationPayloads_SmallMessageIsSentWhole(t *testing.T) {
	msg := []byte(`{"type":"chat_response","data":"aGk="}`)
	payloads, err := notificationPayloads(msg)
	if err != nil || len(payloads) != 1 || payloads[0] != string(msg) {
		t.Fatalf("notificationPayloads() = %v, %v; want the message unchanged", payloads, err)
	}
}

func TestNotificationPayloads_LargeMessageRoundTrips(t *testing.T) {
	msg := []byte(strings.Repeat(`{"payload":"ünïcødé \"quoted\" text"}`, 3000)) // ~110 KB
	payloads, err := notificationPayloads(msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(payloads) < 2 {
		t.Fatalf("got %d payloads, want the message split", len(payloads))
	}
	for i, p := range payloads {
		if len(p) >= pgNotifyLimit {
			t.Fatalf("payload %d is %d bytes, over the NOTIFY limit", i, len(p))
		}
	}

	// Deliver out of order with a duplicate: only the last missing chunk
	// completes the message.
	var r notifyReassembler
	now := time.Now()
	order := append([]int{len(payloads) - 1, 0}, seq(1, len(payloads)-1)...)
	order = append(order[:2], append([]int{0}, order[2:]...)...)
	var got string
	for i, idx := range order {
		out, complete, err := r.add(payloads[idx], now)
		if err != nil {
			t.Fatalf("add(chunk %d): %v", idx, err)
		}
		if complete != (i == len(order)-1) {
			t.Fatalf("add(chunk %d) complete = %v at step %d of %d", idx, complete, i+1, len(order))
		}
		got = out
	}
	if got != string(msg) {
		t.Fatal("reassembled message differs from the original")
	}
	if len(r.partial) != 0 {
		t.Errorf("reassembler kept %d partial messages after completion", len(r.partial))
	}
}

func TestNotifyReassembler_DropsStalePartials(t *testing.T) {
	payloads, err := notificationPayloads([]byte(strings.Repeat("x", 20000)))
	if err != nil {
		t.Fatal(err)
	}
	var r notifyReassembler
	start := time.Now()
	if _, complete, _ := r.add(payloads[0], start); complete {
		t.Fatal("first chunk completed the message")
	}

	// A different message arriving after the TTL sweeps the abandoned one.
	other, _ := notificationPayloads([]byte(strings.Repeat("y", 20000)))
	if _, _, err := r.add(other[0], start.Add(partialNotifyTTL+time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(r.partial) != 1 {
		t.Errorf("reassembler holds %d partial messages, want only the fresh one", len(r.partial))
	}
}

// A session holds at most maxPartialNotifies reassemblies; a new one drops
// the oldest, and storage follows the chunks that arrived, not the claimed
// total.
func TestNotifyReassembler_CapsConcurrentPartials(t *testing.T) {
	var r notifyReassembler
	start := time.Now()
	for i := 0; i < maxPartialNotifies+5; i++ {
		payload := fmt.Sprintf(`{"chunk_id":"m%d","chunk_seq":0,"chunk_total":%d,"chunk_data":"eA=="}`, i, maxNotifyChunks)
		if _, _, err := r.add(payload, start.Add(time.Duration(i)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.partial) != maxPartialNotifies {
		t.Fatalf("reassembler holds %d partial messages, want %d", len(r.partial), maxPartialNotifies)
	}
	if _, ok := r.partial["m0"]; ok {
		t.Error("the oldest partial message was not the one dropped")
	}
	for id, p := range r.partial {
		if len(p.parts) != 1 {
			t.Errorf("partial %s stores %d parts for one received chunk", id, len(p.parts))
		}
	}
}

func TestNotifyReassembler_RejectsBadChunks(t *testing.T) {
	var r notifyReassembler
	for _, payload := range []string{
		`{"chunk_id":"a","chunk_seq":2,"chunk_total":2,"chunk_data":"eA=="}`,
		`{"chunk_id":"a","chunk_seq":0,"chunk_total":0,"chunk_data":"eA=="}`,
		`{"chunk_id":"a",`,
	} {
		if _, complete, err := r.add(payload, time.Now()); err == nil || complete {
			t.Errorf("add(%s) = complete %v, err %v; want an error", payload, complete, err)
		}
	}
}

func seq(from, to int) []int {
	out := []int{}
	for i := from; i < to; i++ {
		out = append(out, i)
	}
	return out
}

// Messages far above the 8000-byte NOTIFY limit go through both Postgres
// queue implementations: chat responses, stream chunks, errors and LLM
// responses.
func TestPostgreSQLQueue_LargePayloads(t *testing.T) {
	db := openPostgresForQueueTest(t, 4)
	big := strings.Repeat("A long LLM answer with ünïcødé and \"quotes\". ", 2000) // ~90 KB

	factories := map[string]func(sessionID string) (MessageQueue, error){
		"shared": func(id string) (MessageQueue, error) {
			return NewSharedPostgreSQLQueueFactory(db, testQueueConfig(10, 5*time.Second)).CreateQueue(id, nil)
		},
		"classic": func(id string) (MessageQueue, error) {
			return NewPostgreSQLQueue(id, db, testQueueConfig(10, 5*time.Second))
		},
	}
	for name, create := range factories {
		t.Run(name, func(t *testing.T) {
			q, err := create(fmt.Sprintf("large-%s-%d", name, time.Now().UnixNano()))
			if err != nil {
				t.Fatal(err)
			}
			defer q.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			if err := q.PublishMessage(ctx, &ChatResponse{Payload: big}); err != nil {
				t.Fatalf("PublishMessage: %v", err)
			}
			select {
			case m := <-q.ConsumeMessages(ctx):
				if m.Payload != big {
					t.Fatalf("chat response arrived with %d bytes, want %d", len(m.Payload), len(big))
				}
			case <-ctx.Done():
				t.Fatal("timed out waiting for the large chat response")
			}

			if err := q.PublishStream(ctx, []byte(big)); err != nil {
				t.Fatalf("PublishStream: %v", err)
			}
			select {
			case s := <-q.ConsumeStream(ctx):
				if string(s) != big {
					t.Fatalf("stream data arrived with %d bytes, want %d", len(s), len(big))
				}
			case <-ctx.Done():
				t.Fatal("timed out waiting for the large stream chunk")
			}

			if err := q.PublishError(ctx, errors.New(big)); err != nil {
				t.Fatalf("PublishError: %v", err)
			}
			select {
			case e := <-q.ConsumeErrors(ctx):
				if e.Error() != big {
					t.Fatalf("error arrived with %d bytes, want %d", len(e.Error()), len(big))
				}
			case <-ctx.Done():
				t.Fatal("timed out waiting for the large error")
			}

			// A small message after the large ones still arrives whole.
			if err := q.PublishMessage(ctx, &ChatResponse{Payload: "small"}); err != nil {
				t.Fatal(err)
			}
			select {
			case m := <-q.ConsumeMessages(ctx):
				if m.Payload != "small" {
					t.Fatalf("small message = %q", m.Payload)
				}
			case <-ctx.Done():
				t.Fatal("timed out waiting for the small message")
			}
		})
	}
}

package chat_session

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid"
)

// Postgres refuses a NOTIFY payload of 8000 bytes or more, and a chat
// response or a stream chunk can be far larger. A queue message that does not
// fit is base64-encoded and split into chunks, all sent in one transaction:
// Postgres delivers a transaction's notifications together and in order, or
// not at all. The receiver reassembles them before routing. Nothing is stored,
// so there is nothing to clean up, and the in-memory and NATS queues (which
// have no such limit) are unaffected.
const (
	// maxInlineNotifyPayload is the largest message sent as one notification,
	// with headroom below the 8000-byte limit.
	maxInlineNotifyPayload = 7000
	// notifyChunkSize is the base64 characters carried per chunk.
	notifyChunkSize = 6000
	// maxNotifyChunks bounds one reassembly (about 60 MB of message).
	maxNotifyChunks = 10000
	// partialNotifyTTL drops a reassembly whose remaining chunks never came.
	partialNotifyTTL = 2 * time.Minute
)

// notifyChunk is one part of a split message. ChunkID is the first field, so
// a chunk's payload starts with chunkPrefix.
type notifyChunk struct {
	ChunkID string `json:"chunk_id"`
	Seq     int    `json:"chunk_seq"`
	Total   int    `json:"chunk_total"`
	Data    string `json:"chunk_data"`
}

const chunkPrefix = `{"chunk_id":`

// notificationPayloads returns the NOTIFY payloads that carry msg: msg
// itself when it fits, else its chunks in order.
func notificationPayloads(msg []byte) ([]string, error) {
	if len(msg) <= maxInlineNotifyPayload {
		return []string{string(msg)}, nil
	}
	encoded := base64.StdEncoding.EncodeToString(msg)
	total := (len(encoded) + notifyChunkSize - 1) / notifyChunkSize
	if total > maxNotifyChunks {
		return nil, fmt.Errorf("queue message of %d bytes is too large to send", len(msg))
	}
	id, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	payloads := make([]string, 0, total)
	for seq := 0; seq < total; seq++ {
		end := min((seq+1)*notifyChunkSize, len(encoded))
		b, err := json.Marshal(notifyChunk{ChunkID: id.String(), Seq: seq, Total: total, Data: encoded[seq*notifyChunkSize : end]})
		if err != nil {
			return nil, err
		}
		payloads = append(payloads, string(b))
	}
	return payloads, nil
}

// notifyPostgreSQL publishes msg on channel, split into chunks when it is
// too large for one notification.
func notifyPostgreSQL(ctx context.Context, db *sql.DB, channel string, msg []byte) error {
	payloads, err := notificationPayloads(msg)
	if err != nil {
		return err
	}
	if len(payloads) == 1 {
		if _, err := db.ExecContext(ctx, "SELECT pg_notify($1, $2)", channel, payloads[0]); err != nil {
			return fmt.Errorf("failed to notify: %w", err)
		}
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	for _, p := range payloads {
		if _, err := tx.ExecContext(ctx, "SELECT pg_notify($1, $2)", channel, p); err != nil {
			return fmt.Errorf("failed to notify: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit notification: %w", err)
	}
	return nil
}

// notifyReassembler joins the chunks of split messages for one session.
type notifyReassembler struct {
	mu      sync.Mutex
	partial map[string]*partialNotify
}

type partialNotify struct {
	parts    []string
	received int
	started  time.Time
}

// add takes a notification payload. It returns the whole message and true
// when the payload is a whole message or completes one, and false while a
// split message is still missing chunks.
func (r *notifyReassembler) add(payload string, now time.Time) (string, bool, error) {
	if !strings.HasPrefix(payload, chunkPrefix) {
		return payload, true, nil
	}
	var c notifyChunk
	if err := json.Unmarshal([]byte(payload), &c); err != nil {
		return "", false, fmt.Errorf("failed to unmarshal notification chunk: %w", err)
	}
	if c.Total < 1 || c.Total > maxNotifyChunks || c.Seq < 0 || c.Seq >= c.Total {
		return "", false, fmt.Errorf("invalid notification chunk %d of %d", c.Seq, c.Total)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.partial == nil {
		r.partial = map[string]*partialNotify{}
	}
	for id, p := range r.partial {
		if now.Sub(p.started) > partialNotifyTTL {
			delete(r.partial, id)
		}
	}
	p := r.partial[c.ChunkID]
	if p == nil {
		p = &partialNotify{parts: make([]string, c.Total), started: now}
		r.partial[c.ChunkID] = p
	}
	if len(p.parts) != c.Total {
		delete(r.partial, c.ChunkID)
		return "", false, fmt.Errorf("notification chunk %s changed its total", c.ChunkID)
	}
	if p.parts[c.Seq] == "" {
		p.parts[c.Seq] = c.Data
		p.received++
	}
	if p.received < c.Total {
		return "", false, nil
	}
	delete(r.partial, c.ChunkID)
	msg, err := base64.StdEncoding.DecodeString(strings.Join(p.parts, ""))
	if err != nil {
		return "", false, fmt.Errorf("failed to decode notification chunks: %w", err)
	}
	return string(msg), true, nil
}

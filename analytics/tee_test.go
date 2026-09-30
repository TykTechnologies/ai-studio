package analytics

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// teeRecorder records what it is given.
type teeRecorder struct {
	mu        sync.Mutex
	chats     []*models.LLMChatRecord
	logs      []*models.ProxyLog
	exchanges int
	block     chan struct{}
	panics    bool
}

func (h *teeRecorder) wait() {
	if h.block != nil {
		<-h.block
	}
	if h.panics {
		panic("a broken sink")
	}
}
func (h *teeRecorder) RecordChatRecord(_ context.Context, r *models.LLMChatRecord) {
	h.wait()
	h.mu.Lock()
	h.chats = append(h.chats, r)
	h.mu.Unlock()
}
func (h *teeRecorder) RecordChatLogEntry(context.Context, *models.LLMChatLogEntry) {}
func (h *teeRecorder) RecordProxyLog(_ context.Context, l *models.ProxyLog) {
	h.wait()
	h.mu.Lock()
	h.logs = append(h.logs, l)
	h.mu.Unlock()
}
func (h *teeRecorder) RecordToolCall(context.Context, string, time.Time, int, uint) {}
func (h *teeRecorder) SetAsGlobalHandler()                                          {}
func (h *teeRecorder) RecordChatRecordsBatch(_ context.Context, rs []*models.LLMChatRecord) {
	for _, r := range rs {
		h.RecordChatRecord(context.Background(), r)
	}
}
func (h *teeRecorder) RecordProxyLogsBatch(_ context.Context, ls []*models.ProxyLog) {
	for _, l := range ls {
		h.RecordProxyLog(context.Background(), l)
	}
}
func (h *teeRecorder) RecordComplianceEvents(context.Context, []*models.ComplianceEvent) {}
func (h *teeRecorder) counts() (int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.chats), len(h.logs)
}

// teeExchangeSink also stores a request's log and record together.
type teeExchangeSink struct{ teeRecorder }

func (h *teeExchangeSink) RecordExchange(_ context.Context, l *models.ProxyLog, r *models.LLMChatRecord) {
	h.mu.Lock()
	h.exchanges++
	h.mu.Unlock()
}

func TestTee_PrimaryAndSinksGetEveryRecord(t *testing.T) {
	primary, sink := &teeRecorder{}, &teeRecorder{}
	tee := NewTee(primary, sink)

	rec := &models.LLMChatRecord{Name: "gpt", Cost: 12}
	tee.RecordChatRecord(context.Background(), rec)
	tee.RecordChatRecordsBatch(context.Background(), []*models.LLMChatRecord{{Name: "a"}, {Name: "b"}})
	tee.RecordProxyLogsBatch(context.Background(), []*models.ProxyLog{{Vendor: "openai"}})
	tee.Stop(time.Second)

	c, l := primary.counts()
	assert.Equal(t, 3, c)
	assert.Equal(t, 1, l)
	c, l = sink.counts()
	assert.Equal(t, 3, c)
	assert.Equal(t, 1, l)

	// Copies: the primary writes into its records; the sink's are its own.
	assert.Same(t, rec, primary.chats[0])
	assert.NotSame(t, rec, sink.chats[0])
	rec.ID = 99
	assert.Zero(t, sink.chats[0].ID)
	assert.Equal(t, "gpt", sink.chats[0].Name)
}

// A slow or stuck sink never slows the request path: its queue fills and
// further records are dropped for it (counted), while the primary still
// gets them all.
func TestTee_StuckSinkDoesNotBlock(t *testing.T) {
	old := TeeQueueSize
	TeeQueueSize = 5
	t.Cleanup(func() { TeeQueueSize = old })
	primary := &teeRecorder{}
	stuck := &teeRecorder{block: make(chan struct{})}
	tee := NewTee(primary, stuck)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			tee.RecordChatRecord(context.Background(), &models.LLMChatRecord{})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("recording blocked on a stuck sink")
	}
	c, _ := primary.counts()
	assert.Equal(t, 100, c)
	assert.GreaterOrEqual(t, tee.Dropped()[0], uint64(90))

	close(stuck.block)
	tee.Stop(time.Second)
}

// A request's proxy log and chat record reach a sink that stores them
// together in one call; others get the two separately.
func TestTee_Exchange(t *testing.T) {
	primary, paired, split := &teeRecorder{}, &teeExchangeSink{}, &teeRecorder{}
	tee := NewTee(primary, paired, split)
	tee.RecordExchange(context.Background(), &models.ProxyLog{}, &models.LLMChatRecord{})
	tee.RecordExchange(context.Background(), &models.ProxyLog{}, nil) // no usage
	tee.Stop(time.Second)

	assert.Equal(t, 2, paired.exchanges)
	c, l := split.counts()
	assert.Equal(t, 1, c)
	assert.Equal(t, 2, l)
	c, l = primary.counts()
	assert.Equal(t, 1, c)
	assert.Equal(t, 2, l)
}

// A panicking sink is contained; recording during and after Stop neither
// panics nor loses anything for the primary.
func TestTee_BrokenSinkAndStop(t *testing.T) {
	primary := &teeRecorder{}
	tee := NewTee(primary, &teeRecorder{panics: true})

	var sent atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				tee.RecordChatRecord(context.Background(), &models.LLMChatRecord{})
				sent.Add(1)
			}
		}()
	}
	time.Sleep(time.Millisecond)
	tee.Stop(time.Second)
	wg.Wait()
	tee.Stop(time.Second) // again: no-op
	c, _ := primary.counts()
	require.Equal(t, int(sent.Load()), c)
}

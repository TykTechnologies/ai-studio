package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// pairHandler records the calls it receives, like a handler that predates
// ExchangeRecorder.
type pairHandler struct {
	mockHandler
	calls []string
}

func (p *pairHandler) RecordProxyLog(_ context.Context, _ *models.ProxyLog) {
	p.calls = append(p.calls, "proxy_log")
}

func (p *pairHandler) RecordChatRecord(_ context.Context, _ *models.LLMChatRecord) {
	p.calls = append(p.calls, "chat_record")
}

// exchangeHandler implements ExchangeRecorder.
type exchangeHandler struct {
	pairHandler
	exchanges int
	lastRec   *models.LLMChatRecord
}

func (e *exchangeHandler) RecordExchange(_ context.Context, _ *models.ProxyLog, rec *models.LLMChatRecord) {
	e.exchanges++
	e.lastRec = rec
}

func withHandler(t *testing.T, h AnalyticsHandler) {
	t.Helper()
	handlerMu.Lock()
	old := globalHandler
	globalHandler = h
	handlerMu.Unlock()
	t.Cleanup(func() {
		handlerMu.Lock()
		globalHandler = old
		handlerMu.Unlock()
	})
}

func TestRecordExchangeFallsBackToSeparateCalls(t *testing.T) {
	h := &pairHandler{}
	withHandler(t, h)

	log := &models.ProxyLog{AppID: 1, TimeStamp: time.Now()}
	RecordExchange(context.Background(), log, &models.LLMChatRecord{AppID: 1})
	RecordExchange(context.Background(), log, nil)

	want := []string{"proxy_log", "chat_record", "proxy_log"}
	if len(h.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", h.calls, want)
	}
	for i := range want {
		if h.calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", h.calls, want)
		}
	}
}

func TestRecordExchangeUsesExchangeRecorder(t *testing.T) {
	h := &exchangeHandler{}
	withHandler(t, h)

	rec := &models.LLMChatRecord{AppID: 1}
	RecordExchange(context.Background(), &models.ProxyLog{AppID: 1}, rec)

	if h.exchanges != 1 || h.lastRec != rec {
		t.Fatalf("exchanges = %d, rec passed = %v", h.exchanges, h.lastRec == rec)
	}
	if len(h.calls) != 0 {
		t.Fatalf("separate calls made too: %v", h.calls)
	}
}

func TestRecordExchangeWithoutHandler(t *testing.T) {
	withHandler(t, nil)
	RecordExchange(context.Background(), &models.ProxyLog{AppID: 1}, &models.LLMChatRecord{AppID: 1})
}

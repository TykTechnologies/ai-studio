package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// A context carrying a handler (WithHandler) sends its records there, not to
// the global handler, also once detached from its request (WithoutCancel).
// A context without one still records into the global handler.
func TestWithHandlerRoutesRecordsToTheContextsHandler(t *testing.T) {
	global := &exchangeHandler{}
	withHandler(t, global)
	scoped := &exchangeHandler{}

	ctx := context.WithoutCancel(WithHandler(context.Background(), scoped))
	RecordExchange(ctx, &models.ProxyLog{AppID: 1, TimeStamp: time.Now()}, &models.LLMChatRecord{AppID: 1})
	RecordProxyLog(ctx, &models.ProxyLog{AppID: 1})
	RecordChatRecord(ctx, &models.LLMChatRecord{AppID: 1})

	if scoped.exchanges != 1 || len(scoped.calls) != 2 {
		t.Fatalf("scoped handler: exchanges = %d, calls = %v; want 1 and [proxy_log chat_record]", scoped.exchanges, scoped.calls)
	}
	if global.exchanges != 0 || len(global.calls) != 0 {
		t.Fatalf("global handler got records meant for the scoped one: exchanges = %d, calls = %v", global.exchanges, global.calls)
	}

	RecordProxyLog(context.Background(), &models.ProxyLog{AppID: 1})
	if len(global.calls) != 1 {
		t.Fatalf("a context without a handler records into the global one: calls = %v", global.calls)
	}
}

// BindHandler fixes the handler current at bind time on the context: records
// made on it after the global handler is replaced still go to the old one.
// A context that already carries a handler keeps it.
func TestBindHandlerKeepsTheHandlerCurrentAtBindTime(t *testing.T) {
	first := &exchangeHandler{}
	withHandler(t, first)
	ctx := BindHandler(context.Background())

	second := &exchangeHandler{}
	SetHandler(second)
	RecordProxyLog(ctx, &models.ProxyLog{AppID: 1})
	if len(first.calls) != 1 || len(second.calls) != 0 {
		t.Fatalf("first got %v, second got %v; want the record in the first only", first.calls, second.calls)
	}

	scoped := &exchangeHandler{}
	ctx = BindHandler(WithHandler(context.Background(), scoped))
	RecordProxyLog(ctx, &models.ProxyLog{AppID: 1})
	if len(scoped.calls) != 1 || len(second.calls) != 0 {
		t.Fatalf("a scoped handler must win over binding: scoped %v, global %v", scoped.calls, second.calls)
	}

	SetHandler(nil)
	if got := BindHandler(context.Background()); got != context.Background() {
		t.Fatal("with no handler at all, BindHandler leaves the context unchanged")
	}
}

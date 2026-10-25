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

//go:build !enterprise

package studio

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
)

type hostSink struct {
	mu    sync.Mutex
	chats []*models.LLMChatRecord
}

func (h *hostSink) RecordChatRecord(_ context.Context, r *models.LLMChatRecord) {
	h.mu.Lock()
	h.chats = append(h.chats, r)
	h.mu.Unlock()
}
func (h *hostSink) RecordChatLogEntry(context.Context, *models.LLMChatLogEntry)       {}
func (h *hostSink) RecordProxyLog(context.Context, *models.ProxyLog)                  {}
func (h *hostSink) RecordToolCall(context.Context, string, time.Time, int, uint)      {}
func (h *hostSink) SetAsGlobalHandler()                                               {}
func (h *hostSink) RecordChatRecordsBatch(context.Context, []*models.LLMChatRecord)   {}
func (h *hostSink) RecordProxyLogsBatch(context.Context, []*models.ProxyLog)          {}
func (h *hostSink) RecordComplianceEvents(context.Context, []*models.ComplianceEvent) {}
func (h *hostSink) count() int                                                        { h.mu.Lock(); defer h.mu.Unlock(); return len(h.chats) }

// A host's sink gets Studio's analytics records alongside Studio's own
// database, and Stop takes the sink out of the process-wide handler.
func TestAnalyticsSinksReceiveRecords(t *testing.T) {
	opts := newTestOptions(t)
	sink := &hostSink{}
	opts.AnalyticsSinks = []analytics.AnalyticsHandler{sink}
	s, err := New(opts)
	require.NoError(t, err)

	analytics.RecordChatRecord(context.Background(), &models.LLMChatRecord{Name: "gpt-test", AppID: 7})
	require.Eventually(t, func() bool { return sink.count() == 1 }, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, "gpt-test", sink.chats[0].Name)

	// Studio's own database still gets it (budgets and spend read it).
	require.Eventually(t, func() bool {
		var n int64
		opts.DB.Model(&models.LLMChatRecord{}).Where("name = ?", "gpt-test").Count(&n)
		return n == 1
	}, 10*time.Second, 50*time.Millisecond)

	stopStudio(t, s)
	_, teed := analytics.GetHandler().(*analytics.Tee)
	assert.False(t, teed, "Stop unwraps the tee")
	analytics.RecordChatRecord(context.Background(), &models.LLMChatRecord{Name: "after-stop"})
	assert.Equal(t, 1, sink.count())
}

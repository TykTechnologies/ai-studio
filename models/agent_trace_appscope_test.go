package models

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Trace ids are client-supplied and not secret: two Apps sending the same
// traceparent (and even the same span id) keep separate rows, separate
// rollups and separate links, and App-scoped retention touches only its App.
func TestAgentTrace_SameIDsInTwoAppsStaySeparate(t *testing.T) {
	agentTraceDBs(t, func(t *testing.T, db *gorm.DB) {
		t0 := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
		for _, app := range []uint{1, 2} {
			require.NoError(t, UpsertAgentTraceTraceRollup(db, &AgentTraceTrace{TraceID: traceID(7), AppID: app,
				SessionID: "s", Source: "client_traceparent", StartedAt: t0, Status: "ok", SpanCount: 1, CostMicros: int64(app * 100)}))
		}
		require.NoError(t, UpsertAgentTraceTraceRollup(db, &AgentTraceTrace{TraceID: traceID(7), AppID: 1,
			SessionID: "s", Source: "client_traceparent", StartedAt: t0, Status: "ok", SpanCount: 1, CostMicros: 5}))

		var traces []AgentTraceTrace
		require.NoError(t, db.Order("app_id").Find(&traces, "trace_id = ?", traceID(7)).Error)
		require.Len(t, traces, 2, "one row per App")
		assert.Equal(t, 2, traces[0].SpanCount)
		assert.EqualValues(t, 105, traces[0].CostMicros)
		assert.Equal(t, 1, traces[1].SpanCount, "App 2's rollup is untouched by App 1")
		assert.EqualValues(t, 200, traces[1].CostMicros)

		span := func(app uint) AgentTraceSpan {
			return AgentTraceSpan{SpanID: spanID(1), TraceID: traceID(7), SessionID: "s", AppID: app,
				Kind: "llm_inference", Name: "chat", StartTime: t0, Status: "ok", ContentState: "metadata"}
		}
		require.NoError(t, InsertAgentTraceSpans(db, []AgentTraceSpan{span(1), span(2)}, 10))
		require.NoError(t, db.Create(&[]AgentTraceSpanLink{
			{AppID: 1, SpanID: spanID(1), SpanStart: t0, LinkedTraceID: traceID(7), LinkedSpanID: spanID(2), LinkType: "caused_by", Confidence: "exact", Rule: "tool_call_id"},
			{AppID: 2, SpanID: spanID(1), SpanStart: t0, LinkedTraceID: traceID(7), LinkedSpanID: spanID(3), LinkType: "caused_by", Confidence: "exact", Rule: "tool_call_id"},
		}).Error)
		for _, app := range []uint{1, 2} {
			page, err := ListAgentTraceSessionSpans(db, app, "s", nil, 10)
			require.NoError(t, err)
			require.Len(t, page, 1)
			assert.Equal(t, app, page[0].AppID)
			var links []AgentTraceSpanLink
			require.NoError(t, db.Find(&links, "app_id = ? AND span_id = ?", app, spanID(1)).Error)
			require.Len(t, links, 1)
		}

		// App-scoped retention deletes only that App's rows, even with
		// identical ids in the other App.
		app1 := uint(1)
		for _, target := range []AgentTraceRetentionTarget{AgentTraceRetentionSpans, AgentTraceRetentionTraces, AgentTraceRetentionSpanLinks} {
			n, err := DeleteAgentTraceRowsBefore(db, target, t0.Add(time.Hour), &app1, 10)
			require.NoError(t, err, target.Table)
			assert.EqualValues(t, 1, n, target.Table)
		}
		var spans []AgentTraceSpan
		require.NoError(t, db.Find(&spans).Error)
		require.Len(t, spans, 1)
		assert.EqualValues(t, 2, spans[0].AppID)
		var nTraces, nLinks int64
		require.NoError(t, db.Model(&AgentTraceTrace{}).Count(&nTraces).Error)
		require.NoError(t, db.Model(&AgentTraceSpanLink{}).Count(&nLinks).Error)
		assert.EqualValues(t, 1, nTraces)
		assert.EqualValues(t, 1, nLinks)
	})
}

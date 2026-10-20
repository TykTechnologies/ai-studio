package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
)

// Model-level helpers for the agent tracing tables. Business logic
// (correlation, rollup computation, retention policy) belongs to the
// Enterprise assembler; these only read and write rows, portably on SQLite
// and Postgres.
//
// Times are stored in UTC: SQLite keeps time.Time as text, and the keyset
// comparisons below only order correctly when every value has the same zone.

// InsertAgentTraceCaptureEvents appends capture events to the staging table,
// skipping any whose idempotency key is already there (a redelivered edge
// pulse). It returns the number of rows inserted. The IDs written back into
// events are not reliable when rows were skipped; consumers read IDs back
// with ListAgentTraceCaptureEventsAfter.
func InsertAgentTraceCaptureEvents(db *gorm.DB, events []AgentTraceCaptureEvent) (int64, error) {
	if len(events) == 0 {
		return 0, nil
	}
	for i := range events {
		events[i].ReceivedAt = events[i].ReceivedAt.UTC()
		if events[i].Size == 0 {
			events[i].Size = len(events[i].Event)
		}
	}
	res := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(&events)
	return res.RowsAffected, res.Error
}

// ListAgentTraceCaptureEventsAfter returns up to limit staged events with an
// ID above afterID, in ID order: the assembler's consume cursor.
func ListAgentTraceCaptureEventsAfter(db *gorm.DB, afterID uint64, limit int) ([]AgentTraceCaptureEvent, error) {
	var out []AgentTraceCaptureEvent
	err := db.Where("id > ?", afterID).Order("id ASC").Limit(limit).Find(&out).Error
	return out, err
}

// InsertAgentTraceSpans writes spans in multi-row batches of batchSize
// (500 when batchSize <= 0).
func InsertAgentTraceSpans(db *gorm.DB, spans []AgentTraceSpan, batchSize int) error {
	if len(spans) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	for i := range spans {
		spans[i].StartTime = spans[i].StartTime.UTC()
		if spans[i].EndTime != nil {
			t := spans[i].EndTime.UTC()
			spans[i].EndTime = &t
		}
	}
	return db.CreateInBatches(&spans, batchSize).Error
}

// AgentTraceSpanCursor is a keyset position in a (start_time, span_id) order.
type AgentTraceSpanCursor struct {
	StartTime time.Time
	SpanID    []byte
}

// ListAgentTraceSessionSpans returns up to limit spans of one session of one
// App, ordered by (start_time, span_id), starting after the cursor (nil for
// the first page). The App is part of the predicate so a session id from
// another App returns nothing.
func ListAgentTraceSessionSpans(db *gorm.DB, appID uint, sessionID string, after *AgentTraceSpanCursor, limit int) ([]AgentTraceSpan, error) {
	q := db.Where("app_id = ? AND session_id = ?", appID, sessionID)
	if after != nil {
		ts := after.StartTime.UTC()
		// The []byte argument must not follow "(" directly: gorm expands it
		// into a list there.
		q = q.Where("start_time > ? OR (start_time = ? AND span_id > ?)", ts, ts, after.SpanID)
	}
	var out []AgentTraceSpan
	err := q.Order("start_time ASC").Order("span_id ASC").Limit(limit).Find(&out).Error
	return out, err
}

// UpsertAgentTraceSessionRollup inserts s when the session is new, and
// otherwise adds s's counters, tokens and cost to the stored row as deltas
// and moves last_activity_at forward (never back). It is a single statement
// (INSERT ... ON CONFLICT DO UPDATE), so it needs no read inside a
// transaction. Every other column keeps its stored value on conflict.
func UpsertAgentTraceSessionRollup(db *gorm.DB, s *AgentTraceSession) error {
	s.StartedAt = s.StartedAt.UTC()
	s.LastActivityAt = s.LastActivityAt.UTC()
	s.RetentionUntil = s.RetentionUntil.UTC()
	const t = "agent_trace_sessions"
	set := addColumns(t, "turn_count", "span_count", "llm_call_count", "tool_call_count",
		"error_count", "dropped_count", "input_tokens", "output_tokens", "cost_micros")
	set = append(set, laterOf(t, "last_activity_at"))
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: set,
	}).Create(s).Error
}

// UpsertAgentTraceTraceRollup is UpsertAgentTraceSessionRollup for a trace:
// counters, tokens and cost are added as deltas.
func UpsertAgentTraceTraceRollup(db *gorm.DB, tr *AgentTraceTrace) error {
	tr.StartedAt = tr.StartedAt.UTC()
	const t = "agent_trace_traces"
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "app_id"}, {Name: "trace_id"}},
		DoUpdates: addColumns(t, "span_count", "llm_call_count", "tool_call_count",
			"error_count", "input_tokens", "output_tokens", "cost_micros"),
	}).Create(tr).Error
}

// addColumns builds "col = table.col + excluded.col" assignments.
func addColumns(table string, cols ...string) clause.Set {
	set := make(clause.Set, 0, len(cols))
	for _, c := range cols {
		set = append(set, clause.Assignment{
			Column: clause.Column{Name: c},
			Value:  gorm.Expr(fmt.Sprintf("%s.%s + excluded.%s", table, c, c)),
		})
	}
	return set
}

// laterOf keeps the later of the stored and the incoming time.
func laterOf(table, col string) clause.Assignment {
	return clause.Assignment{
		Column: clause.Column{Name: col},
		Value: gorm.Expr(fmt.Sprintf("CASE WHEN excluded.%s > %s.%s THEN excluded.%s ELSE %s.%s END",
			col, table, col, col, table, col)),
	}
}

// AgentTraceRetentionTarget names a table the retention job deletes from,
// the time column that decides expiry, and its primary key.
type AgentTraceRetentionTarget struct {
	Table string
	// PK is the primary key column, or a comma-separated list for a
	// composite key.
	PK     string
	Column string
	// AppColumn is the App scope column; empty when the table has none, in
	// which case an App-scoped delete is refused.
	AppColumn string
}

// Retention targets. Session rows expire at retention_until; their filter
// join rows go with them. Spans, traces and links expire by time, payloads
// and correlation keys at expires_at.
var (
	AgentTraceRetentionSessions        = AgentTraceRetentionTarget{"agent_trace_sessions", "id", "retention_until", "app_id"}
	AgentTraceRetentionTraces          = AgentTraceRetentionTarget{"agent_trace_traces", "app_id, trace_id", "started_at", "app_id"}
	AgentTraceRetentionSpans           = AgentTraceRetentionTarget{"agent_trace_spans", "app_id, span_id", "start_time", "app_id"}
	AgentTraceRetentionSpanLinks       = AgentTraceRetentionTarget{"agent_trace_span_links", "id", "span_start", "app_id"}
	AgentTraceRetentionAgents          = AgentTraceRetentionTarget{"agent_trace_agents", "id", "last_seen", ""}
	AgentTraceRetentionPayloads        = AgentTraceRetentionTarget{"agent_trace_payloads", "id", "expires_at", "app_id"}
	AgentTraceRetentionCorrelationKeys = AgentTraceRetentionTarget{"agent_trace_correlation_keys", "id", "expires_at", "app_id"}
	AgentTraceRetentionCaptureEvents   = AgentTraceRetentionTarget{"agent_trace_capture_events", "id", "received_at", ""}
)

// ErrAgentTraceNoAppScope is returned for an App-scoped delete on a table
// without an App column.
var ErrAgentTraceNoAppScope = errors.New("agent trace table has no app column")

// DeleteAgentTraceRowsBefore deletes at most limit rows of target whose
// expiry column is before cutoff (and, when appID is set, that belong to that
// App), returning the number deleted. Call it until it returns fewer than
// limit. Each call is a short statement of its own, so retention never holds
// a long lock; it is idempotent, so no transaction is needed.
func DeleteAgentTraceRowsBefore(db *gorm.DB, target AgentTraceRetentionTarget, cutoff time.Time, appID *uint, limit int) (int64, error) {
	if limit <= 0 {
		limit = 1000
	}
	if appID != nil && target.AppColumn == "" {
		return 0, ErrAgentTraceNoAppScope
	}
	sub := db.Table(target.Table).Select(target.PK).Where(target.Column+" < ?", cutoff.UTC())
	if appID != nil {
		sub = sub.Where(target.AppColumn+" = ?", *appID)
	}
	sub = sub.Limit(limit)

	if target.Table == AgentTraceRetentionSessions.Table {
		// Pick the ids once so the filter rows and the sessions match.
		var ids []string
		if err := sub.Pluck(target.PK, &ids).Error; err != nil {
			return 0, err
		}
		if len(ids) == 0 {
			return 0, nil
		}
		for _, m := range []interface{}{&AgentTraceSessionModel{}, &AgentTraceSessionTool{}, &AgentTraceSessionAgent{}} {
			if err := db.Where("session_id IN ?", ids).Delete(m).Error; err != nil {
				return 0, err
			}
		}
		res := db.Where("id IN ?", ids).Delete(&AgentTraceSession{})
		return res.RowsAffected, res.Error
	}

	// Composite keys (traces, spans) use a row-value IN, which SQLite (3.15+)
	// and Postgres both support.
	pk := target.PK
	if strings.Contains(pk, ",") {
		pk = "(" + pk + ")"
	}
	res := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE %s IN (?)", target.Table, pk), sub)
	return res.RowsAffected, res.Error
}

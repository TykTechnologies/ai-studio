package models

import (
	"errors"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Read-side helpers for the agent tracing API (spec §6.1, §8). Every query
// is scoped by App: the caller passes the visible Apps (or AllApps), and a
// row of another App is never returned. Filtering, ordering and paging run
// in SQL, portably on SQLite and Postgres.
//
// A []byte argument must not follow "(" directly: gorm expands it into a
// list there, so the predicates below put a space or a column first.

// AgentTraceAppScope is the set of Apps a reader may see.
type AgentTraceAppScope struct {
	AllApps bool
	AppIDs  []uint
}

func (s AgentTraceAppScope) apply(q *gorm.DB, column string) *gorm.DB {
	if s.AllApps {
		return q
	}
	if len(s.AppIDs) == 0 {
		return q.Where("1 = 0")
	}
	return q.Where(column+" IN ?", s.AppIDs)
}

// AgentTraceSessionFilter is the sessions list filter. Zero values do not
// filter; From/To bound started_at (From inclusive, To exclusive).
type AgentTraceSessionFilter struct {
	Scope            AgentTraceAppScope
	From, To         time.Time
	AppID            uint
	UserID           uint
	EndUser          string
	ExternalID       string
	SessionID        string
	Agent            string
	Model            string
	Tool             string
	Status           string
	Source           string
	WeakLinks        bool
	HasErrors        *bool
	HasGatewayEvents *bool
	CostMinMicros    *int64
	CostMaxMicros    *int64
	DurationMinMs    *int64
	DurationMaxMs    *int64
}

// AgentTraceSessionSortColumns maps the API's sort names to SQL. Every
// column is integral or a time, so a keyset position is (value, id).
var AgentTraceSessionSortColumns = map[string]string{
	"started_at": "started_at",
	"cost":       "cost_micros",
	"duration":   "duration_ms",
	"tokens":     "(input_tokens + output_tokens)",
	"turns":      "turn_count",
	"llm_calls":  "llm_call_count",
	"tool_calls": "tool_call_count",
	"errors":     "error_count",
}

// AgentTraceSessionKey is a keyset position in a session order: the sort
// value (Time for started_at, Int otherwise) and the session id breaking
// ties.
type AgentTraceSessionKey struct {
	Time time.Time
	Int  int64
	ID   string
}

// SessionKey returns s's position in the order sorted by sort.
func (s *AgentTraceSession) SessionKey(sort string) AgentTraceSessionKey {
	k := AgentTraceSessionKey{ID: s.ID}
	switch sort {
	case "started_at":
		k.Time = s.StartedAt.UTC()
	case "cost":
		k.Int = s.CostMicros
	case "duration":
		k.Int = s.DurationMs
	case "tokens":
		k.Int = s.InputTokens + s.OutputTokens
	case "turns":
		k.Int = int64(s.TurnCount)
	case "llm_calls":
		k.Int = int64(s.LLMCallCount)
	case "tool_calls":
		k.Int = int64(s.ToolCallCount)
	case "errors":
		k.Int = int64(s.ErrorCount)
	}
	return k
}

// ErrAgentTraceUnknownSort is returned for a sort name not in
// AgentTraceSessionSortColumns.
var ErrAgentTraceUnknownSort = errors.New("unknown agent trace session sort")

// ListAgentTraceSessions returns up to limit sessions matching f, ordered by
// sort (desc reverses it) and then id, starting after the key (nil for the
// first page).
func ListAgentTraceSessions(db *gorm.DB, f AgentTraceSessionFilter, sort string, desc bool, after *AgentTraceSessionKey, limit int) ([]AgentTraceSession, error) {
	q, err := listSessionsQuery(db, f, sort, desc, after, limit)
	if err != nil {
		return nil, err
	}
	var out []AgentTraceSession
	err = q.Find(&out).Error
	return out, err
}

// listSessionsQuery builds the ListAgentTraceSessions statement.
func listSessionsQuery(db *gorm.DB, f AgentTraceSessionFilter, sort string, desc bool, after *AgentTraceSessionKey, limit int) (*gorm.DB, error) {
	col, ok := AgentTraceSessionSortColumns[sort]
	if !ok {
		return nil, ErrAgentTraceUnknownSort
	}
	q := f.Scope.apply(db.Model(&AgentTraceSession{}), "app_id").
		Where("started_at >= ? AND started_at < ?", f.From.UTC(), f.To.UTC())
	if f.AppID != 0 {
		q = q.Where("app_id = ?", f.AppID)
	}
	if f.UserID != 0 {
		q = q.Where("user_id = ?", f.UserID)
	}
	for column, v := range map[string]string{
		"end_user": f.EndUser, "external_id": f.ExternalID, "id": f.SessionID, "status": f.Status, "source": f.Source,
	} {
		if v != "" {
			q = q.Where(column+" = ?", v)
		}
	}
	for table, v := range map[string]string{
		"agent_trace_session_agents": f.Agent, "agent_trace_session_models": f.Model, "agent_trace_session_tools": f.Tool,
	} {
		if v != "" {
			q = q.Where("id IN (SELECT session_id FROM "+table+" WHERE name = ?)", v)
		}
	}
	if f.WeakLinks {
		q = q.Where("min_confidence = ?", "weak")
	}
	if f.HasErrors != nil {
		if *f.HasErrors {
			q = q.Where("error_count > 0")
		} else {
			q = q.Where("error_count = 0")
		}
	}
	if f.HasGatewayEvents != nil {
		if *f.HasGatewayEvents {
			q = q.Where("gateway_event_count > 0")
		} else {
			q = q.Where("gateway_event_count = 0")
		}
	}
	for cond, v := range map[string]*int64{
		"cost_micros >= ?": f.CostMinMicros, "cost_micros <= ?": f.CostMaxMicros,
		"duration_ms >= ?": f.DurationMinMs, "duration_ms <= ?": f.DurationMaxMs,
	} {
		if v != nil {
			q = q.Where(cond, *v)
		}
	}
	op, dir := ">", "ASC"
	if desc {
		op, dir = "<", "DESC"
	}
	if after != nil {
		var v any = after.Int
		if sort == "started_at" {
			v = after.Time.UTC()
		}
		q = q.Where(col+" "+op+" ? OR ("+col+" = ? AND id "+op+" ?)", v, v, after.ID)
	}
	return q.Order(col + " " + dir).Order("id " + dir).Limit(limit), nil
}

// GetAgentTraceSession returns one session in scope, or gorm.ErrRecordNotFound.
func GetAgentTraceSession(db *gorm.DB, scope AgentTraceAppScope, id string) (*AgentTraceSession, error) {
	var s AgentTraceSession
	err := scope.apply(db, "app_id").Where("id = ?", id).First(&s).Error
	return &s, err
}

// AgentTraceSessionNames are the agent, model and tool names of sessions.
type AgentTraceSessionNames struct {
	Agents, Models, Tools map[string][]string
}

// ListAgentTraceSessionNames returns the names of the given sessions, each
// list sorted, in three queries.
func ListAgentTraceSessionNames(db *gorm.DB, sessionIDs []string) (AgentTraceSessionNames, error) {
	out := AgentTraceSessionNames{Agents: map[string][]string{}, Models: map[string][]string{}, Tools: map[string][]string{}}
	if len(sessionIDs) == 0 {
		return out, nil
	}
	type row struct{ SessionID, Name string }
	for table, dst := range map[string]map[string][]string{
		"agent_trace_session_agents": out.Agents, "agent_trace_session_models": out.Models, "agent_trace_session_tools": out.Tools,
	} {
		var rows []row
		if err := db.Table(table).Select("session_id, name").Where("session_id IN ?", sessionIDs).
			Order("session_id").Order("name").Scan(&rows).Error; err != nil {
			return out, err
		}
		for _, r := range rows {
			dst[r.SessionID] = append(dst[r.SessionID], r.Name)
		}
	}
	return out, nil
}

// CountAgentTraceSessionConfidence counts the parent edges of the given
// sessions' spans by confidence (declared, exact, strong, weak), in one
// query.
func CountAgentTraceSessionConfidence(db *gorm.DB, sessionIDs []string) (map[string]map[string]int, error) {
	out := map[string]map[string]int{}
	if len(sessionIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		SessionID        string
		ParentConfidence string
		N                int
	}
	err := db.Model(&AgentTraceSpan{}).Select("session_id, parent_confidence, COUNT(*) AS n").
		Where("session_id IN ? AND parent_confidence <> ''", sessionIDs).
		Group("session_id").Group("parent_confidence").Scan(&rows).Error
	for _, r := range rows {
		if out[r.SessionID] == nil {
			out[r.SessionID] = map[string]int{}
		}
		out[r.SessionID][r.ParentConfidence] = r.N
	}
	return out, err
}

// AgentTraceSpanFilter narrows a span page: one session or one trace of one
// App, optionally by kind, status, agent and start time (From inclusive, To
// exclusive).
type AgentTraceSpanFilter struct {
	AppID     uint
	SessionID string
	TraceID   []byte
	Kind      string
	Status    string
	AgentRef  string
	From, To  time.Time
}

// ListAgentTraceSpans returns up to limit spans matching f, ordered by
// (start_time, span_id), starting after the cursor (nil for the first page).
func ListAgentTraceSpans(db *gorm.DB, f AgentTraceSpanFilter, after *AgentTraceSpanCursor, limit int) ([]AgentTraceSpan, error) {
	q := db.Where("app_id = ?", f.AppID)
	if f.SessionID != "" {
		q = q.Where("session_id = ?", f.SessionID)
	}
	if len(f.TraceID) > 0 {
		q = q.Where("trace_id = ?", f.TraceID)
	}
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.AgentRef != "" {
		q = q.Where("agent_ref = ?", f.AgentRef)
	}
	if !f.From.IsZero() {
		q = q.Where("start_time >= ?", f.From.UTC())
	}
	if !f.To.IsZero() {
		q = q.Where("start_time < ?", f.To.UTC())
	}
	if after != nil {
		ts := after.StartTime.UTC()
		q = q.Where("start_time > ? OR (start_time = ? AND span_id > ?)", ts, ts, after.SpanID)
	}
	var out []AgentTraceSpan
	err := q.Order("start_time ASC").Order("span_id ASC").Limit(limit).Find(&out).Error
	return out, err
}

// GetAgentTraceSpan returns the span (appID, spanID) when appID is in
// scope, or gorm.ErrRecordNotFound. Span ids are unique only within an App.
func GetAgentTraceSpan(db *gorm.DB, scope AgentTraceAppScope, appID uint, spanID []byte) (*AgentTraceSpan, error) {
	var s AgentTraceSpan
	err := scope.apply(db, "app_id").Where("app_id = ? AND span_id = ?", appID, spanID).First(&s).Error
	return &s, err
}

// GetAgentTraceTrace returns the trace (appID, traceID) when appID is in
// scope, or gorm.ErrRecordNotFound. Trace ids are unique only within an App.
func GetAgentTraceTrace(db *gorm.DB, scope AgentTraceAppScope, appID uint, traceID []byte) (*AgentTraceTrace, error) {
	var t AgentTraceTrace
	err := scope.apply(db, "app_id").Where("app_id = ? AND trace_id = ?", appID, traceID).First(&t).Error
	return &t, err
}

// ListAgentTraceSpanLinks returns the links of the given spans of one App;
// with withCandidates false the candidate links (spec §3.3) are left out.
func ListAgentTraceSpanLinks(db *gorm.DB, appID uint, spanIDs [][]byte, withCandidates bool) ([]AgentTraceSpanLink, error) {
	if len(spanIDs) == 0 {
		return nil, nil
	}
	q := db.Where("app_id = ? AND span_id IN ?", appID, spanIDs)
	if !withCandidates {
		q = q.Where("link_type <> ?", "candidate")
	}
	var out []AgentTraceSpanLink
	err := q.Order("id").Find(&out).Error
	return out, err
}

// ListAgentTracePayloadMeta returns the payload rows of the given spans of
// one App without their data: content states and sizes only.
func ListAgentTracePayloadMeta(db *gorm.DB, appID uint, spanIDs [][]byte) ([]AgentTracePayload, error) {
	if len(spanIDs) == 0 {
		return nil, nil
	}
	var out []AgentTracePayload
	err := db.Omit("data").Where("app_id = ? AND span_id IN ?", appID, spanIDs).Find(&out).Error
	return out, err
}

// ListAgentTraceChildEvents returns the gateway_event spans whose parent is
// spanID, in start order.
func ListAgentTraceChildEvents(db *gorm.DB, appID uint, sessionID string, spanID []byte) ([]AgentTraceSpan, error) {
	var out []AgentTraceSpan
	err := db.Where("app_id = ? AND session_id = ? AND kind = ? AND parent_span_id = ?", appID, sessionID, "gateway_event", spanID).
		Order("start_time ASC").Order("span_id ASC").Limit(1000).Find(&out).Error
	return out, err
}

// SearchAgentTraceSessionSpans returns up to limit spans of one session of
// one App whose name, tool name, models, operation or error type contains
// term (case-insensitively), in start order, and the total number of
// matches. It reads metadata only: never attributes, error messages or
// payloads, which can hold captured content.
func SearchAgentTraceSessionSpans(db *gorm.DB, appID uint, sessionID, term string, limit int) ([]AgentTraceSpan, int64, error) {
	like := "%" + escapeLike(lowerASCII(term)) + "%"
	cond := "(LOWER(name) LIKE ? ESCAPE '\\' OR LOWER(tool_name) LIKE ? ESCAPE '\\' OR LOWER(request_model) LIKE ? ESCAPE '\\'" +
		" OR LOWER(response_model) LIKE ? ESCAPE '\\' OR LOWER(operation) LIKE ? ESCAPE '\\' OR LOWER(error_type) LIKE ? ESCAPE '\\')"
	args := []any{like, like, like, like, like, like}
	base := func() *gorm.DB {
		return db.Model(&AgentTraceSpan{}).Where("app_id = ? AND session_id = ?", appID, sessionID).Where(cond, args...)
	}
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []AgentTraceSpan
	err := base().Order("start_time ASC").Order("span_id ASC").Limit(limit).Find(&out).Error
	return out, total, err
}

func escapeLike(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\', '%', '_':
			out = append(out, '\\')
		}
		out = append(out, s[i])
	}
	return string(out)
}

// lowerASCII lowers ASCII letters only, matching SQL LOWER on both SQLite
// (ASCII only) and Postgres for the identifiers searched here.
func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// ListAgentTraceTracesToExport returns up to limit turns not yet exported
// that have settled (spec §5.6): closed, or started before cutoff (now minus
// export.max_delay), oldest first.
func ListAgentTraceTracesToExport(db *gorm.DB, cutoff time.Time, limit int) ([]AgentTraceTrace, error) {
	var out []AgentTraceTrace
	err := db.Where("exported_at IS NULL").
		Where("status <> ? OR started_at < ?", "in_progress", cutoff.UTC()).
		Order("started_at ASC").Limit(limit).Find(&out).Error
	return out, err
}

// MarkAgentTraceExported records that a turn was exported.
func MarkAgentTraceExported(db *gorm.DB, appID uint, traceID []byte, at time.Time) error {
	return db.Model(&AgentTraceTrace{}).Where("app_id = ? AND trace_id = ?", appID, traceID).
		Update("exported_at", at.UTC()).Error
}

// AgentTraceResourceNames returns display names of the given ids of one
// resource type (llm, tool, datasource, plugin, mcp_server), in one query.
// Unknown types and ids are left out.
func AgentTraceResourceNames(db *gorm.DB, resourceType string, ids []uint) (map[uint]string, error) {
	table := map[string]string{
		"llm": "llms", "tool": "tools", "datasource": "datasources", "plugin": "plugins", "mcp_server": "mcp_servers",
	}[resourceType]
	out := map[uint]string{}
	if table == "" || len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID   uint
		Name string
	}
	err := db.Table(table).Select("id, name").Where("id IN ? AND deleted_at IS NULL", ids).Scan(&rows).Error
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out, err
}

// AgentTraceNames returns display names of Apps or users by id, in one
// query. kind is "app" or "user".
func AgentTraceNames(db *gorm.DB, kind string, ids []uint) (map[uint]string, error) {
	table := map[string]string{"app": "apps", "user": "users"}[kind]
	out := map[uint]string{}
	if table == "" || len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID   uint
		Name string
	}
	err := db.Table(table).Select("id, name").Where("id IN ? AND deleted_at IS NULL", ids).Scan(&rows).Error
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out, err
}

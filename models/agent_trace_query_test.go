package models

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var queryBase = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func seedQuerySession(t *testing.T, db *gorm.DB, id string, appID uint, started time.Time, cost int64, mut func(*AgentTraceSession)) {
	t.Helper()
	s := AgentTraceSession{
		ID: id, AppID: appID, Source: "declared", Status: "closed",
		StartedAt: started, LastActivityAt: started, CostMicros: cost,
		Sampled: true, ContentMode: "metadata", RetentionUntil: started.Add(30 * 24 * time.Hour),
	}
	if mut != nil {
		mut(&s)
	}
	require.NoError(t, db.Create(&s).Error)
}

func sid(n int) string { return fmt.Sprintf("0192a4c8-7f00-7000-8000-%012d", n) }

// Keyset paging visits every visible session exactly once, in order, even
// with ties on the sort value, and never another App's.
func TestListAgentTraceSessions_KeysetWithTies(t *testing.T) {
	agentTraceDBs(t, func(t *testing.T, db *gorm.DB) {
		// Ten sessions of App 1 sharing three start times and two costs;
		// two of App 2 in the same window.
		for i := 0; i < 10; i++ {
			seedQuerySession(t, db, sid(i), 1, queryBase.Add(time.Duration(i%3)*time.Minute), int64(i%2)*100, nil)
		}
		seedQuerySession(t, db, sid(100), 2, queryBase, 5, nil)
		seedQuerySession(t, db, sid(101), 2, queryBase, 5, nil)

		scope := AgentTraceAppScope{AppIDs: []uint{1}}
		f := AgentTraceSessionFilter{Scope: scope, From: queryBase.Add(-time.Hour), To: queryBase.Add(time.Hour)}
		for _, tc := range []struct {
			sort string
			desc bool
		}{{"started_at", true}, {"started_at", false}, {"cost", true}, {"cost", false}, {"tokens", true}} {
			var seen []string
			var after *AgentTraceSessionKey
			var prev *AgentTraceSession
			for page := 0; page < 20; page++ {
				rows, err := ListAgentTraceSessions(db, f, tc.sort, tc.desc, after, 3)
				require.NoError(t, err)
				for i := range rows {
					r := rows[i]
					assert.Equal(t, uint(1), r.AppID, "another App's session leaked")
					if prev != nil {
						a, b := prev.SessionKey(tc.sort), r.SessionKey(tc.sort)
						cmp := compareKeys(a, b, tc.sort)
						if tc.desc {
							assert.Positive(t, cmp, "%s desc out of order: %v then %v", tc.sort, a, b)
						} else {
							assert.Negative(t, cmp, "%s asc out of order: %v then %v", tc.sort, a, b)
						}
					}
					prev = &rows[i]
					seen = append(seen, r.ID)
				}
				if len(rows) < 3 {
					break
				}
				k := rows[len(rows)-1].SessionKey(tc.sort)
				after = &k
			}
			assert.Len(t, seen, 10, "%s desc=%v", tc.sort, tc.desc)
			uniq := map[string]bool{}
			for _, id := range seen {
				uniq[id] = true
			}
			assert.Len(t, uniq, 10, "%s desc=%v visited a session twice", tc.sort, tc.desc)
		}

		all, err := ListAgentTraceSessions(db, AgentTraceSessionFilter{Scope: AgentTraceAppScope{AllApps: true},
			From: queryBase.Add(-time.Hour), To: queryBase.Add(time.Hour)}, "started_at", true, nil, 100)
		require.NoError(t, err)
		assert.Len(t, all, 12)
		none, err := ListAgentTraceSessions(db, AgentTraceSessionFilter{From: queryBase.Add(-time.Hour), To: queryBase.Add(time.Hour)},
			"started_at", true, nil, 100)
		require.NoError(t, err)
		assert.Empty(t, none, "an empty scope sees nothing")

		_, err = ListAgentTraceSessions(db, f, "name", true, nil, 10)
		assert.True(t, errors.Is(err, ErrAgentTraceUnknownSort))
	})
}

func compareKeys(a, b AgentTraceSessionKey, sort string) int {
	if sort == "started_at" {
		if !a.Time.Equal(b.Time) {
			if a.Time.Before(b.Time) {
				return -1
			}
			return 1
		}
	} else if a.Int != b.Int {
		if a.Int < b.Int {
			return -1
		}
		return 1
	}
	switch {
	case a.ID < b.ID:
		return -1
	case a.ID > b.ID:
		return 1
	}
	return 0
}

func TestListAgentTraceSessions_Filters(t *testing.T) {
	agentTraceDBs(t, func(t *testing.T, db *gorm.DB) {
		seedQuerySession(t, db, sid(1), 1, queryBase, 10, func(s *AgentTraceSession) {
			s.ErrorCount, s.GatewayEventCount, s.DurationMs, s.MinConfidence = 2, 1, 5000, "weak"
			s.EndUser, s.ExternalID, s.Source, s.Status, s.UserID = "alice@example.com", "conv-1", "inferred", "error", 7
		})
		seedQuerySession(t, db, sid(2), 1, queryBase.Add(time.Minute), 500, func(s *AgentTraceSession) {
			s.DurationMs, s.MinConfidence = 60000, "exact"
		})
		seedQuerySession(t, db, sid(3), 1, queryBase.Add(-2*time.Hour), 10, nil) // outside the window
		seedQuerySession(t, db, sid(4), 2, queryBase, 10, nil)
		require.NoError(t, db.Create(&AgentTraceSessionModel{SessionID: sid(1), Name: "gpt-4.1"}).Error)
		require.NoError(t, db.Create(&AgentTraceSessionTool{SessionID: sid(2), Name: "web_search"}).Error)
		require.NoError(t, db.Create(&AgentTraceSessionAgent{SessionID: sid(2), Name: "planner"}).Error)

		yes, no := true, false
		n := func(v int64) *int64 { return &v }
		base := AgentTraceSessionFilter{Scope: AgentTraceAppScope{AllApps: true}, From: queryBase.Add(-time.Hour), To: queryBase.Add(time.Hour)}
		ids := func(f AgentTraceSessionFilter) []string {
			rows, err := ListAgentTraceSessions(db, f, "started_at", false, nil, 100)
			require.NoError(t, err)
			var out []string
			for _, r := range rows {
				out = append(out, r.ID)
			}
			return out
		}
		with := func(mut func(*AgentTraceSessionFilter)) AgentTraceSessionFilter {
			f := base
			mut(&f)
			return f
		}
		assert.Equal(t, []string{sid(1), sid(4), sid(2)}, ids(base))
		assert.Equal(t, []string{sid(1), sid(2)}, ids(with(func(f *AgentTraceSessionFilter) { f.AppID = 1 })))
		assert.Equal(t, []string{sid(1)}, ids(with(func(f *AgentTraceSessionFilter) { f.Model = "gpt-4.1" })))
		assert.Equal(t, []string{sid(2)}, ids(with(func(f *AgentTraceSessionFilter) { f.Tool = "web_search" })))
		assert.Equal(t, []string{sid(2)}, ids(with(func(f *AgentTraceSessionFilter) { f.Agent = "planner" })))
		assert.Equal(t, []string{sid(1)}, ids(with(func(f *AgentTraceSessionFilter) { f.HasErrors = &yes })))
		assert.Equal(t, []string{sid(4), sid(2)}, ids(with(func(f *AgentTraceSessionFilter) { f.HasErrors = &no })))
		assert.Equal(t, []string{sid(1)}, ids(with(func(f *AgentTraceSessionFilter) { f.HasGatewayEvents = &yes })))
		assert.Equal(t, []string{sid(1)}, ids(with(func(f *AgentTraceSessionFilter) { f.WeakLinks = true })))
		assert.Equal(t, []string{sid(1)}, ids(with(func(f *AgentTraceSessionFilter) { f.EndUser = "alice@example.com" })))
		assert.Equal(t, []string{sid(1)}, ids(with(func(f *AgentTraceSessionFilter) { f.ExternalID = "conv-1" })))
		assert.Equal(t, []string{sid(2)}, ids(with(func(f *AgentTraceSessionFilter) { f.SessionID = sid(2) })))
		assert.Equal(t, []string{sid(1)}, ids(with(func(f *AgentTraceSessionFilter) { f.Status, f.Source = "error", "inferred" })))
		assert.Equal(t, []string{sid(1)}, ids(with(func(f *AgentTraceSessionFilter) { f.UserID = 7 })))
		assert.Equal(t, []string{sid(2)}, ids(with(func(f *AgentTraceSessionFilter) { f.CostMinMicros = n(100) })))
		assert.Equal(t, []string{sid(1), sid(4)}, ids(with(func(f *AgentTraceSessionFilter) { f.CostMaxMicros = n(10) })))
		assert.Equal(t, []string{sid(2)}, ids(with(func(f *AgentTraceSessionFilter) { f.DurationMinMs = n(10000) })))
		assert.Equal(t, []string{sid(1)}, ids(with(func(f *AgentTraceSessionFilter) { f.DurationMinMs, f.DurationMaxMs = n(1), n(9999) })))
		assert.Empty(t, ids(with(func(f *AgentTraceSessionFilter) { f.Scope = AgentTraceAppScope{AppIDs: []uint{2}}; f.AppID = 1 })),
			"an App filter cannot widen the scope")

		got, err := GetAgentTraceSession(db, AgentTraceAppScope{AppIDs: []uint{2}}, sid(1))
		assert.True(t, errors.Is(err, gorm.ErrRecordNotFound), "another App's session is not found: %+v", got)
		_, err = GetAgentTraceSession(db, AgentTraceAppScope{AppIDs: []uint{1}}, sid(1))
		assert.NoError(t, err)

		names, err := ListAgentTraceSessionNames(db, []string{sid(1), sid(2)})
		require.NoError(t, err)
		assert.Equal(t, []string{"gpt-4.1"}, names.Models[sid(1)])
		assert.Equal(t, []string{"web_search"}, names.Tools[sid(2)])
		assert.Equal(t, []string{"planner"}, names.Agents[sid(2)])
	})
}

func seedQuerySpan(t *testing.T, db *gorm.DB, appID uint, sessionID string, id byte, start time.Time, mut func(*AgentTraceSpan)) {
	t.Helper()
	s := AgentTraceSpan{
		AppID: appID, SpanID: []byte{0, 0, 0, 0, 0, 0, 0, id}, TraceID: []byte("0123456789abcdef"), SessionID: sessionID,
		Kind: "llm_inference", Name: "chat", StartTime: start, Status: "ok", ContentState: "metadata",
	}
	if mut != nil {
		mut(&s)
	}
	require.NoError(t, db.Create(&s).Error)
}

func TestListAgentTraceSpans_FiltersAndScope(t *testing.T) {
	agentTraceDBs(t, func(t *testing.T, db *gorm.DB) {
		for i := byte(1); i <= 5; i++ {
			seedQuerySpan(t, db, 1, sid(1), i, queryBase.Add(time.Duration(i%2)*time.Second), func(s *AgentTraceSpan) {
				if i%2 == 0 {
					s.Kind, s.ToolName, s.Status = "tool_call", "Web_Search", "error"
				}
			})
		}
		seedQuerySpan(t, db, 2, sid(1), 9, queryBase, nil) // same session id, another App

		f := AgentTraceSpanFilter{AppID: 1, SessionID: sid(1)}
		var got [][]byte
		var after *AgentTraceSpanCursor
		for {
			page, err := ListAgentTraceSpans(db, f, after, 2)
			require.NoError(t, err)
			for _, s := range page {
				got = append(got, s.SpanID)
			}
			if len(page) < 2 {
				break
			}
			last := page[len(page)-1]
			after = &AgentTraceSpanCursor{StartTime: last.StartTime, SpanID: last.SpanID}
		}
		require.Len(t, got, 5)
		assert.Equal(t, byte(2), got[0][7], "ties on start_time order by span_id")

		tools, err := ListAgentTraceSpans(db, AgentTraceSpanFilter{AppID: 1, SessionID: sid(1), Kind: "tool_call", Status: "error"}, nil, 10)
		require.NoError(t, err)
		assert.Len(t, tools, 2)
		windowed, err := ListAgentTraceSpans(db, AgentTraceSpanFilter{AppID: 1, SessionID: sid(1), From: queryBase.Add(time.Second)}, nil, 10)
		require.NoError(t, err)
		assert.Len(t, windowed, 3)

		_, err = GetAgentTraceSpan(db, AgentTraceAppScope{AppIDs: []uint{2}}, []byte{0, 0, 0, 0, 0, 0, 0, 1})
		assert.True(t, errors.Is(err, gorm.ErrRecordNotFound))
		sp, err := GetAgentTraceSpan(db, AgentTraceAppScope{AllApps: true}, []byte{0, 0, 0, 0, 0, 0, 0, 1})
		require.NoError(t, err)
		assert.Equal(t, uint(1), sp.AppID)

		// Find matches names case-insensitively, treats % and _ literally,
		// and never reads attributes.
		require.NoError(t, db.Model(&AgentTraceSpan{}).Where("app_id = 1 AND span_id = ?", []byte{0, 0, 0, 0, 0, 0, 0, 1}).
			Update("attributes", RawJSON(`{"gen_ai.input.messages":"secret web_search plan"}`)).Error)
		matches, total, err := SearchAgentTraceSessionSpans(db, 1, sid(1), "WEB_SEARCH", 1)
		require.NoError(t, err)
		assert.EqualValues(t, 2, total)
		assert.Len(t, matches, 1)
		_, total, err = SearchAgentTraceSessionSpans(db, 1, sid(1), "secret", 10)
		require.NoError(t, err)
		assert.Zero(t, total, "attributes may hold content and are not searched")
		_, total, err = SearchAgentTraceSessionSpans(db, 1, sid(1), "%", 10)
		require.NoError(t, err)
		assert.Zero(t, total, "% is literal")
		_, total, err = SearchAgentTraceSessionSpans(db, 2, sid(1), "chat", 10)
		require.NoError(t, err)
		assert.EqualValues(t, 1, total, "only the App's own spans")
	})
}

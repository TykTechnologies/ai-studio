package models

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/require"
)

// TestAgentTraceSessionListPlan_1M checks spec US-1 on Postgres: the first
// page of the sessions list with 1M sessions in range is well under 2 s. It
// loads 1M synthetic sessions (50 Apps, one model name each), so it runs
// only when AGENT_TRACE_BENCH_DSN names a scratch Postgres database:
//
//	AGENT_TRACE_BENCH_DSN=postgres://... go test ./models -run SessionListPlan -v
//
// It logs each query's time and EXPLAIN ANALYZE plan.
func TestAgentTraceSessionListPlan_1M(t *testing.T) {
	dsn := os.Getenv("AGENT_TRACE_BENCH_DSN")
	if dsn == "" {
		t.Skip("AGENT_TRACE_BENCH_DSN not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	schema := fmt.Sprintf("agent_trace_bench_%d", time.Now().UnixNano())
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() { db.Exec("DROP SCHEMA " + schema + " CASCADE") })
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err = gorm.Open(postgres.Open(dsn+sep+"search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(AnalyticsModels()...))

	const n = 1000000
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	// One session every ~86 ms over the 24 h before end: all 1M are in the
	// default range.
	require.NoError(t, db.Exec(`
INSERT INTO agent_trace_sessions (id, app_id, user_id, source, status, started_at, last_activity_at,
  turn_count, span_count, llm_call_count, tool_call_count, error_count, dropped_count, input_tokens, output_tokens,
  cost_micros, gateway_event_count, duration_ms, min_confidence, sampled, content_mode, retention_until)
SELECT lpad(to_hex(g), 8, '0') || '-0000-7000-8000-' || lpad(to_hex(g), 12, '0'),
  (g % 50) + 1, (g % 500) + 1, CASE WHEN g % 3 = 0 THEN 'inferred' ELSE 'declared' END,
  CASE WHEN g % 20 = 0 THEN 'error' ELSE 'closed' END,
  ?::timestamptz - (g * interval '86 milliseconds'), ?::timestamptz - (g * interval '86 milliseconds') + interval '30 seconds',
  1 + g % 7, 10 + g % 50, 3 + g % 9, g % 6, CASE WHEN g % 20 = 0 THEN 1 ELSE 0 END, 0, g % 5000, g % 900,
  g % 100000, g % 4, 30000 + g % 60000, CASE WHEN g % 10 = 0 THEN 'weak' ELSE 'exact' END, true, 'metadata',
  ?::timestamptz + interval '30 days'
FROM generate_series(1, ?) AS g`, end, end, end, n).Error)
	require.NoError(t, db.Exec(`INSERT INTO agent_trace_session_models (session_id, name)
SELECT id, 'model-' || (app_id % 10) FROM agent_trace_sessions`).Error)
	require.NoError(t, db.Exec("ANALYZE").Error)

	base := AgentTraceSessionFilter{Scope: AgentTraceAppScope{AllApps: true}, From: end.Add(-24 * time.Hour), To: end}
	appScoped := base
	appScoped.Scope = AgentTraceAppScope{AppIDs: []uint{3, 4, 5}}
	byModel := base
	byModel.Model = "model-3"
	byApp := base
	byApp.AppID = 7
	errorsOnly := base
	yes := true
	errorsOnly.HasErrors = &yes
	for _, tc := range []struct {
		name string
		f    AgentTraceSessionFilter
		sort string
	}{
		{"default: all Apps, newest first", base, "started_at"},
		{"three visible Apps", appScoped, "started_at"},
		{"one App filter", byApp, "started_at"},
		{"model filter", byModel, "started_at"},
		{"has errors", errorsOnly, "started_at"},
		{"sort by cost", base, "cost"},
	} {
		start := time.Now()
		rows, err := ListAgentTraceSessions(db, tc.f, tc.sort, true, nil, 51)
		took := time.Since(start)
		require.NoError(t, err)
		require.Len(t, rows, 51, tc.name)
		var plan []string
		sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			var out []AgentTraceSession
			r, _ := listSessionsQuery(tx, tc.f, tc.sort, true, nil, 51)
			return r.Find(&out)
		})
		require.NoError(t, db.Raw("EXPLAIN (ANALYZE, BUFFERS) "+sql).Scan(&plan).Error)
		t.Logf("%s: %v\n%s", tc.name, took, strings.Join(plan, "\n"))
		require.Less(t, took, 2*time.Second, tc.name)
	}
}

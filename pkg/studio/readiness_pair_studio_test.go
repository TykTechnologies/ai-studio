//go:build !enterprise

package studio

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbstate"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// The readiness test that runs a whole Studio on the failover rig. It is
// Community only, as the other tests here that start a whole Studio are: in
// the Enterprise build Studio refuses to start without a license. The
// readiness tests on bare replicas (readiness_pair_test.go) run in both.

// startPairStudio runs a Studio on the pair's multi-host DSN with the
// given readiness grace and metrics on.
func startPairStudio(t *testing.T, pair *containers.PostgresPair, grace time.Duration) *Studio {
	t.Helper()
	dir := t.TempDir()
	vals := map[string]string{
		"DATABASE_TYPE":            "postgres",
		"DATABASE_URL":             pair.DSN(),
		"DATABASE_CONNECT_TIMEOUT": "2s",
		"DATABASE_READY_GRACE":     grace.String(),
		"METRICS_ENABLED":          "true",
		"TELEMETRY_ENABLED":        "false",
		"MARKETPLACE_ENABLED":      "false",
		"DOCS_DISABLED":            "true",
		"TYK_AI_SECRET_KEY":        "readiness-test-secret-key",
		"EXPORT_STORAGE_PATH":      filepath.Join(dir, "exports"),
		"BRANDING_STORAGE_PATH":    filepath.Join(dir, "branding"),
		"SKIP_FILTER_DEFAULTS":     "true",
	}
	conf := config.LoadFrom(func(k string) string { return vals[k] })
	t.Cleanup(config.ResetGlobalConfig)
	require.Equal(t, grace, conf.DatabaseReadyGrace)

	db, err := OpenDatabase(conf)
	require.NoError(t, err)
	t.Cleanup(func() {
		if pool, err := db.DB(); err == nil {
			pool.Close()
		}
	})
	s, err := New(Options{
		Config: conf, DB: db, Version: "test", SkipLLMDefaults: true,
		UIAssets: fstest.MapFS{"index.html": {Data: []byte("<html>studio</html>")}},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		s.Stop(context.Background())
		metrics.SetDatabaseState(nil)
	})
	return s
}

// A failover shorter than the grace keeps /ready 200 on both replicas
// throughout, /health never changes, and the metrics follow: the gauge
// drops to 0 while there is no primary and the counter counts the
// failover.
func TestFailover_ReadinessKeepsReplicasThroughAFailover(t *testing.T) {
	pair := containers.StartPostgresPair(t, &containers.PostgresPairConfig{Synchronous: true})
	ctx := context.Background()
	const grace = 30 * time.Second

	s := startPairStudio(t, pair, grace)
	h := s.HTTPHandler()
	tracker := dbstate.For(s.db)
	require.NotNil(t, tracker)

	other := openFailoverDB(t, pair)
	otherReady := bareReplica(other, grace)

	require.Equal(t, "1", metricSample(t, "aistudio_db_writable"))
	require.Equal(t, "0", metricSample(t, "aistudio_db_failovers_total"))

	p := startProber(t, map[string]func() probeAnswer{
		"studio /ready":                  func() probeAnswer { return get(t, h, "/ready") },
		"studio /readyz":                 func() probeAnswer { return get(t, h, "/readyz") },
		"studio /health":                 func() probeAnswer { return get(t, h, "/health") },
		"studio /healthz":                func() probeAnswer { return get(t, h, "/healthz") },
		"studio /ready?require=write":    func() probeAnswer { return get(t, h, "/ready?require=write") },
		"other replica /ready":           func() probeAnswer { return get(t, otherReady, "/ready") },
		"other replica /ready?require=w": func() probeAnswer { return get(t, otherReady, "/ready?require=write") },
	})

	// The primary's site is lost; the failover manager promotes the
	// standby once both replicas have seen that there is no primary.
	start := time.Now()
	require.NoError(t, pair.StopPrimary(ctx))
	require.Eventually(t, func() bool {
		return tracker.Status().State == dbstate.Unreachable && dbstate.For(other).Status().State == dbstate.Unreachable
	}, dbstate.DefaultCheckInterval+10*time.Second, 50*time.Millisecond, "both replicas see no primary")
	assert.Equal(t, "0", metricSample(t, "aistudio_db_writable"), "the gauge drops while there is no primary")
	require.NoError(t, pair.Promote(ctx))

	for _, db := range []*gorm.DB{s.db, other} {
		tr := dbstate.For(db)
		require.Eventually(t, func() bool {
			st := tr.Status()
			return st.State == dbstate.Writable && st.Failovers == 1
		}, 2*dbstate.DefaultCheckInterval+5*time.Second, 50*time.Millisecond, "%+v", tr.Status())
	}
	outage := time.Since(start)
	require.Less(t, outage, grace, "the test's failover must be shorter than the grace")
	p.finish()

	assert.Equal(t, "1", metricSample(t, "aistudio_db_writable"))
	assert.Equal(t, "1", metricSample(t, "aistudio_db_failovers_total"))

	for name, answers := range map[string][]probeAnswer{
		"studio /ready": p.of("studio /ready"), "studio /readyz": p.of("studio /readyz"),
		"studio /ready?require=write": p.of("studio /ready?require=write"),
		"other replica /ready":        p.of("other replica /ready"), "other replica /ready?require=w": p.of("other replica /ready?require=w"),
	} {
		require.NotEmpty(t, answers, name)
		sawUnreachable := false
		for _, a := range answers {
			assert.Equal(t, http.StatusOK, a.code, "%s answered %d (database %s) during a %s failover", name, a.code, a.database, outage)
			sawUnreachable = sawUnreachable || a.database == string(dbstate.Unreachable)
		}
		assert.True(t, sawUnreachable, "%s reported the outage in its body", name)
	}
	for _, name := range []string{"studio /health", "studio /healthz"} {
		for _, a := range p.of(name) {
			assert.Equal(t, http.StatusOK, a.code, name)
		}
	}
}

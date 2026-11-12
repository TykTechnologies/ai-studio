package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/api"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbstate"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Readiness across database faults, on the failover rig (suite "failover"
// of scripts/ci-postgres-tests.sh): what /ready, /health and the database
// metrics say while the database fails over, is lost by one replica only,
// or is a standby (DATABASE_READY_GRACE; features/ActiveActive.md,
// readiness).

// probeAnswer is one answer of a readiness or liveness endpoint.
type probeAnswer struct {
	code     int
	database string
}

func get(t *testing.T, h http.Handler, path string) probeAnswer {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body struct {
		Database string `json:"database"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return probeAnswer{code: rec.Code, database: body.Database}
}

// bareReplica is a replica reduced to its pool and its readiness probe:
// enough to stand for a second Studio, which one process cannot run.
func bareReplica(db *gorm.DB, grace time.Duration) http.Handler {
	r := gin.New()
	r.GET("/ready", api.Readiness(db, grace))
	return r
}

// prober asks endpoints every 100ms until stopped and keeps every answer.
type prober struct {
	mu      sync.Mutex
	answers map[string][]probeAnswer
	stop    chan struct{}
	done    sync.WaitGroup
}

func startProber(t *testing.T, endpoints map[string]func() probeAnswer) *prober {
	p := &prober{answers: map[string][]probeAnswer{}, stop: make(chan struct{})}
	p.done.Add(1)
	go func() {
		defer p.done.Done()
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			for name, ask := range endpoints {
				a := ask()
				p.mu.Lock()
				p.answers[name] = append(p.answers[name], a)
				p.mu.Unlock()
			}
			select {
			case <-p.stop:
				return
			case <-tick.C:
			}
		}
	}()
	t.Cleanup(p.finish)
	return p
}

func (p *prober) finish() {
	select {
	case <-p.stop:
	default:
		close(p.stop)
	}
	p.done.Wait()
}

func (p *prober) of(name string) []probeAnswer {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]probeAnswer(nil), p.answers[name]...)
}

// metricSample reads one series of the scrape ("" when it is absent).
func metricSample(t *testing.T, name string) string {
	t.Helper()
	h := metrics.Handler()
	require.NotNil(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	b, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	m := regexp.MustCompile(`(?m)^` + name + `(?:\{[^}]*\})? (\S+)$`).FindSubmatch(b)
	if m == nil {
		return ""
	}
	return string(m[1])
}

// tcpCut forwards connections to a target until Cut, which drops every
// connection and refuses new ones: one replica's network path to the
// database fails while the other's does not.
type tcpCut struct {
	ln     net.Listener
	target string
	cut    atomic.Bool
	mu     sync.Mutex
	conns  []net.Conn
}

func startTCPCut(t *testing.T, target string) *tcpCut {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	c := &tcpCut{ln: ln, target: target}
	t.Cleanup(func() { ln.Close(); c.Cut() })
	go func() {
		for {
			in, err := ln.Accept()
			if err != nil {
				return
			}
			if c.cut.Load() {
				in.Close()
				continue
			}
			out, err := net.Dial("tcp", target)
			if err != nil {
				in.Close()
				continue
			}
			c.mu.Lock()
			c.conns = append(c.conns, in, out)
			c.mu.Unlock()
			go func() { _, _ = io.Copy(out, in); out.Close() }()
			go func() { _, _ = io.Copy(in, out); in.Close() }()
		}
	}()
	return c
}

func (c *tcpCut) Addr() string { return c.ln.Addr().String() }

func (c *tcpCut) Cut() {
	c.cut.Store(true)
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, conn := range c.conns {
		conn.Close()
	}
	c.conns = nil
}

func (c *tcpCut) Restore() { c.cut.Store(false) }

// withHost is dsn with its hosts replaced by host.
func withHost(t *testing.T, dsn, host string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.Host = host
	return u.String()
}

// A replica that alone loses the database (the cluster's partition fault:
// one site cut off from it) stays ready for the grace, then answers 503 and
// leaves the load balancer; the replica that still reaches the database
// stays ready throughout. When the path is back, so is the replica.
//
// With the default grace (30 s) the timing is explicit: the tracker sees
// the loss within one check period plus one probe's bound after the cut
// (dbstate's lag), and /ready turns 503 once the grace has passed from
// then: never earlier than the grace after the cut, never later than grace
// + check period + probe bound (plus the poll's own granularity).
func TestFailover_ReadinessReplicaAloneLosesTheDatabase(t *testing.T) {
	pair := containers.StartPostgresPair(t, nil)
	grace := config.DefaultDatabaseReadyGrace

	healthy := openFailoverDB(t, pair)
	cut := startTCPCut(t, pair.Primary().Addr())
	conf := failoverConf(pair)
	conf.DatabaseURL = withHost(t, pair.Primary().DSN(), cut.Addr())
	lonely, err := OpenDatabase(conf)
	require.NoError(t, err)
	t.Cleanup(func() {
		if pool, err := lonely.DB(); err == nil {
			pool.Close()
		}
	})
	connConfig, err := pgx.ParseConfig(conf.DatabaseURL)
	require.NoError(t, err)
	probeBound := probeTimeout(connConfig)
	maxLag := dbstate.DefaultCheckInterval + probeBound
	const slack = time.Second // the polls below run every 100ms, and scheduling

	healthyReady, lonelyReady := bareReplica(healthy, grace), bareReplica(lonely, grace)
	require.Equal(t, probeAnswer{200, "writable"}, get(t, lonelyReady, "/ready"))

	p := startProber(t, map[string]func() probeAnswer{
		"healthy": func() probeAnswer { return get(t, healthyReady, "/ready") },
	})

	cutAt := time.Now()
	cut.Cut()

	// Poll the lonely replica until it answers 503; until then every
	// answer is 200.
	tracker := dbstate.For(lonely)
	var turned time.Duration
	sawUnreachable := false
	for {
		a := get(t, lonelyReady, "/ready")
		elapsed := time.Since(cutAt)
		if a.code == http.StatusServiceUnavailable {
			turned = elapsed
			assert.Equal(t, "unreachable", a.database)
			break
		}
		require.Equal(t, http.StatusOK, a.code, "ready until it turns 503 (%s after the cut)", elapsed)
		sawUnreachable = sawUnreachable || a.database == string(dbstate.Unreachable)
		require.Less(t, elapsed, grace+maxLag+slack, "/ready still 200 %s after the cut: later than grace %s + check period %s + probe bound %s",
			elapsed, grace, dbstate.DefaultCheckInterval, probeBound)
		time.Sleep(100 * time.Millisecond)
	}
	lag := tracker.Status().NotWritableSince.Sub(cutAt)
	t.Logf("cut -> tracker sees unreachable: %s (bound %s); cut -> /ready 503: %s (grace %s, bound %s)",
		lag, maxLag, turned, grace, grace+maxLag)
	assert.True(t, sawUnreachable, "the body said unreachable within the grace")
	assert.GreaterOrEqual(t, lag, time.Duration(0))
	assert.LessOrEqual(t, lag, maxLag, "dbstate's lag")
	assert.GreaterOrEqual(t, turned, grace, "never before the grace")
	assert.LessOrEqual(t, turned, grace+lag+slack, "the grace counts from when the tracker saw the loss")

	// It stays out while the path is cut.
	for i := 0; i < 10; i++ {
		require.Equal(t, probeAnswer{503, "unreachable"}, get(t, lonelyReady, "/ready"))
		time.Sleep(100 * time.Millisecond)
	}

	restoredAt := time.Now()
	cut.Restore()
	require.Eventually(t, func() bool { return get(t, lonelyReady, "/ready").code == http.StatusOK },
		maxLag+slack, 50*time.Millisecond, "back within one check period and probe once its path is")
	t.Logf("restore -> /ready 200: %s", time.Since(restoredAt))
	p.finish()

	answers := p.of("healthy")
	require.NotEmpty(t, answers)
	for _, a := range answers {
		assert.Equal(t, probeAnswer{200, "writable"}, a, "the replica that reaches the database is untouched")
	}
}

// A replica whose database is read-only (here: pointed at the standby)
// stays ready: reads work there. /ready?require=write leaves it after the
// grace, and the cluster status warns.
func TestFailover_ReadinessOnAReadOnlyDatabase(t *testing.T) {
	pair := containers.StartPostgresPair(t, &containers.PostgresPairConfig{Synchronous: true})
	const grace = 2 * time.Second

	primary := openFailoverDB(t, pair)
	require.NoError(t, primary.AutoMigrate(&models.ClusterNode{}, &models.ClusterLease{}, &models.EdgeInstance{}, &models.EdgePushCommand{}))

	// OpenDatabase waits for a writable server (it migrates at start), so
	// the standby's pool is opened as OpenDatabase opens a Postgres pool,
	// with its tracker, without that wait: a replica whose primary was
	// demoted under it is in the same state.
	conf := failoverConf(pair)
	conf.DatabaseURL = pair.Standby().DSN()
	dialector, _, err := openPostgres(conf, conf.DatabaseURL)
	require.NoError(t, err)
	standby, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if pool, err := standby.DB(); err == nil {
			pool.Close()
		}
	})
	tracker := dbstate.For(standby)
	require.NotNil(t, tracker)
	require.Equal(t, dbstate.ReadOnly, tracker.Status().State)
	ready := bareReplica(standby, grace)

	if time.Since(tracker.Status().NotWritableSince) < grace-500*time.Millisecond {
		assert.Equal(t, probeAnswer{200, "read_only"}, get(t, ready, "/ready?require=write"), "require=write within the grace")
	}
	require.Eventually(t, func() bool { return get(t, ready, "/ready?require=write").code == http.StatusServiceUnavailable },
		grace+5*time.Second, 50*time.Millisecond)
	assert.Equal(t, probeAnswer{503, "read_only"}, get(t, ready, "/ready?require=write"))

	// Well past the grace and several probes later, still ready.
	deadline := time.Now().Add(dbstate.DefaultCheckInterval + 2*time.Second)
	for time.Now().Before(deadline) {
		require.Equal(t, probeAnswer{200, "read_only"}, get(t, ready, "/ready"))
		time.Sleep(250 * time.Millisecond)
	}

	st, err := cluster.Snapshot(context.Background(), standby, "node-a", nil, nil)
	require.NoError(t, err, "a read-only database still answers the cluster status")
	assert.Contains(t, fmt.Sprint(st.Warnings), "Replica node-a has seen its database as read-only since")
	st, err = cluster.Snapshot(context.Background(), primary, "node-a", nil, nil)
	require.NoError(t, err)
	assert.NotContains(t, fmt.Sprint(st.Warnings), "its database")
}

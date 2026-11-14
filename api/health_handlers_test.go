package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbstate"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// readinessServer is a database whose state the test sets: a pool (on
// SQLite, so the request's ping can be made to fail by closing it) with a
// dbstate.Tracker whose probe answers what the test last set.
type readinessServer struct {
	db *gorm.DB

	mu    sync.Mutex
	probe dbstate.Probe
	err   error
	tr    *dbstate.Tracker
}

func newReadinessServer(t *testing.T) *readinessServer {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	s := &readinessServer{db: db, probe: dbstate.Probe{Server: "pg-a:5432"}}
	s.tr = dbstate.Start(func(context.Context) (dbstate.Probe, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.probe, s.err
	}, dbstate.Options{Interval: 5 * time.Millisecond})
	dbstate.Register(pool, s.tr)
	t.Cleanup(func() {
		dbstate.Unregister(pool)
		s.tr.Stop()
		pool.Close()
	})
	return s
}

// set makes the probe answer as a database in state and waits until the
// tracker has seen it.
func (s *readinessServer) set(t *testing.T, state dbstate.State) {
	t.Helper()
	s.mu.Lock()
	switch state {
	case dbstate.Writable:
		s.probe, s.err = dbstate.Probe{Server: "pg-a:5432"}, nil
	case dbstate.ReadOnly:
		s.probe, s.err = dbstate.Probe{Server: "pg-a:5432", ReadOnly: true}, nil
	case dbstate.Unreachable:
		s.probe, s.err = dbstate.Probe{}, errors.New("connection refused")
	}
	s.mu.Unlock()
	require.Eventually(t, func() bool { return s.tr.Status().State == state }, 5*time.Second, time.Millisecond)
}

type readinessBody struct {
	Status        string `json:"status"`
	Database      string `json:"database"`
	DatabaseSince string `json:"database_since"`
}

func ready(t *testing.T, a *API, path string) (int, readinessBody) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	a.handleReadiness(c)
	var body readinessBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return w.Code, body
}

// The status code and body for each state, with and without require=write,
// with a grace: writable and read-only are ready, unreachable is ready
// until the grace has passed, require=write is ready only while writable or
// within the grace.
func TestReadinessReportsDatabaseState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const grace = 300 * time.Millisecond
	s := newReadinessServer(t)
	a := &API{config: &auth.Config{DB: s.db}}
	a.SetDatabaseReadyGrace(grace)

	type want struct {
		code int
		db   string
	}
	check := func(what string, plain, write want) {
		t.Helper()
		code, body := ready(t, a, "/ready")
		assert.Equal(t, plain.code, code, "%s: /ready", what)
		assert.Equal(t, plain.db, body.Database, "%s: /ready", what)
		assert.NotEmpty(t, body.DatabaseSince, "%s", what)
		if plain.code == http.StatusOK {
			assert.Equal(t, "ready", body.Status)
		} else {
			assert.Equal(t, "not ready", body.Status)
		}
		code, body = ready(t, a, "/ready?require=write")
		assert.Equal(t, write.code, code, "%s: /ready?require=write", what)
		assert.Equal(t, write.db, body.Database, "%s: /ready?require=write", what)
	}

	check("writable", want{200, "writable"}, want{200, "writable"})

	s.set(t, dbstate.ReadOnly)
	check("read-only within the grace", want{200, "read_only"}, want{200, "read_only"})
	time.Sleep(grace)
	check("read-only after the grace", want{200, "read_only"}, want{503, "read_only"})

	s.set(t, dbstate.Writable)
	check("writable again", want{200, "writable"}, want{200, "writable"})

	s.set(t, dbstate.Unreachable)
	check("unreachable within the grace", want{200, "unreachable"}, want{200, "unreachable"})
	time.Sleep(grace)
	check("unreachable after the grace", want{503, "unreachable"}, want{503, "unreachable"})

	// The grace runs from when the database stopped taking writes, not
	// from the last change between read-only and unreachable.
	s.set(t, dbstate.ReadOnly)
	check("read-only after unreachable", want{200, "read_only"}, want{503, "read_only"})
	s.set(t, dbstate.Unreachable)
	check("unreachable again", want{503, "unreachable"}, want{503, "unreachable"})

	s.set(t, dbstate.Writable)
	check("back", want{200, "writable"}, want{200, "writable"})
}

// A grace of 0 is the readiness check before 2.3: the database is pinged
// in the request and that decides, whatever the tracker last saw; the body
// still says the state, and require=write needs a writable database.
func TestReadinessWithoutGraceIsThePing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := newReadinessServer(t)
	a := &API{config: &auth.Config{DB: s.db}}

	code, body := ready(t, a, "/ready")
	assert.Equal(t, 200, code)
	assert.Equal(t, "writable", body.Database)

	s.set(t, dbstate.ReadOnly)
	code, body = ready(t, a, "/ready")
	assert.Equal(t, 200, code, "a read-only database answers the ping")
	assert.Equal(t, "read_only", body.Database)
	code, _ = ready(t, a, "/ready?require=write")
	assert.Equal(t, 503, code)

	s.set(t, dbstate.Unreachable)
	code, body = ready(t, a, "/ready")
	assert.Equal(t, 200, code, "the tracker does not decide: the ping does, and it still answers")
	assert.Equal(t, "unreachable", body.Database)

	s.set(t, dbstate.Writable)
	pool, err := s.db.DB()
	require.NoError(t, err)
	require.NoError(t, pool.Close())
	code, body = ready(t, a, "/ready")
	assert.Equal(t, 503, code, "the ping fails at once")
	assert.Equal(t, "unreachable", body.Database, "and the body says so, though the tracker has not probed since")
	code, _ = ready(t, a, "/ready?require=write")
	assert.Equal(t, 503, code)
}

// SQLite has no tracker: the ping alone, and the body as before 2.3.
func TestReadinessWithoutTracker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	a := &API{config: &auth.Config{DB: db}}
	a.SetDatabaseReadyGrace(30 * time.Second)

	for _, path := range []string{"/ready", "/ready?require=write"} {
		code, body := ready(t, a, path)
		assert.Equal(t, 200, code, path)
		assert.Equal(t, readinessBody{Status: "ready"}, body, path)
	}
	require.NoError(t, pool.Close())
	code, body := ready(t, a, "/ready")
	assert.Equal(t, 503, code)
	assert.Equal(t, readinessBody{Status: "not ready"}, body)

	code, _ = ready(t, &API{}, "/ready")
	assert.Equal(t, 503, code, "no database")
}

// The rule itself, at given times.
func TestDatabaseReady(t *testing.T) {
	now := time.Now()
	const grace = 30 * time.Second
	for _, tc := range []struct {
		st           dbstate.Status
		grace        time.Duration
		requireWrite bool
		want         bool
	}{
		{dbstate.Status{State: dbstate.Writable}, grace, false, true},
		{dbstate.Status{State: dbstate.Writable}, grace, true, true},
		{dbstate.Status{State: dbstate.Writable}, 0, true, true},
		{dbstate.Status{State: dbstate.ReadOnly, NotWritableSince: now.Add(-time.Hour)}, grace, false, true},
		{dbstate.Status{State: dbstate.ReadOnly, NotWritableSince: now.Add(-29 * time.Second)}, grace, true, true},
		{dbstate.Status{State: dbstate.ReadOnly, NotWritableSince: now.Add(-30 * time.Second)}, grace, true, false},
		{dbstate.Status{State: dbstate.Unreachable, NotWritableSince: now.Add(-29 * time.Second)}, grace, false, true},
		{dbstate.Status{State: dbstate.Unreachable, NotWritableSince: now.Add(-30 * time.Second)}, grace, false, false},
		{dbstate.Status{State: dbstate.Unreachable, NotWritableSince: now}, 0, false, false},
		{dbstate.Status{State: dbstate.Unreachable}, grace, false, false},
	} {
		assert.Equal(t, tc.want, databaseReady(tc.st, tc.grace, tc.requireWrite, now), "%+v grace=%s require_write=%v", tc.st, tc.grace, tc.requireWrite)
	}
}

// Liveness never looks at the database.
func TestHealthIgnoresTheDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := newReadinessServer(t)
	a := &API{config: &auth.Config{DB: s.db}}
	a.SetDatabaseReadyGrace(time.Millisecond)
	s.set(t, dbstate.Unreachable)
	pool, err := s.db.DB()
	require.NoError(t, err)
	require.NoError(t, pool.Close())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/health", nil)
	a.handleHealth(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

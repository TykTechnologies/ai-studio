package studio

import (
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbstate"
)

// silentHost accepts TCP connections and never answers them: a host whose
// site is down hard, as a client sees it through a firewall that drops
// packets. (A stopped container, as in the failover rig, refuses the
// connection at once, which hides what this costs.)
func silentHost(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		l.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			c.Close()
		}
	})
	return l.Addr().String()
}

// The first host of a multi-host DATABASE_URL does not answer at all and the
// second is the writable server. Statements reach the second after waiting
// out the connect timeout on the first; the tracker's probe, which has to
// dial too, must be given as long, and report the database writable. With a
// probe bounded more tightly than the connection attempts (it had 3 s; here
// they take 2 s each, and two with sslmode=prefer) it said unreachable while
// statements were writing.
func TestOpenDatabase_SilentFirstHost_Postgres(t *testing.T) {
	base := os.Getenv("DATABASE_URL")
	if !strings.HasPrefix(base, "postgres") {
		t.Skip("DATABASE_URL is not a Postgres URL")
	}
	u, err := url.Parse(base)
	require.NoError(t, err)
	u.Host = silentHost(t) + "," + u.Host
	q := u.Query()
	q.Set("target_session_attrs", "read-write")
	q.Del("connect_timeout")
	u.RawQuery = q.Encode()

	start := time.Now()
	db, err := OpenDatabase(&config.AppConf{
		DatabaseType:           "postgres",
		DatabaseURL:            u.String(),
		DatabaseConnectTimeout: 2 * time.Second,
		DatabaseMaxOpenConns:   4,
	})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	opened := time.Since(start)
	assert.GreaterOrEqual(t, opened, 2*time.Second, "the first connection waited out the silent host")

	tracker := dbstate.For(db)
	require.NotNil(t, tracker)
	assert.Equal(t, dbstate.Writable, tracker.Status().State, "the first probe had to dial past the silent host")
	assert.Zero(t, tracker.Status().Failovers)

	// Every new connection pays for the silent host again; with all of the
	// pool's connections busy the periodic probe has to dial one. It still
	// finds the database writable.
	var hold sync.WaitGroup
	for i := 0; i < 3; i++ {
		hold.Add(1)
		go func() {
			defer hold.Done()
			assert.NoError(t, db.Exec("SELECT pg_sleep(?)", (dbstate.DefaultCheckInterval+4*time.Second).Seconds()).Error)
		}()
	}
	deadline := time.Now().Add(dbstate.DefaultCheckInterval + 3*time.Second)
	for time.Now().Before(deadline) {
		require.Equal(t, dbstate.Writable, tracker.Status().State, "after %s", time.Since(start))
		time.Sleep(100 * time.Millisecond)
	}
	hold.Wait()
	assert.NoError(t, db.Exec("CREATE TEMP TABLE silent_host_probe (id int)").Error, "and it is writable")
}

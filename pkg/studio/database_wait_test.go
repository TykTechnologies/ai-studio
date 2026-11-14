package studio

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config"
)

// closedPort is a local TCP port nothing listens on: a connection to it is
// refused at once.
func closedPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

// No database answers: OpenDatabase keeps trying until the timeout, logs
// the reason once (not once per attempt), and fails naming the setting.
func TestOpenDatabaseWaitsUpToTheStartupTimeout(t *testing.T) {
	sink := captureLog(t)
	conf := &config.AppConf{
		DatabaseType:           "postgres",
		DatabaseURL:            "postgres://u:p@" + closedPort(t) + "/db?sslmode=disable",
		DatabaseConnectTimeout: time.Second,
		DatabaseStartupTimeout: 2 * time.Second,
	}
	started := time.Now()
	_, err := OpenDatabase(conf)
	took := time.Since(started)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no writable database within DATABASE_STARTUP_TIMEOUT (2s")
	assert.Contains(t, err.Error(), "refused")
	assert.GreaterOrEqual(t, took, 2*time.Second)
	assert.Less(t, took, 10*time.Second)
	assert.Equal(t, 1, strings.Count(sink.String(), "Waiting for the database"), sink.String())
}

// A timeout of 0 keeps the old behaviour: one attempt, then the error.
func TestOpenDatabaseWithoutStartupTimeoutFailsAtOnce(t *testing.T) {
	conf := &config.AppConf{
		DatabaseType:           "postgres",
		DatabaseURL:            "postgres://u:p@" + closedPort(t) + "/db?sslmode=disable",
		DatabaseConnectTimeout: time.Second,
	}
	started := time.Now()
	_, err := OpenDatabase(conf)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "DATABASE_STARTUP_TIMEOUT")
	assert.Less(t, time.Since(started), 5*time.Second)
}

func TestDatabaseStartupTimeoutSetting(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	assert.Equal(t, 60*time.Second, config.LoadFrom(env(nil)).DatabaseStartupTimeout)
	assert.Equal(t, 90*time.Second, config.LoadFrom(env(map[string]string{"DATABASE_STARTUP_TIMEOUT": "90s"})).DatabaseStartupTimeout)
	assert.Equal(t, time.Duration(0), config.LoadFrom(env(map[string]string{"DATABASE_STARTUP_TIMEOUT": "0"})).DatabaseStartupTimeout)
}

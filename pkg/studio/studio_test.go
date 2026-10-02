//go:build !enterprise

package studio

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStudioServesItsHandlerAndStopsCleanly(t *testing.T) {
	opts := newTestOptions(t)
	s, err := New(opts)
	require.NoError(t, err)

	srv := httptest.NewServer(s.HTTPHandler())
	defer srv.Close()

	get := func(path string) *http.Response {
		resp, err := http.Get(srv.URL + path)
		require.NoError(t, err)
		resp.Body.Close()
		return resp
	}
	assert.Equal(t, http.StatusOK, get("/health").StatusCode)
	assert.Equal(t, http.StatusOK, get("/auth/config").StatusCode)
	assert.Equal(t, http.StatusUnauthorized, get("/api/v1/llms").StatusCode, "the management API requires authentication")
	assert.Equal(t, http.StatusOK, get("/portal/dashboard").StatusCode, "the SPA fallback serves index.html")

	stopStudio(t, s)
	assert.NoError(t, s.Stop(context.Background()), "Stop is idempotent")

	// Stop leaves the host's database open.
	sqlDB, err := opts.DB.DB()
	require.NoError(t, err)
	assert.NoError(t, sqlDB.Ping())
}

func TestOnlyOneStudioRunsAtATime(t *testing.T) {
	first, err := New(newTestOptions(t))
	require.NoError(t, err)

	_, err = New(newTestOptions(t))
	assert.ErrorIs(t, err, ErrAlreadyRunning)

	stopStudio(t, first)

	second, err := New(newTestOptions(t))
	require.NoError(t, err, "a Studio can be built again once the first has stopped")
	stopStudio(t, second)
}

func TestNewRequiresConfigAndDB(t *testing.T) {
	_, err := New(Options{})
	assert.Error(t, err)
	assert.False(t, errors.Is(err, ErrAlreadyRunning))

	// A failed New must not leave the process marked as running.
	s, err := New(newTestOptions(t))
	require.NoError(t, err)
	stopStudio(t, s)
}

func TestNewRejectsAnInvalidHostTykConnection(t *testing.T) {
	opts := newTestOptions(t)
	opts.HostTykConnection = &HostTykConnection{URL: "dashboard:3000"}
	_, err := New(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HostTykConnection")

	// A failed New must not leave the process marked as running.
	s, err := New(newTestOptions(t))
	require.NoError(t, err)
	stopStudio(t, s)
}

func TestStartGRPCRequiresControlMode(t *testing.T) {
	s, err := New(newTestOptions(t))
	require.NoError(t, err)
	defer stopStudio(t, s)

	assert.ErrorIs(t, s.StartGRPC(nil), ErrNotControlPlane)
}

func TestControlModeServesGRPCOnAHostListener(t *testing.T) {
	opts := newTestOptions(t)
	opts.Config.GatewayMode = "control"
	opts.Config.MicrogatewayEncryptionKey = "0123456789abcdef0123456789abcdef"
	opts.Config.GRPCAuthToken = "test-token"
	opts.Config.GRPCTLSEnabled = false
	s, err := New(opts)
	require.NoError(t, err)

	lis, err := listenLocal()
	require.NoError(t, err)
	served := make(chan error, 1)
	go func() { served <- s.StartGRPC(lis) }()

	// The listener accepts connections while served.
	require.Eventually(t, func() bool {
		conn, err := dialLocal(lis.Addr().String())
		if err != nil {
			return false
		}
		conn.Close()
		return true
	}, 5*time.Second, 50*time.Millisecond)

	stopStudio(t, s)
	select {
	case err := <-served:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal(fmt.Errorf("StartGRPC did not return after Stop"))
	}
}

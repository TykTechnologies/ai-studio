//go:build !enterprise

package studio

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/proxy"
)

// New starts the native resource reconciler gated by the leader lease, and
// Stop stops it (once) before the cluster goes away.
func TestNativeWorkersFollowStudioLifecycle(t *testing.T) {
	var started, stopped atomic.Int32
	var gate func() bool
	startNativeWorkers = func(p *proxy.Proxy, ctx context.Context, g func() bool) func() {
		require.NotNil(t, p)
		started.Add(1)
		gate = g
		return func() {
			stopped.Add(1)
			assert.NotPanics(t, func() { gate() }, "the lease is still readable when the workers stop")
		}
	}
	t.Cleanup(func() {
		startNativeWorkers = func(p *proxy.Proxy, ctx context.Context, g func() bool) func() {
			return p.StartNativeWorkers(ctx, g)
		}
	})

	s, err := New(newTestOptions(t))
	require.NoError(t, err)
	assert.EqualValues(t, 1, started.Load())
	require.NotNil(t, gate)
	assert.Equal(t, s.leadership.IsLeader(), gate(), "gated by this replica's leader lease")
	assert.Zero(t, stopped.Load())

	stopStudio(t, s)
	assert.EqualValues(t, 1, stopped.Load())
	require.NoError(t, s.Stop(context.Background()))
	assert.EqualValues(t, 1, stopped.Load(), "a second Stop does not stop again")
}

// The real workers start and stop with a Studio (no leak under -race).
func TestNativeWorkersRealStartStop(t *testing.T) {
	s, err := New(newTestOptions(t))
	require.NoError(t, err)
	require.NotNil(t, s.stopNative)
	stopStudio(t, s)
}

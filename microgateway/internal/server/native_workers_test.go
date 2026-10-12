package server

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// orderGateway records the calls the server makes on the gateway.
type orderGateway struct {
	aigateway.Gateway
	mu    sync.Mutex
	calls []string
	gate  func() bool
}

func (g *orderGateway) record(c string) {
	g.mu.Lock()
	g.calls = append(g.calls, c)
	g.mu.Unlock()
}

func (g *orderGateway) StartNativeWorkers(_ context.Context, gate func() bool) func() {
	g.record("start")
	g.gate = gate
	return func() { g.record("stop") }
}

func (g *orderGateway) WaitForAnalytics(context.Context) error {
	g.record("wait-analytics")
	return nil
}

// The edge runs the native reconciler ungated, and Shutdown stops it before
// waiting for the analytics, so what it accounted is analysed (and pulsed)
// before the plugins stop. Once shut down it is never started again.
func TestNativeWorkers_StartedUngatedAndStoppedBeforeAnalyticsDrain(t *testing.T) {
	gw := &orderGateway{}
	s := &Server{server: &http.Server{}, gateway: gw}

	s.startNativeWorkers()
	s.startNativeWorkers() // idempotent
	assert.Nil(t, gw.gate, "an edge's ledger is its own: no leader gate")

	require.NoError(t, s.Shutdown(context.Background()))
	s.startNativeWorkers()
	require.NoError(t, s.Shutdown(context.Background()))

	gw.mu.Lock()
	defer gw.mu.Unlock()
	assert.Equal(t, []string{"start", "stop", "wait-analytics", "wait-analytics"}, gw.calls)
}

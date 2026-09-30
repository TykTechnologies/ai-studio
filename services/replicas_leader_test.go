package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
)

type testLeadership struct{ leader atomic.Bool }

func (l *testLeadership) IsLeader() bool                       { return l.leader.Load() }
func (l *testLeadership) Signal(context.Context, string) error { return nil }

// One telemetry report per deployment: replicas that do not hold the
// leader lease send none.
func TestTelemetryManager_OnlyTheLeaderReports(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	oldURL := telemetryEndpoint
	telemetryEndpoint = server.URL
	t.Cleanup(func() { telemetryEndpoint = oldURL })

	lead := &testLeadership{}
	replicas.SetBackend(lead)
	t.Cleanup(func() { replicas.SetBackend(nil) })

	_, tm := setupTelemetryManagerTest(t)
	tm.collectAndSend()
	assert.Zero(t, hits.Load(), "a follower does not report")

	lead.leader.Store(true)
	tm.collectAndSend()
	assert.Equal(t, int32(1), hits.Load(), "the leader reports")
}

//go:build !enterprise

package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/services/agenttracing"
)

// D2: Agent Call Tracing is Enterprise only. In a Community build, with
// tracing configured on and a licence that says valid, the start gate stays
// shut: no capture, no key or content policy for edges, no data served,
// nothing written to the agent trace tables.
func TestAgentTracing_CommunityBuildIsInert(t *testing.T) {
	t.Cleanup(func() { agenttrace.SetEnabled(false) })
	db := setupTestDBForDatasources(t)
	require.NoError(t, db.AutoMigrate(models.AgentTraceModels()...))
	lic := &licenceStub{}
	lic.valid.Store(true)
	s := &Service{DB: db, LicensingService: lic}

	cfg := config.DefaultAgentTracingConfig()
	cfg.Enabled = true
	cfg.Key = "community-build-inert-root-key-0123"
	s.InitAgentTracing(context.Background(), cfg, "node-1", "test")

	assert.False(t, agenttrace.Enabled(), "no capture")
	assert.False(t, s.agentTracingStarted.Load())
	require.NotNil(t, s.AgentTracing)
	assert.False(t, s.AgentTracing.IsEnterpriseAvailable())
	assert.False(t, s.AgentTracing.Running())
	assert.False(t, agenttracing.GateOpen(s.AgentTracing))

	_, _, ok := agenttrace.EdgeKey()
	assert.False(t, ok, "no fingerprint key for edges (snapshot field 17)")
	_, ok = agenttrace.EdgeContentGlobals()
	assert.False(t, ok, "no content policy for edges (snapshot field 18)")

	ctx, rec := agenttrace.Begin(context.Background())
	assert.Nil(t, rec, "the proxy attaches no recorder")
	agenttrace.CaptureRaw(ctx, agenttrace.PartRequest, "application/json", []byte(`{"secret":"x"}`))

	_, err := s.AgentTracing.ExportSession(context.Background(), agenttracing.Viewer{AllApps: true, CanReadContent: true, CanExport: true},
		"0192a4c8-7f00-7000-8000-000000000001", agenttracing.ExportQuery{Format: "json", Content: true}, agenttracing.AccessMeta{})
	assert.ErrorIs(t, err, agenttracing.ErrEnterpriseFeature, "no data is served")

	s.SyncAgentTracingLicence()
	assert.False(t, agenttrace.Enabled(), "a licence check cannot open the gate either")

	for _, m := range models.AgentTraceModels() {
		var n int64
		require.NoError(t, db.Model(m).Count(&n).Error)
		assert.Zero(t, n, "%T has rows", m)
	}
}

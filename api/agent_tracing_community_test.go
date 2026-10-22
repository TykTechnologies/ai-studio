//go:build !enterprise
// +build !enterprise

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Community build with AGENT_TRACING_ENABLED=true stays inert: the capture
// flag is off, no recorder is created and the UI feature flag is false.
func TestAgentTracingCommunity_EnabledFlagStaysInert(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	cfg := config.DefaultAgentTracingConfig()
	cfg.Enabled = true
	service.InitAgentTracing(context.Background(), cfg, "node-1", "test")
	t.Cleanup(func() { agenttrace.SetEnabled(false) })

	assert.False(t, agenttrace.Enabled())
	_, rec := agenttrace.Begin(context.Background())
	assert.Nil(t, rec)
	assert.False(t, service.AgentTracing.Running())

	authConfig := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	r := NewAPI(service, true, authService, authConfig, nil, emptyFile, nil).router
	w := apitest.PerformRequest(r, "GET", "/common/system", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var sys struct {
		Features map[string]interface{} `json:"features"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sys))
	assert.Equal(t, false, sys.Features["feature_agent_tracing"])
}

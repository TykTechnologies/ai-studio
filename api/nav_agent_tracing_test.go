package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The Agent sessions entry exists only while agent tracing runs: never in
// the Community Edition, and not in an Enterprise build with it off.
func TestAdminNav_AgentSessionsOnlyWhenTracingRuns(t *testing.T) {
	has := func(items []NavItem) bool {
		for _, it := range items {
			if it.ID == "agent-sessions" {
				return it.Path == "/admin/agent-sessions" && it.Permission == "traces:read"
			}
		}
		return false
	}
	off := allFeatures()
	delete(off, "feature_agent_tracing")
	assert.False(t, has(adminNav(navInputs{features: features(off)})), "CE")
	assert.False(t, has(adminNav(navInputs{features: features(off), enterprise: true})), "ENT with tracing off")
	assert.True(t, has(adminNav(navInputs{features: features(allFeatures()), enterprise: true})), "ENT with tracing on")
}

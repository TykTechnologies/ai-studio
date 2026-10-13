package main

import (
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
)

// An edge never resolves an $ENV/ reference it finds in a snapshot field,
// even for a name its own allowlist lists: the hub resolves what its
// operator allows, and sends what it refused as a bare "$ENV/". (Edge code
// such as the semantic router's LLM client and the guardrail runner resolve
// snapshot values through secrets.GetValue.)
//
// The switch is process-wide, so this test is the only one in the package
// that turns it on, and it does not turn it off.
func TestEdgeIgnoresEnvReferencesInSnapshotFields(t *testing.T) {
	t.Setenv("MICROGATEWAY_ENCRYPTION_KEY", "edge-own-secret")
	t.Setenv(secrets.EnvRefAllowlistVar, "MICROGATEWAY_ENCRYPTION_KEY")
	assert.Equal(t, "edge-own-secret", secrets.GetValue("$ENV/MICROGATEWAY_ENCRYPTION_KEY", false),
		"before the edge policy, the allowlist applies")

	applyEdgeReferencePolicy(&config.Config{HubSpoke: config.HubSpokeConfig{Mode: "edge"}})

	for _, v := range []string{"$ENV/MICROGATEWAY_ENCRYPTION_KEY", "$ENV/"} {
		assert.Equal(t, v, secrets.GetValue(v, false), "an edge resolved %q", v)
	}
	assert.Equal(t, "plain-hub-value", secrets.GetValue("plain-hub-value", false), "hub-resolved values pass through")
}

package config

import (
	"testing"
	"time"

	"github.com/caarlos0/env/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHubSpokeEndpoints(t *testing.T) {
	assert.Nil(t, HubSpokeConfig{}.Endpoints())
	assert.Equal(t, []string{"cp:50051"}, HubSpokeConfig{ControlEndpoint: " cp:50051 "}.Endpoints(),
		"CONTROL_ENDPOINT is a list of one")
	assert.Equal(t, []string{"cp-a:50051", "cp-b:50051"},
		HubSpokeConfig{ControlEndpoints: []string{"cp-a:50051", " cp-b:50051"}}.Endpoints(),
		"the order is kept and entries are trimmed")
}

// CONTROL_ENDPOINTS is read from the environment as an ordered,
// comma-separated list.
func TestControlEndpointsFromEnvironment(t *testing.T) {
	t.Setenv("CONTROL_ENDPOINTS", "cp-a.example:50051, cp-b.example:50051")
	t.Setenv("EDGE_CONTROL_FAILBACK_INTERVAL", "5m")
	var hs HubSpokeConfig
	require.NoError(t, env.Parse(&hs))
	assert.Equal(t, []string{"cp-a.example:50051", "cp-b.example:50051"}, hs.Endpoints())
	assert.Equal(t, 5*time.Minute, hs.ControlFailbackInterval)

	t.Run("failback is off by default", func(t *testing.T) {
		t.Setenv("EDGE_CONTROL_FAILBACK_INTERVAL", "")
		var hs HubSpokeConfig
		require.NoError(t, env.Parse(&hs))
		assert.Zero(t, hs.ControlFailbackInterval)
	})
}

func TestValidateControlEndpoints(t *testing.T) {
	for name, tc := range map[string]struct {
		hs      HubSpokeConfig
		wantErr string
	}{
		"one endpoint, as before":    {HubSpokeConfig{ControlEndpoint: "cp:50051"}, ""},
		"a list":                     {HubSpokeConfig{ControlEndpoints: []string{"cp-a:50051", "cp-b:50051"}}, ""},
		"a list of one":              {HubSpokeConfig{ControlEndpoints: []string{"cp-a:50051"}}, ""},
		"neither":                    {HubSpokeConfig{}, "control endpoint is required for edge mode"},
		"both":                       {HubSpokeConfig{ControlEndpoint: "cp:50051", ControlEndpoints: []string{"cp-a:50051"}}, "set CONTROL_ENDPOINTS or CONTROL_ENDPOINT, not both"},
		"an empty entry":             {HubSpokeConfig{ControlEndpoints: []string{"cp-a:50051", " "}}, "CONTROL_ENDPOINTS has an empty entry"},
		"a trailing comma":           {HubSpokeConfig{ControlEndpoints: []string{"cp-a:50051", ""}}, "CONTROL_ENDPOINTS has an empty entry"},
		"the same endpoint twice":    {HubSpokeConfig{ControlEndpoints: []string{"cp-a:50051", "cp-a:50051"}}, "CONTROL_ENDPOINTS lists cp-a:50051 twice"},
		"a negative failback":        {HubSpokeConfig{ControlEndpoint: "cp:50051", ControlFailbackInterval: -time.Second}, "EDGE_CONTROL_FAILBACK_INTERVAL must not be negative"},
		"failback with one endpoint": {HubSpokeConfig{ControlEndpoint: "cp:50051", ControlFailbackInterval: time.Minute}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.hs.validateEndpoints()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// Validate applies the endpoint rules to an edge and to nothing else.
func TestValidateChecksEndpointsOnlyInEdgeMode(t *testing.T) {
	base := func() *Config {
		return &Config{
			Server:        ServerConfig{Port: 8080},
			Database:      DatabaseConfig{Type: "sqlite"},
			Cache:         CacheConfig{MaxSize: 1000},
			Analytics:     AnalyticsConfig{BufferSize: 100, RetentionDays: 30},
			Security:      SecurityConfig{EncryptionKey: "12345678901234567890123456789012", BCryptCost: 10},
			Observability: ObservabilityConfig{LogLevel: "info", LogFormat: "json"},
		}
	}

	edge := base()
	edge.HubSpoke = HubSpokeConfig{Mode: "edge", EdgeID: "e", ClientToken: "t",
		ControlEndpoint: "cp:50051", ControlEndpoints: []string{"cp-a:50051"}}
	err := edge.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not both")

	edge.HubSpoke.ControlEndpoint = ""
	edge.HubSpoke.ControlEndpoints = []string{"cp-a:50051", "cp-b:50051"}
	require.NoError(t, edge.Validate())

	standalone := base()
	standalone.HubSpoke = HubSpokeConfig{Mode: "standalone",
		ControlEndpoint: "cp:50051", ControlEndpoints: []string{"cp-a:50051"}}
	require.NoError(t, standalone.Validate(), "a standalone gateway does not read the control endpoints")
}

// The last-known-good settings: off by default, 30 s and 72 h.
func TestLastKnownGoodSettings(t *testing.T) {
	var hs HubSpokeConfig
	require.NoError(t, env.Parse(&hs))
	assert.False(t, hs.StartFromLastKnownGood, "off by default")
	assert.Equal(t, 30*time.Second, hs.StartupControlTimeout)
	assert.Equal(t, 72*time.Hour, hs.LastKnownGoodMaxAge)

	t.Setenv("EDGE_START_FROM_LAST_KNOWN_GOOD", "true")
	t.Setenv("EDGE_STARTUP_CONTROL_TIMEOUT", "10s")
	t.Setenv("EDGE_LAST_KNOWN_GOOD_MAX_AGE", "0")
	hs = HubSpokeConfig{}
	require.NoError(t, env.Parse(&hs))
	assert.True(t, hs.StartFromLastKnownGood)
	assert.Equal(t, 10*time.Second, hs.StartupControlTimeout)
	assert.Zero(t, hs.LastKnownGoodMaxAge, "0 means no limit")

	base := HubSpokeConfig{ControlEndpoint: "cp:50051"}
	neg := base
	neg.StartupControlTimeout = -time.Second
	require.ErrorContains(t, neg.validateEndpoints(), "EDGE_STARTUP_CONTROL_TIMEOUT must not be negative")
	neg = base
	neg.LastKnownGoodMaxAge = -time.Hour
	require.ErrorContains(t, neg.validateEndpoints(), "EDGE_LAST_KNOWN_GOOD_MAX_AGE must not be negative")
}

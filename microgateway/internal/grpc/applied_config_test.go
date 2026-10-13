package grpc

import (
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
)

// Heartbeats report the configuration the edge applied when an applied
// source is set, and the last snapshot received until it has one.
func TestLoadedConfigPrefersTheAppliedSource(t *testing.T) {
	client := NewSimpleEdgeClient(&config.Config{HubSpoke: config.HubSpokeConfig{EdgeID: "e"}}, "test", "hash", "time")
	client.setLoadedConfig(&pb.ConfigurationSnapshot{Checksum: "received", Version: "2"})

	sum, version := client.loadedConfig()
	assert.Equal(t, "received", sum)
	assert.Equal(t, "2", version)

	applied := struct {
		sum, version string
		ok           bool
	}{}
	client.SetAppliedConfigSource(func() (string, string, bool) { return applied.sum, applied.version, applied.ok })
	sum, _ = client.loadedConfig()
	assert.Equal(t, "received", sum, "no sync attempted yet")

	applied.ok = true // attempted, nothing applied yet
	sum, version = client.loadedConfig()
	assert.Equal(t, "", sum, "a snapshot that failed to apply is not reported as loaded")
	assert.Equal(t, "", version)

	applied.sum, applied.version = "applied", "1"
	sum, version = client.loadedConfig()
	assert.Equal(t, "applied", sum)
	assert.Equal(t, "1", version)
}

package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentTracingConfig_Defaults(t *testing.T) {
	os.Clearenv()
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
	cfg, err := Load("")
	require.NoError(t, err)
	at := cfg.AgentTracing
	assert.False(t, at.Enabled)
	assert.Equal(t, 10000, at.QueueMaxEvents)
	assert.Equal(t, int64(64<<20), at.QueueMaxBytes)
	assert.Equal(t, 1<<20, at.MaxStreamBufferBytes)
	assert.Equal(t, 10*time.Second, at.FlushInterval) // D3
	assert.Equal(t, 500, at.FlushMaxEvents)           // D3
	assert.Equal(t, 4<<20, at.ChunkMaxBytes)          // D3
}

func TestAgentTracingConfig_Overrides(t *testing.T) {
	os.Clearenv()
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
	t.Setenv("AGENT_TRACING_ENABLED", "true")
	t.Setenv("AGENT_TRACING_FLUSH_INTERVAL", "2s")
	t.Setenv("AGENT_TRACING_FLUSH_MAX_EVENTS", "50")
	t.Setenv("AGENT_TRACING_QUEUE_MAX_EVENTS", "200")
	cfg, err := Load("")
	require.NoError(t, err)
	at := cfg.AgentTracing
	assert.True(t, at.Enabled)
	assert.Equal(t, 2*time.Second, at.FlushInterval)
	assert.Equal(t, 50, at.FlushMaxEvents)
	assert.Equal(t, 200, at.QueueMaxEvents)
}

func TestAgentTracingConfig_Normalise(t *testing.T) {
	at := AgentTracingConfig{
		QueueMaxEvents: -1, QueueMaxBytes: 0, MaxStreamBufferBytes: -5,
		FlushInterval: -time.Second, FlushMaxEvents: 0, ChunkMaxBytes: 64 << 20,
	}
	warnings := at.normalise()
	assert.Len(t, warnings, 6)
	assert.Equal(t, AgentTracingConfig{
		QueueMaxEvents: 10000, QueueMaxBytes: 64 << 20, MaxStreamBufferBytes: 1 << 20,
		FlushInterval: 10 * time.Second, FlushMaxEvents: 500, ChunkMaxBytes: 4 << 20,
	}, at)

	valid := at
	assert.Empty(t, valid.normalise())
}

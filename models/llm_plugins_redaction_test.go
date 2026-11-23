package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An LLM that leaves the process (system events, cluster_events, object
// hooks) lists its plugins without their configuration, which can hold the
// plugins' own credentials (midsommar-clean-6bg5.47). A hook's result gets
// the stored configuration back by plugin id.
func TestRedactObjectCredentials_LLMPluginsCarryNoConfig(t *testing.T) {
	p1 := &Plugin{Name: "rate-limiter", Config: map[string]interface{}{"redis_addr": "redis:6379", "redis_password": "S3CRET-1"}}
	p1.ID = 1
	p2 := &Plugin{Name: "rag", Config: map[string]interface{}{"vault_token": "S3CRET-2"}}
	p2.ID = 2
	llm := &LLM{Name: "gpt", Plugins: []*Plugin{p1, nil, p2}}

	for _, in := range []any{llm, *llm} {
		out, ok := RedactObjectCredentials(in).(*LLM)
		require.True(t, ok)
		b, err := json.Marshal(out)
		require.NoError(t, err)
		assert.NotContains(t, string(b), "S3CRET")
		assert.NotContains(t, string(b), "redis:6379")
		require.Len(t, out.Plugins, 3)
		assert.Equal(t, "rate-limiter", out.Plugins[0].Name)
		assert.Nil(t, out.Plugins[1])
	}
	assert.Equal(t, "S3CRET-1", p1.Config["redis_password"], "the stored plugins are not modified")
	assert.Same(t, p1, llm.Plugins[0])

	// An App's LLMs go through the same redaction.
	app := RedactObjectCredentials(&App{LLMs: []LLM{*llm}})
	b, err := json.Marshal(app)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "S3CRET")

	// A hook's result: configuration back by id; nothing the hook wrote there is taken.
	modified := &LLM{Name: "renamed", Plugins: []*Plugin{
		{ID: 2},
		{ID: 1, Config: map[string]interface{}{"redis_addr": "attacker:6379"}},
		{ID: 9, Config: map[string]interface{}{"injected": "x"}},
		nil,
	}}
	RestoreObjectCredentials(llm, modified)
	assert.Equal(t, p2.Config, modified.Plugins[0].Config)
	assert.Equal(t, p1.Config, modified.Plugins[1].Config)
	assert.Nil(t, modified.Plugins[2].Config, "a plugin the LLM did not have gets no configuration from a hook")
}

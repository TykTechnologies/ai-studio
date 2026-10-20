package api

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slugGateway resolves every slug to one LLM. Every other method panics
// through the nil embedded interface, which these tests never reach.
type slugGateway struct {
	services.GatewayServiceInterface
	llm *database.LLM
}

func (g slugGateway) GetLLMBySlug(string) (interface{}, error) { return g.llm, nil }

// headerChain stands in for the plugin manager's post_auth chain with one
// plugin. Like the real chain it hands the enriched request back whether or
// not the plugin changed it; set lays a plugin's returned headers over it.
type headerChain struct {
	set  map[string]string
	sent map[string]string
}

func (c *headerChain) GetPluginsForLLM(uint, interfaces.HookType) ([]*plugins.LoadedPlugin, error) {
	return []*plugins.LoadedPlugin{{ID: 1, Name: "p", IsHealthy: true}}, nil
}

func (c *headerChain) ExecutePluginChain(_ uint, _ interfaces.HookType, input interface{}, _ *interfaces.PluginContext) (interface{}, error) {
	req := input.(*interfaces.EnrichedRequest)
	c.sent = maps.Clone(req.PluginRequest.Headers)
	if len(c.set) > 0 {
		headers := maps.Clone(req.PluginRequest.Headers)
		maps.Copy(headers, c.set)
		req.PluginRequest.Headers = headers
	}
	return req, nil
}

func runPostAuth(t *testing.T, chain *headerChain) *http.Request {
	t.Helper()
	llms := &hookLLMLookup{gateway: slugGateway{llm: &database.LLM{Slug: "claude", Vendor: "anthropic"}}}
	hook := createPostAuthHook(&services.ServiceContainer{}, chain, llms)

	r := httptest.NewRequest(http.MethodPost, "/llm/rest/claude/v1/messages", strings.NewReader(`{"model":"m"}`))
	r.Header.Add("Anthropic-Beta", "prompt-caching-2024-07-31")
	r.Header.Add("Anthropic-Beta", "context-management-2025-06-27")
	r.Header.Add("Accept", "application/json")
	r.Header.Add("Accept", "text/event-stream")
	r.Header.Set("Anthropic-Version", "2023-06-01")
	r = r.WithContext(context.WithValue(r.Context(), "request_id", "req-1"))

	stop := hook(httptest.NewRecorder(), r, 3)
	require.False(t, stop, "the request continues to the proxy")
	return r
}

// A post_auth plugin that changes nothing must not cost a repeated header its
// later values: the chain hands back the first value of each, and setting
// them all again dropped the rest (anthropic-beta lines, Accept lists).
func TestPostAuthHook_UnmodifiedChainKeepsRepeatedHeaders(t *testing.T) {
	chain := &headerChain{}
	r := runPostAuth(t, chain)

	assert.Equal(t, "prompt-caching-2024-07-31", chain.sent["Anthropic-Beta"], "plugins still see the first value")
	assert.Equal(t, []string{"prompt-caching-2024-07-31", "context-management-2025-06-27"}, r.Header.Values("Anthropic-Beta"))
	assert.Equal(t, []string{"application/json", "text/event-stream"}, r.Header.Values("Accept"))
	assert.Equal(t, "2023-06-01", r.Header.Get("Anthropic-Version"))
}

// A plugin's own header changes still apply, and a header it hands back
// unchanged keeps all its values.
func TestPostAuthHook_ModifiedHeadersStillApply(t *testing.T) {
	chain := &headerChain{set: map[string]string{
		"X-Plugin":          "1",
		"Anthropic-Version": "2024-01-01",
		"accept":            "application/json", // the first value, sent back under another case
	}}
	r := runPostAuth(t, chain)

	assert.Equal(t, "1", r.Header.Get("X-Plugin"))
	assert.Equal(t, []string{"2024-01-01"}, r.Header.Values("Anthropic-Version"))
	assert.Equal(t, []string{"prompt-caching-2024-07-31", "context-management-2025-06-27"}, r.Header.Values("Anthropic-Beta"))
	assert.Equal(t, []string{"application/json", "text/event-stream"}, r.Header.Values("Accept"))
}

// A plugin that changes one value of a repeated header replaces all of them:
// it was sent one value, and that is the one it means.
func TestApplyPluginHeaders_ChangedRepeatedHeaderIsReplaced(t *testing.T) {
	h := http.Header{"Anthropic-Beta": {"a", "b"}, "X-Keep": {"k"}}
	sent := map[string]string{"Anthropic-Beta": "a", "X-Keep": "k"}

	applyPluginHeaders(h, sent, map[string]string{"Anthropic-Beta": "c", "X-Keep": "k"})

	assert.Equal(t, []string{"c"}, h.Values("Anthropic-Beta"))
	assert.Equal(t, []string{"k"}, h.Values("X-Keep"))
}

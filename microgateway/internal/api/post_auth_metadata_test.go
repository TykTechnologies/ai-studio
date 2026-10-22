package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contextChain is a post_auth chain that keeps the plugin context it ran with.
type contextChain struct {
	headerChain
	ctx *interfaces.PluginContext
}

func (c *contextChain) ExecutePluginChain(llmID uint, hook interfaces.HookType, input interface{}, pluginCtx *interfaces.PluginContext) (interface{}, error) {
	c.ctx = pluginCtx
	return c.headerChain.ExecutePluginChain(llmID, hook, input, pluginCtx)
}

// The post_auth context carries the model the request names and whether it
// streams, for every vendor (8xf.11); a Bedrock ARN model ID comes whole.
func TestPostAuthHook_ExchangeMetadata(t *testing.T) {
	const arn = "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-3"
	cases := []struct {
		vendor, path, body, model, stream string
	}{
		{"openai", "/llm/rest/m/v1/chat/completions", `{"model":"gpt-4o"}`, "gpt-4o", "false"},
		{"openai", "/llm/stream/m/v1/chat/completions", `{"model":"gpt-4o","stream":true}`, "gpt-4o", "true"},
		{"anthropic", "/llm/call/m/v1/messages", `{"model":"claude-sonnet-5","stream":true}`, "claude-sonnet-5", "true"},
		{"google_ai", "/llm/call/m/v1beta/models/gemini-2.5-flash:streamGenerateContent", `{"contents":[]}`, "gemini-2.5-flash", "true"},
		{"bedrock", "/llm/rest/m/model/arn%3Aaws%3Abedrock%3Aus-east-1%3A123456789012%3Ainference-profile%2Fus.anthropic.claude-3/converse", `{"messages":[]}`, arn, "false"},
		{"bedrock", "/llm/call/m/model/anthropic.claude-3/converse-stream", `{"messages":[]}`, "anthropic.claude-3", "true"},
	}
	for _, tc := range cases {
		t.Run(tc.vendor+" "+tc.path, func(t *testing.T) {
			chain := &contextChain{}
			llms := &hookLLMLookup{gateway: slugGateway{llm: &database.LLM{Slug: "m", Vendor: tc.vendor}}}
			hook := createPostAuthHook(&services.ServiceContainer{}, chain, llms)

			r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			r = r.WithContext(context.WithValue(r.Context(), "request_id", "req-1"))
			require.False(t, hook(httptest.NewRecorder(), r, 3))

			require.NotNil(t, chain.ctx)
			assert.Equal(t, tc.model, chain.ctx.Metadata["model"])
			assert.Equal(t, tc.stream, chain.ctx.Metadata["stream"])
		})
	}
}

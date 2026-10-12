package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms/anthropic"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The chat-session path reads the in-tree OpenAI driver's GenerationInfo as
// the driver really builds it, streamed and not: prompt_tokens includes
// prompt_tokens_details.cached_tokens, so the record carries the cached tokens
// apart and bills them at the (fallback) cache price, as the proxy does.
func TestRecordContentMessage_OpenAIDriverCachedTokens(t *testing.T) {
	const prompt, cached, completion = 1400, 400, 300
	usage := fmt.Sprintf(`{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d,"prompt_tokens_details":{"cached_tokens":%d}}`,
		prompt, completion, prompt+completion, cached)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !body.Stream {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"id":"c1","object":"chat.completion","model":"gpt-x","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":%s}`, usage)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"id":"c1","object":"chat.completion.chunk","model":"gpt-x","choices":[{"index":0,"delta":{"role":"assistant","content":"hello"}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"id":"c1","object":"chat.completion.chunk","model":"gpt-x","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprintf(w, `data: {"id":"c1","object":"chat.completion.chunk","model":"gpt-x","choices":[],"usage":%s}`+"\n\n", usage)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			db := setupStatsTest(t)
			const model = "gpt-x"
			p := models.ModelPrice{ModelName: model, Vendor: string(models.OPENAI), Currency: "USD", CPIT: 0.000010, CPT: 0.000030}
			require.NoError(t, db.Create(&p).Error)

			llm, err := openai.New(openai.WithBaseURL(srv.URL), openai.WithToken("x"), openai.WithModel(model))
			require.NoError(t, err)
			opts := []llms.CallOption{}
			if stream {
				opts = append(opts, llms.WithStreamingFunc(func(context.Context, []byte) error { return nil }))
			}
			msgs := []llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hi")}
			cr, err := llm.GenerateContent(context.Background(), msgs, opts...)
			require.NoError(t, err)

			var rec *models.LLMChatRecord
			withHandler(t, &mockHandler{onRecordChatRecord: func(r *models.LLMChatRecord) { rec = r }})
			RecordContentMessage(&msgs[0], cr, models.OPENAI, model, "chat-1", 10, 1, 1, 9, time.Now(), &services.Service{DB: db}, true)
			require.NotNil(t, rec)

			assert.Equal(t, prompt-cached, rec.PromptTokens, "prompt tokens exclude the cached ones")
			assert.Equal(t, cached, rec.CacheReadPromptTokens, "cache read tokens")
			assert.Equal(t, completion, rec.ResponseTokens, "response tokens")
			assert.Equal(t, prompt+completion, rec.TotalTokens, "total tokens = vendor total")
			// Unset cache price: cached tokens bill at the input price, so the
			// cost equals the plain list cost of the vendor's prompt count.
			assert.InDelta(t, (p.CPIT*prompt+p.CPT*completion)*10000, rec.Cost, 1e-6, "cost")
		})
	}
}

// The real in-tree Anthropic driver turns a text + tool_use reply (streamed or
// not) into one choice per content block, each with the whole usage; the
// chat-session record must carry that usage once.
func TestRecordContentMessage_AnthropicDriverTextAndToolUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !body.Stream {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-x","stop_reason":"tool_use",`+
				`"content":[{"type":"text","text":"let me check"},{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"q":"x"}}],`+
				`"usage":{"input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":200,"cache_read_input_tokens":300}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		ev := func(name, data string) { fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data) }
		ev("message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-x","content":[],"usage":{"input_tokens":1000,"output_tokens":1,"cache_creation_input_tokens":200,"cache_read_input_tokens":300}}}`)
		ev("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		ev("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"let me check"}}`)
		ev("content_block_stop", `{"type":"content_block_stop","index":0}`)
		ev("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"lookup","input":{}}}`)
		ev("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"x\"}"}}`)
		ev("content_block_stop", `{"type":"content_block_stop","index":1}`)
		ev("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":500}}`)
		ev("message_stop", `{"type":"message_stop"}`)
	}))
	t.Cleanup(srv.Close)

	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			db := setupStatsTest(t)
			const model = "claude-x"
			p := models.ModelPrice{ModelName: model, Vendor: string(models.ANTHROPIC), Currency: "USD",
				CPIT: 0.000010, CPT: 0.000030, CacheWritePT: 0.0000125, CacheReadPT: 0.000001}
			require.NoError(t, db.Create(&p).Error)

			llm, err := anthropic.New(anthropic.WithBaseURL(srv.URL+"/v1"), anthropic.WithToken("x"), anthropic.WithModel(model))
			require.NoError(t, err)
			opts := []llms.CallOption{llms.WithTools([]llms.Tool{{Type: "function", Function: &llms.FunctionDefinition{
				Name: "lookup", Description: "look up", Parameters: map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}}}}})}
			if stream {
				opts = append(opts, llms.WithStreamingFunc(func(context.Context, []byte) error { return nil }))
			}
			msgs := []llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hi")}
			cr, err := llm.GenerateContent(context.Background(), msgs, opts...)
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(cr.Choices), 2, "driver returns one choice per content block")

			var rec *models.LLMChatRecord
			withHandler(t, &mockHandler{onRecordChatRecord: func(r *models.LLMChatRecord) { rec = r }})
			RecordContentMessage(&msgs[0], cr, models.ANTHROPIC, model, "chat-1", 10, 1, 1, 9, time.Now(), &services.Service{DB: db}, true)
			require.NotNil(t, rec)

			assert.Equal(t, 1000, rec.PromptTokens, "prompt tokens")
			assert.Equal(t, 500, rec.ResponseTokens, "response tokens")
			assert.Equal(t, 200, rec.CacheWritePromptTokens, "cache write tokens")
			assert.Equal(t, 300, rec.CacheReadPromptTokens, "cache read tokens")
			assert.Equal(t, 2000, rec.TotalTokens, "total tokens")
			assert.InDelta(t, models.ChatRecordCost(string(models.ANTHROPIC), p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, 1000, 500, 200, 300), rec.Cost, 1e-6, "cost")
		})
	}
}

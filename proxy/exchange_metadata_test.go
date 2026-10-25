package proxy

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// metadataHook records the plugin context metadata of every response hook
// phase.
type metadataHook struct {
	mu       sync.Mutex
	headers  []map[string]string
	write    []map[string]string
	complete chan map[string]string
}

func newMetadataHook() *metadataHook {
	return &metadataHook{complete: make(chan map[string]string, 1)}
}

func (h *metadataHook) GetName() string { return "metadata-hook" }

func (h *metadataHook) OnBeforeWriteHeaders(_ context.Context, req *HeadersRequest) (*HeadersResponse, error) {
	h.mu.Lock()
	h.headers = append(h.headers, req.Context.Metadata)
	h.mu.Unlock()
	return &HeadersResponse{Headers: req.Headers}, nil
}

func (h *metadataHook) OnBeforeWrite(_ context.Context, req *ResponseWriteRequest) (*ResponseWriteResponse, error) {
	h.mu.Lock()
	h.write = append(h.write, req.Context.Metadata)
	h.mu.Unlock()
	return &ResponseWriteResponse{Body: req.Body, Headers: req.Headers}, nil
}

func (h *metadataHook) OnStreamComplete(_ context.Context, req *StreamCompleteRequest) (*StreamCompleteResponse, error) {
	h.complete <- req.Context.Metadata
	return &StreamCompleteResponse{}, nil
}

func (h *metadataHook) phases() (headers, write []map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.headers, h.write
}

// The response hooks' contexts carry the model the request names and whether
// it streams, for every vendor (8xf.11): buffered (header phase and
// OnBeforeWrite) and streamed (header phase and OnStreamComplete).
func TestResponseHooks_ExchangeMetadata(t *testing.T) {
	t.Parallel()
	const converse = `{"messages":[{"role":"user","content":[{"text":"hi"}]}]}`
	var signed http.Header
	cases := []struct {
		name   string
		vendor models.Vendor
		serve  http.HandlerFunc
		path   string
		body   string
		model  string
		stream bool
	}{
		{"openai", models.OPENAI, serveOpenAIText("hi"), "/llm/rest/primary/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`, "gpt-4o", false},
		{"openai stream", models.OPENAI, serveOpenAIText("hi"), "/llm/stream/primary/v1/chat/completions", failoverStreamBody, "gpt-4o", true},
		{"anthropic", models.ANTHROPIC, serveAnthropic(t), "/llm/rest/primary/v1/messages", `{"model":"claude-sonnet-5","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`, "claude-sonnet-5", false},
		{"google", models.GOOGLEAI, serveGemini, "/llm/rest/primary/v1beta/models/gemini-2.5-flash:generateContent", `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`, "gemini-2.5-flash", false},
		{"bedrock", models.BEDROCK, sigV4VerifyHandler("AKIDTEST", "secret", "us-east-1", &signed), "/llm/rest/primary/model/" + sdkEscapedARN + "/converse", converse, profileARN, false},
		{"bedrock stream", models.BEDROCK, sigV4StreamHandler(t, "us-east-1", &signed), "/llm/stream/primary/model/" + sdkEscapedARN + "/converse-stream", converse, profileARN, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newFailoverHarness(t, tc.serve, serveStatus(500), func(primary, _ *models.LLM) {
				primary.Vendor = tc.vendor
				primary.Failover = models.LLMFailover{}
				if tc.vendor == models.BEDROCK {
					primary.Metadata = models.JSONMap{"aws_access_key_id": "AKIDTEST", "aws_secret_access_key": "secret", "aws_region": "us-east-1"}
				}
			})
			hook := newMetadataHook()
			h.proxy.AddResponseHook(hook)

			resp, body := h.post(tc.path, tc.body)
			require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)

			want := map[string]string{MetadataModel: tc.model, MetadataStream: "false"}
			if tc.stream {
				want[MetadataStream] = "true"
			}
			headers, write := hook.phases()
			require.Len(t, headers, 1)
			assert.Subset(t, headers[0], want)
			if !tc.stream {
				require.Len(t, write, 1)
				assert.Subset(t, write[0], want)
				return
			}
			select {
			case got := <-hook.complete:
				assert.Subset(t, got, want)
			case <-time.After(5 * time.Second):
				t.Fatal("OnStreamComplete was not called")
			}
		})
	}
}

func TestExchangeMetadata(t *testing.T) {
	assert.Equal(t, map[string]string{"model": "gpt-4o", "stream": "true"},
		ExchangeMetadata("openai", "/llm/call/x/v1/chat/completions", []byte(`{"model":"gpt-4o","stream":true}`)))
	assert.Equal(t, map[string]string{"model": profileARN, "stream": "true"},
		ExchangeMetadata("bedrock", "/llm/call/x/model/"+sdkEscapedARN+"/converse-stream", []byte(`{}`)))
	assert.Equal(t, map[string]string{"stream": "false"},
		ExchangeMetadata("openai", "/llm/rest/x/v1/embeddings", []byte(`not json`)), "no model, no key")
}

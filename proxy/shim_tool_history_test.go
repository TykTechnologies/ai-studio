package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// agentLoop is an OpenAI-shaped agent turn: the model called two tools in
// parallel and both answered.
const agentLoop = `{"model":"gpt-4o","stream":false,"messages":[` +
	`{"role":"system","content":"You review code."},` +
	`{"role":"user","content":"review the PR"},` +
	`{"role":"assistant","content":null,"tool_calls":[` +
	`{"id":"call_r1","type":"function","function":{"name":"read","arguments":"{\"path\":\"a.go\"}"}},` +
	`{"id":"call_s1","type":"function","function":{"name":"shell","arguments":"{\"cmd\":\"ls\"}"}}]},` +
	`{"role":"tool","tool_call_id":"call_r1","content":"package a"},` +
	`{"role":"tool","tool_call_id":"call_s1","content":[{"type":"text","text":"a.go b.go"}]}],` +
	`"tools":[{"type":"function","function":{"name":"read","parameters":{"type":"object","properties":{"path":{"type":"string"}}}}},` +
	`{"type":"function","function":{"name":"shell","parameters":{"type":"object","properties":{"cmd":{"type":"string"}}}}}]}`

// kindRecorder records the kind and text of every segment it is given.
type kindRecorder struct {
	mu   sync.Mutex
	segs []guardrails.Segment
}

func (k *kindRecorder) Capabilities() guardrails.Capabilities {
	return guardrails.Capabilities{ToolSegments: true}
}

func (k *kindRecorder) Classify(_ context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	k.mu.Lock()
	k.segs = append(k.segs, in.Segments...)
	k.mu.Unlock()
	return guardrails.Verdict{}, nil
}

var (
	theKindRecorder      = &kindRecorder{}
	registerKindRecorder sync.Once
)

func kindRecordingGuardrail() *models.Filter {
	registerKindRecorder.Do(func() {
		guardrails.RegisterSpec(guardrails.Spec{Name: "test_kind_recorder", DisplayName: "kinds", OpenDetectors: true})
		guardrails.RegisterFactory("test_kind_recorder", func(guardrails.Config) (guardrails.Provider, error) { return theKindRecorder, nil })
	})
	return &models.Filter{Name: "kinds", Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider": "test_kind_recorder", "detectors": []any{map[string]any{"name": "x"}}, "on_detect": "log",
		"scope": "all_messages", "tool_calls": map[string]any{"scan": true}, "cache": false,
	}}
}

func upstreamJSON(t *testing.T, h *failoverHarness) map[string]any {
	t.Helper()
	h.primaryVendor.mu.Lock()
	defer h.primaryVendor.mu.Unlock()
	require.NotEmpty(t, h.primaryVendor.requests, "the vendor was not called")
	var doc map[string]any
	require.NoError(t, json.Unmarshal(h.primaryVendor.requests[len(h.primaryVendor.requests)-1].Body, &doc))
	return doc
}

// The /ai/ bridge carries an agent loop's tool calls and tool results to the
// vendor in its own shape (ids and names kept, parallel results in one
// turn), so the vendor sees the conversation the client sent and the inner
// hop's guardrails see tool results as tool results, not as user text.
func TestShimToolHistory_ReachesTheVendor(t *testing.T) {
	cases := shimCases()
	check := map[models.Vendor]func(t *testing.T, doc map[string]any){
		models.OPENAI: func(t *testing.T, doc map[string]any) {
			msgs := doc["messages"].([]any)
			require.Len(t, msgs, 5, doc)
			asst := msgs[2].(map[string]any)
			calls := asst["tool_calls"].([]any)
			require.Len(t, calls, 2)
			assert.Equal(t, "call_r1", calls[0].(map[string]any)["id"])
			assert.Equal(t, "read", calls[0].(map[string]any)["function"].(map[string]any)["name"])
			assert.JSONEq(t, `{"path":"a.go"}`, calls[0].(map[string]any)["function"].(map[string]any)["arguments"].(string))
			assert.Equal(t, "tool", msgs[3].(map[string]any)["role"])
			assert.Equal(t, "call_r1", msgs[3].(map[string]any)["tool_call_id"])
			assert.Equal(t, "package a", msgs[3].(map[string]any)["content"])
			assert.Equal(t, "call_s1", msgs[4].(map[string]any)["tool_call_id"])
			assert.Equal(t, "a.go b.go", msgs[4].(map[string]any)["content"])
		},
		models.ANTHROPIC: func(t *testing.T, doc map[string]any) {
			msgs := doc["messages"].([]any)
			require.Len(t, msgs, 3, doc)
			asst := msgs[1].(map[string]any)["content"].([]any)
			require.Len(t, asst, 2, "the assistant turn holds both tool_use blocks: %v", asst)
			assert.Equal(t, "tool_use", asst[0].(map[string]any)["type"])
			assert.Equal(t, "call_r1", asst[0].(map[string]any)["id"])
			assert.Equal(t, map[string]any{"path": "a.go"}, asst[0].(map[string]any)["input"])
			results := msgs[2].(map[string]any)["content"].([]any)
			require.Len(t, results, 2, "both results in one user turn: %v", results)
			assert.Equal(t, "tool_result", results[0].(map[string]any)["type"])
			assert.Equal(t, "call_r1", results[0].(map[string]any)["tool_use_id"])
			assert.Equal(t, "call_s1", results[1].(map[string]any)["tool_use_id"])
		},
		models.GOOGLEAI: func(t *testing.T, doc map[string]any) {
			contents := doc["contents"].([]any)
			require.Len(t, contents, 3, doc)
			model := contents[1].(map[string]any)
			assert.Equal(t, "model", model["role"])
			parts := model["parts"].([]any)
			require.Len(t, parts, 2)
			assert.Equal(t, "read", parts[0].(map[string]any)["functionCall"].(map[string]any)["name"])
			resp := contents[2].(map[string]any)["parts"].([]any)
			require.Len(t, resp, 2, "both responses in one turn: %v", resp)
			assert.Equal(t, "read", resp[0].(map[string]any)["functionResponse"].(map[string]any)["name"])
			assert.Equal(t, "shell", resp[1].(map[string]any)["functionResponse"].(map[string]any)["name"])
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			theKindRecorder.mu.Lock()
			theKindRecorder.segs = nil
			theKindRecorder.mu.Unlock()
			h := shimHarness(t, tc, strings.ReplaceAll(tc.text, "FORBIDDEN", "fine"), kindRecordingGuardrail())
			resp, body := h.post("/ai/primary/v1/chat/completions", agentLoop)
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
			check[tc.vendor](t, upstreamJSON(t, h))

			theKindRecorder.mu.Lock()
			defer theKindRecorder.mu.Unlock()
			// Every segment holding a tool result's text is a tool result
			// (Gemini wraps it in {"response": ...}, so match by substring);
			// the user's message stays user text.
			kindsOf := func(needle string) map[string]bool {
				out := map[string]bool{}
				for _, s := range theKindRecorder.segs {
					if strings.Contains(s.Text, needle) {
						out[s.Role+"/"+s.Kind] = true
					}
				}
				return out
			}
			assert.Equal(t, map[string]bool{"tool/" + guardrails.KindToolResult: true}, kindsOf("package a"))
			assert.Equal(t, map[string]bool{"tool/" + guardrails.KindToolResult: true}, kindsOf("a.go b.go"))
			assert.Equal(t, map[string]bool{"user/": true}, kindsOf("review the PR"))
			assert.Equal(t, map[string]bool{"assistant/" + guardrails.KindToolCall: true}, kindsOf("path"), "the read call's arguments")
		})
	}
}

// Over the Bedrock bridge too: toolUse blocks in the assistant turn, every
// toolResult in one user turn, never an empty text block.
func TestShimToolHistory_Bedrock(t *testing.T) {
	rec := &bedrockRecorder{reply: bedrockStream(t, "fine", "{}")}
	h := bedrockHarness(t, rec)
	resp, body := h.post("/ai/primary/v1/chat/completions", strings.Replace(agentLoop, `"stream":false`, `"stream":true`, 1), "Authorization", "Bearer "+h.apiKey)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var doc struct {
		Messages []struct {
			Role    string           `json:"role"`
			Content []map[string]any `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(rec.body, &doc), string(rec.body))
	require.Len(t, doc.Messages, 3, string(rec.body))
	asst := doc.Messages[1]
	assert.Equal(t, "assistant", asst.Role)
	require.Len(t, asst.Content, 2, string(rec.body))
	use := asst.Content[0]["toolUse"].(map[string]any)
	assert.Equal(t, "call_r1", use["toolUseId"])
	assert.Equal(t, "read", use["name"])
	assert.Equal(t, map[string]any{"path": "a.go"}, use["input"])
	results := doc.Messages[2]
	assert.Equal(t, "user", results.Role)
	require.Len(t, results.Content, 2, string(rec.body))
	assert.Equal(t, "call_r1", results.Content[0]["toolResult"].(map[string]any)["toolUseId"])
	assert.Equal(t, "call_s1", results.Content[1]["toolResult"].(map[string]any)["toolUseId"])
	assert.NotContains(t, string(rec.body), `{"text":""}`)
}

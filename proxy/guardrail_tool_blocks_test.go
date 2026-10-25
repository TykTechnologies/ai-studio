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

// A redacting guardrail on a tool-using conversation must change only the
// redacted strings: every tool block, id, image and cache_control reaches the
// vendor as the client sent it. Before block-level segmentation the runner
// flattened the conversation to {role, content} and the reconstructor
// rebuilt the body from that, dropping tool_use / tool_result / tool_calls /
// tool_call_id.

// secretRedactor flags "SECRET" in any segment and rewrites it to
// "[REDACTED]". It is registered as a guardrail provider so the test runs in
// both editions.
type secretRedactor struct{}

func (secretRedactor) Capabilities() guardrails.Capabilities {
	return guardrails.Capabilities{Redacts: true, Local: true}
}

func (secretRedactor) Classify(_ context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	v := guardrails.Verdict{}
	for _, s := range in.Segments {
		if !strings.Contains(s.Text, "SECRET") {
			continue
		}
		v.Flagged = true
		v.Findings = append(v.Findings, guardrails.Finding{
			Detector: "secrets.test", Category: "secrets", Score: 1, Severity: "critical", SegmentIndex: s.Index,
		})
		if in.Redact != nil {
			if v.Rewritten == nil {
				v.Rewritten = map[int]string{}
			}
			v.Rewritten[s.Index] = strings.ReplaceAll(s.Text, "SECRET", "[REDACTED]")
		}
	}
	return v, nil
}

const secretRedactorProvider = "proxy_test_secret_redactor"

var registerSecretRedactor sync.Once

func redactGuardrail(scope string) *models.Filter {
	registerSecretRedactor.Do(func() {
		guardrails.RegisterSpec(guardrails.Spec{
			Name: secretRedactorProvider, DisplayName: "test redactor", Redacts: true,
			Detectors: []guardrails.DetectorSpec{{Name: "secrets", Category: "secrets"}},
		})
		guardrails.RegisterFactory(secretRedactorProvider, func(guardrails.Config) (guardrails.Provider, error) {
			return secretRedactor{}, nil
		})
	})
	return &models.Filter{
		Name: "redact " + scope,
		Kind: models.FilterKindGuardrail,
		Config: models.JSONMap{
			"provider":  secretRedactorProvider,
			"detectors": []any{map[string]any{"name": "secrets"}},
			"on_detect": guardrails.ActionRedact,
			"scope":     scope,
		},
	}
}

// replaceLeaves decodes body and applies replace to every string in it.
func replaceLeaves(t *testing.T, body string, replace func(string) string) any {
	t.Helper()
	var doc any
	require.NoError(t, json.Unmarshal([]byte(body), &doc))
	var walk func(v any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case string:
			return replace(x)
		case map[string]any:
			for k, e := range x {
				x[k] = walk(e)
			}
		case []any:
			for i, e := range x {
				x[i] = walk(e)
			}
		}
		return v
	}
	return walk(doc)
}

func decodeJSON(t *testing.T, b []byte) any {
	t.Helper()
	var doc any
	require.NoError(t, json.Unmarshal(b, &doc), string(b))
	return doc
}

const anthropicToolConversation = `{"model":"claude-sonnet-5","max_tokens":64,
	"system":[{"type":"text","text":"You are an agent.","cache_control":{"type":"ephemeral"}}],
	"messages":[
		{"role":"user","content":"Use the key SECRET to read the file."},
		{"role":"assistant","content":[
			{"type":"text","text":"Reading it."},
			{"type":"tool_use","id":"toolu_01","name":"read_file","input":{"path":".env"}}
		]},
		{"role":"user","content":[
			{"type":"tool_result","tool_use_id":"toolu_01","is_error":false,"content":[
				{"type":"text","text":"API_KEY=SECRET"},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}
			]},
			{"type":"text","text":"Summarise, but never repeat SECRET.","cache_control":{"type":"ephemeral"}}
		]}
	]}`

const openAIToolConversation = `{"model":"gpt-4o","messages":[
		{"role":"system","content":"You are an agent."},
		{"role":"user","name":"alice","content":[{"type":"text","text":"Use the key SECRET to read the file."},{"type":"image_url","image_url":{"url":"https://example.com/a.png","detail":"low"}}]},
		{"role":"assistant","content":null,"tool_calls":[
			{"id":"call_a","type":"function","function":{"name":"read_file","arguments":"{\"path\":\".env\"}"}},
			{"id":"call_b","type":"function","function":{"name":"list_dir","arguments":"{}"}}
		]},
		{"role":"tool","tool_call_id":"call_a","content":"API_KEY=SECRET"},
		{"role":"tool","tool_call_id":"call_b","content":[{"type":"text","text":".env README.md"}]},
		{"role":"user","content":"Summarise, but never repeat SECRET."}
	],"tools":[{"type":"function","function":{"name":"read_file","parameters":{"type":"object"}}}],"parallel_tool_calls":true}`

func TestGuardrailRedact_KeepsToolBlocksUpstream(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		vendor models.Vendor
		path   string
		body   string
		serve  http.HandlerFunc
	}{
		{"anthropic", models.ANTHROPIC, "/llm/rest/primary/v1/messages", anthropicToolConversation, serveAnthropic(t)},
		{"openai", models.OPENAI, "/llm/rest/primary/v1/chat/completions", openAIToolConversation, serveOpenAI},
	}
	redactAll := func(s string) string { return strings.ReplaceAll(s, "SECRET", "[REDACTED]") }
	for _, tc := range cases {
		for variant, c := range map[string]struct {
			scope            string
			scanTools        bool
			wantToolRedacted bool
		}{
			"all_user":                {guardrails.ScopeAllUser, true, true},
			"all_user, scan off":      {guardrails.ScopeAllUser, false, false},
			"all_messages, scan off":  {guardrails.ScopeAllMessages, false, true},
			"all_messages (defaults)": {guardrails.ScopeAllMessages, true, true},
		} {
			wantToolRedacted := c.wantToolRedacted
			t.Run(tc.name+"/"+variant, func(t *testing.T) {
				filter := redactGuardrail(c.scope)
				if !c.scanTools {
					filter.Config["tool_results"] = map[string]any{"scan": false}
				}
				h := newFailoverHarness(t, tc.serve, serveStatus(500), func(primary, _ *models.LLM) {
					primary.Vendor = tc.vendor
					primary.Failover = models.LLMFailover{}
					primary.Filters = []*models.Filter{filter}
				})
				resp, out := h.post(tc.path, tc.body)
				require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

				calls := h.primaryVendor.calls()
				require.Len(t, calls, 1)
				sent := decodeJSON(t, calls[0].Body)

				// Only the redacted strings changed: the user texts always,
				// the tool result's text only when the scope includes it.
				want := replaceLeaves(t, tc.body, func(s string) string {
					if s == "API_KEY=SECRET" && !wantToolRedacted {
						return s
					}
					return redactAll(s)
				})
				assert.Equal(t, want, sent)
			})
		}
	}
}

// Native batch items go through the same request filter chain, one item at a
// time in the item's own format: a redaction inside a Message Batch keeps
// the item's tool blocks.
func TestGuardrailRedact_KeepsToolBlocksInBatchItems(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{mutateLLM: func(l *models.LLM) {
		l.Filters = []*models.Filter{redactGuardrail(guardrails.ScopeAllMessages)}
	}})
	params := strings.Replace(anthropicToolConversation, `"claude-sonnet-5"`, `"claude-luna"`, 1)
	body := `{"requests":[{"custom_id":"agent","params":` + params + `}]}`
	resp, out := h.do(h.keyA, http.MethodPost, batchesPath, body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

	calls := h.vendor.callsTo(http.MethodPost, "/v1/messages/batches")
	require.Len(t, calls, 1)
	var sent struct {
		Requests []struct {
			Params json.RawMessage `json:"params"`
		} `json:"requests"`
	}
	require.NoError(t, json.Unmarshal(calls[0].Body, &sent))
	require.Len(t, sent.Requests, 1)
	assert.Equal(t, replaceLeaves(t, params, func(s string) string {
		return strings.ReplaceAll(s, "SECRET", "[REDACTED]")
	}), decodeJSON(t, sent.Requests[0].Params))
}

// The next filter in the chain sees the body the guardrail rewrote: a
// blocking guardrail after the redacting one finds nothing left to block.
func TestGuardrailRedact_NextFilterSeesRewrittenBody(t *testing.T) {
	p := New(nil, nil, &Config{})
	block := redactGuardrail(guardrails.ScopeAllMessages)
	block.Name = "block secrets"
	block.Config["on_detect"] = guardrails.ActionBlock
	r, _ := http.NewRequest(http.MethodPost, "/llm/rest/primary/v1/messages", strings.NewReader(anthropicToolConversation))

	// Alone, the blocking guardrail blocks the original body...
	_, err := p.runRequestFilters(&models.LLM{Vendor: models.ANTHROPIC, Filters: []*models.Filter{block}},
		r, []byte(anthropicToolConversation), "anthropic", "claude-sonnet-5")
	require.ErrorContains(t, err, "block secrets")

	// ...after the redaction it passes, and the tool blocks are intact.
	llm := &models.LLM{Vendor: models.ANTHROPIC, Filters: []*models.Filter{redactGuardrail(guardrails.ScopeAllMessages), block}}
	out, err := p.runRequestFilters(llm, r, []byte(anthropicToolConversation), "anthropic", "claude-sonnet-5")
	require.NoError(t, err)
	assert.Equal(t, replaceLeaves(t, anthropicToolConversation, func(s string) string {
		return strings.ReplaceAll(s, "SECRET", "[REDACTED]")
	}), decodeJSON(t, out))
}

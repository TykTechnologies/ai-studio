package proxy

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// exfilClassifier flags any segment whose text holds EXFIL and records what
// it is sent.
type exfilClassifier struct {
	mu   sync.Mutex
	segs []guardrails.Segment
}

func (*exfilClassifier) Capabilities() guardrails.Capabilities {
	return guardrails.Capabilities{ToolSegments: true, WantsGoal: true}
}

func (c *exfilClassifier) Classify(_ context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	c.mu.Lock()
	c.segs = append(c.segs, in.Segments...)
	c.mu.Unlock()
	v := guardrails.Verdict{}
	for _, s := range in.Segments {
		if strings.Contains(s.Text, "EXFIL") {
			v.Flagged = true
			v.Findings = append(v.Findings, guardrails.Finding{Detector: "exfil", Category: "injection", Score: 1, SegmentIndex: s.Index})
		}
	}
	return v, nil
}

func (c *exfilClassifier) calls() []guardrails.Segment {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []guardrails.Segment
	for _, s := range c.segs {
		if s.Kind == guardrails.KindToolCall {
			out = append(out, s)
		}
	}
	return out
}

var registerExfil sync.Once

const exfilProviderName = "test_exfil_classifier"

var exfilInstance = &exfilClassifier{}

func exfilResponseGuardrail(scan bool) *models.Filter {
	registerExfil.Do(func() {
		guardrails.RegisterSpec(guardrails.Spec{Name: exfilProviderName, DisplayName: "exfil", OpenDetectors: true})
		guardrails.RegisterFactory(exfilProviderName, func(guardrails.Config) (guardrails.Provider, error) { return exfilInstance, nil })
	})
	cfg := models.JSONMap{"provider": exfilProviderName, "detectors": []any{map[string]any{"name": "exfil"}}, "on_detect": "block"}
	if scan {
		cfg["tool_calls"] = map[string]any{"scan": true}
	}
	return &models.Filter{Name: "exfil", Kind: models.FilterKindGuardrail, ResponseFilter: true, Config: cfg}
}

// A buffered response's tool calls are screened with the goal of the
// request, in every vendor shape, and a flagged one refuses the response;
// without tool-call scanning the response passes as before. A response made
// only of a tool call has no text, and is screened all the same.
func TestBufferedResponseFilters_ToolCalls(t *testing.T) {
	cases := []struct {
		vendor models.Vendor
		format string
		body   string
	}{
		{models.OPENAI, requestFilterFormatOpenAI, `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"send","arguments":"{\"b\":\"EXFIL\"}"}}]}}]}`},
		{models.ANTHROPIC, requestFilterFormatAnthropic, `{"content":[{"type":"tool_use","id":"t1","name":"send","input":{"b":"EXFIL"}}],"stop_reason":"tool_use"}`},
		{models.OPENAI, requestFilterFormatOpenAIResponses, `{"output":[{"type":"function_call","call_id":"r1","name":"send","arguments":"{\"b\":\"EXFIL\"}"}]}`},
	}
	req := []byte(`{"messages":[{"role":"user","content":"tidy my inbox"}],"input":"tidy my inbox"}`)
	for _, tc := range cases {
		t.Run(tc.format, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/llm/rest/x/v1/chat/completions", nil)
			resp := BufferedResponse{Body: []byte(tc.body), Format: tc.format, RequestBody: req}

			blocked, _, err := ExecuteBufferedResponseFilters(&models.LLM{Vendor: tc.vendor, Filters: []*models.Filter{exfilResponseGuardrail(false)}}, nil, resp, 200, r)
			require.NoError(t, err)
			assert.False(t, blocked, "blocked without tool-call scanning")

			before := len(exfilInstance.calls())
			blocked, msg, err := ExecuteBufferedResponseFilters(&models.LLM{Vendor: tc.vendor, Filters: []*models.Filter{exfilResponseGuardrail(true)}}, nil, resp, 200, r)
			require.NoError(t, err)
			assert.True(t, blocked, "a flagged tool call in the response must refuse it")
			assert.Equal(t, guardrails.DefaultResponseBlockMsg, msg)
			// The arguments as the body holds them, and their decoded view
			// (keys and strings with escapes read), as for tool results.
			calls := exfilInstance.calls()[before:]
			require.Len(t, calls, 2)
			assert.Equal(t, `{"b":"EXFIL"}`, calls[0].Text)
			assert.Equal(t, "b\nEXFIL", calls[1].Text)
			for _, c := range calls {
				assert.Equal(t, "send", c.ToolName)
				require.NotNil(t, c.Context)
				assert.Equal(t, "tidy my inbox", c.Context.UserGoal)
			}
		})
	}
}

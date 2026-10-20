package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/toolfixtures"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// The Responses extractor names the tool behind each tool output it lists,
// as the fixtures' manifest expects: a function or custom tool call's output
// by the call's name (matched on call_id), an MCP call by its own name and
// id, each tool an MCP server lists by that tool's name, and a built-in
// tool's results by the tool type and the item's id.
func TestResponsesExtractor_NamesToolResults(t *testing.T) {
	e := &OpenAIResponsesMessageExtractor{}
	for _, f := range toolfixtures.ByFormat(requestFilterFormatOpenAIResponses) {
		t.Run(f.Name, func(t *testing.T) {
			msgs, err := e.ExtractMessages(nil, f.Body)
			require.NoError(t, err)
			tools := e.messageTools(f.Body)
			require.Len(t, tools, len(msgs))
			for _, want := range f.ToolSegments() {
				i := slices.IndexFunc(msgs, func(m llms.MessageContent) bool {
					return m.Role == llms.ChatMessageTypeTool && strings.Contains(responsesMessageText(m), want.Contains)
				})
				require.GreaterOrEqual(t, i, 0, "no message holds %q", want.Contains)
				assert.Equal(t, scripting.MessageTool{Name: want.ToolName, CallID: want.ToolCallID}, tools[i], want.Path)
			}
			for i, m := range msgs {
				switch {
				case m.Role == llms.ChatMessageTypeTool:
				case tools[i].Call:
					// A tool call's arguments name the call (scanned only by
					// guardrails that scan tool calls).
					assert.Equal(t, llms.ChatMessageTypeAI, m.Role, "message %d is a call but not the model's", i)
					assert.NotEmpty(t, tools[i].Name, "message %d is a call with no tool name", i)
				default:
					assert.Zero(t, tools[i], "message %d is not a tool result", i)
				}
			}
		})
	}
}

// toolRecorder flags every text holding a fixture canary and records the
// segments it is sent. It understands tool segments.
type toolRecorder struct {
	mu   sync.Mutex
	segs []guardrails.Segment
}

func (*toolRecorder) Capabilities() guardrails.Capabilities {
	return guardrails.Capabilities{Local: true, ToolSegments: true}
}

func (r *toolRecorder) Classify(_ context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	r.mu.Lock()
	r.segs = append(r.segs, in.Segments...)
	r.mu.Unlock()
	v := guardrails.Verdict{}
	for _, s := range in.Segments {
		if strings.Contains(s.Text, toolfixtures.CanaryPrefix) {
			v.Flagged = true
			v.Findings = append(v.Findings, guardrails.Finding{Detector: "injection.test", Category: "injection", Score: 1, SegmentIndex: s.Index})
		}
	}
	return v, nil
}

const toolRecorderProvider = "proxy_test_tool_recorder"

var (
	registerToolRecorder sync.Once
	theToolRecorder      = &toolRecorder{}
)

// eventRecorder collects compliance events.
type eventRecorder struct {
	mu     sync.Mutex
	events []*models.ComplianceEvent
}

func (h *eventRecorder) RecordProxyLog(context.Context, *models.ProxyLog)                {}
func (h *eventRecorder) RecordChatLogEntry(context.Context, *models.LLMChatLogEntry)     {}
func (h *eventRecorder) RecordChatRecord(context.Context, *models.LLMChatRecord)         {}
func (h *eventRecorder) RecordToolCall(context.Context, string, time.Time, int, uint)    {}
func (h *eventRecorder) RecordChatRecordsBatch(context.Context, []*models.LLMChatRecord) {}
func (h *eventRecorder) RecordProxyLogsBatch(context.Context, []*models.ProxyLog)        {}
func (h *eventRecorder) SetAsGlobalHandler()                                             { analytics.SetHandler(h) }
func (h *eventRecorder) RecordComplianceEvents(_ context.Context, e []*models.ComplianceEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, e...)
}
func (*eventRecorder) RecordMCPCalls(context.Context, []models.MCPCallRecord) error { return nil }

// Tool outputs of a Responses request reach a tool-aware provider with the
// tool's name and call id, and the compliance events name the tools, as on
// the other formats.
func TestGuardrailToolResults_ResponsesNameTheTool(t *testing.T) {
	registerToolRecorder.Do(func() {
		guardrails.RegisterSpec(guardrails.Spec{Name: toolRecorderProvider, DisplayName: "tool recorder",
			Detectors: []guardrails.DetectorSpec{{Name: "injection", Category: "injection"}}})
		guardrails.RegisterFactory(toolRecorderProvider, func(guardrails.Config) (guardrails.Provider, error) { return theToolRecorder, nil })
	})
	old := analytics.GetHandler()
	sink := &eventRecorder{}
	analytics.SetHandler(sink)
	t.Cleanup(func() { analytics.SetHandler(old) })

	p := New(nil, nil, &Config{})
	f := &models.Filter{Name: "names", Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider": toolRecorderProvider, "detectors": []any{map[string]any{"name": "injection"}},
		"on_detect": guardrails.ActionLog, "scope": guardrails.ScopeAllMessages, "cache": false,
	}}
	for _, fx := range toolfixtures.ByFormat(requestFilterFormatOpenAIResponses) {
		t.Run(fx.Name, func(t *testing.T) {
			theToolRecorder.mu.Lock()
			theToolRecorder.segs = nil
			theToolRecorder.mu.Unlock()
			sink.mu.Lock()
			sink.events = nil
			sink.mu.Unlock()

			r, _ := http.NewRequest(http.MethodPost, "/llm/rest/primary/v1/responses", strings.NewReader(string(fx.Body)))
			_, err := p.runRequestFilters(&models.LLM{ID: 3, Vendor: models.OPENAI, Filters: []*models.Filter{f}}, r, fx.Body, requestFilterFormatOpenAIResponses, "gpt-5")
			require.NoError(t, err)

			theToolRecorder.mu.Lock()
			sent := slices.Clone(theToolRecorder.segs)
			theToolRecorder.mu.Unlock()
			var named []string
			for _, want := range fx.ToolSegments() {
				// Tool results and other text go in separate calls that run
				// concurrently, and an assistant message may repeat a tool
				// result's text: match tool segments only.
				i := slices.IndexFunc(sent, func(s guardrails.Segment) bool {
					return s.IsToolResult() && strings.Contains(s.Text, want.Contains)
				})
				require.GreaterOrEqual(t, i, 0, "the provider was not sent %q", want.Contains)
				assert.Equal(t, want.ToolName, sent[i].ToolName, want.Path)
				assert.Equal(t, want.ToolCallID, sent[i].ToolCallID, want.Path)
				if want.Injection {
					named = append(named, want.ToolName)
				}
			}

			sink.mu.Lock()
			events := slices.Clone(sink.events)
			sink.mu.Unlock()
			var got []string
			for _, e := range events {
				if e.EventType != "guardrail.injection.test" {
					continue
				}
				var meta struct {
					ToolNames []string `json:"tool_names"`
				}
				require.NoError(t, json.Unmarshal([]byte(e.Metadata), &meta))
				got = append(got, meta.ToolNames...)
			}
			for _, n := range named {
				assert.Contains(t, got, n, "the compliance event does not name %s", n)
			}
		})
	}
}

func responsesMessageText(m llms.MessageContent) string {
	var b strings.Builder
	for _, p := range m.Parts {
		if t, ok := p.(llms.TextContent); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

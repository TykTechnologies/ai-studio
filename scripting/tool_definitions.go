package scripting

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// Tool lists AI Studio builds itself (a Studio tool served as an MCP
// server, the tools of a Studio chat) are screened where they are built,
// not on every use, by the request guardrails among the attached filters
// that scan tool definitions. The list goes through the guardrail runner as
// the tools of an OpenAI request, so selection, caching, removal and events
// are those of the gateway path.

// DefinitionScreeners are the filters ScreenToolDefinitions runs: request
// guardrails that scan tool definitions.
func DefinitionScreeners(filters []*models.Filter) []*models.Filter {
	var out []*models.Filter
	for _, f := range filters {
		if f != nil && !f.ResponseFilter && f.IsGuardrail() && guardrails.ConfigScansToolDefinitions(f.Config) {
			out = append(out, f)
		}
	}
	return out
}

// ErrToolDefinitionsBlocked is a tool list a guardrail refused.
type ErrToolDefinitionsBlocked struct {
	Filter, Message string
}

func (e *ErrToolDefinitionsBlocked) Error() string {
	return fmt.Sprintf("tool definitions blocked by policy (%s): %s", e.Filter, e.Message)
}

// ScreenToolDefinitions runs the definition-scanning guardrails among
// filters over tools, in order, and returns the tools that remain: a
// flagged tool is dropped (remove) or the whole list refused
// (*ErrToolDefinitionsBlocked), per each filter's tool_definitions action.
// scope is the script context (session, user, app ids: the tenant of the
// verdict cache); record records each filter's compliance events.
func ScreenToolDefinitions(ctx context.Context, filters []*models.Filter, tools []llms.Tool, scope map[string]interface{}, record func(context.Context, string, *ScriptOutput), svc services.ServiceInterface) ([]llms.Tool, error) {
	screeners := DefinitionScreeners(filters)
	if len(screeners) == 0 || len(tools) == 0 {
		return tools, nil
	}
	body, err := toolListBody(tools)
	if err != nil {
		return nil, err
	}
	for _, f := range screeners {
		in := &ScriptInput{RawInput: body, Format: "openai", Context: scope, Ctx: ctx, RecordEvents: record}
		out, err := NewFilterRunner(f).RunScript(in, svc)
		if err != nil {
			return nil, fmt.Errorf("guardrail '%s' on tool definitions: %w", f.Name, err)
		}
		if record != nil {
			record(ctx, f.Name, out)
		}
		if out.Block {
			return nil, &ErrToolDefinitionsBlocked{Filter: f.Name, Message: out.Message}
		}
		if out.Payload != "" && out.Payload != body {
			kept, err := keptTools(out.Payload, tools)
			if err != nil {
				return nil, err
			}
			tools, body = kept, out.Payload
		}
	}
	return tools, nil
}

// toolListBody is tools as an OpenAI chat request.
func toolListBody(tools []llms.Tool) (string, error) {
	b, err := json.Marshal(map[string]any{"messages": []any{}, "tools": tools})
	if err != nil {
		return "", fmt.Errorf("encoding tool definitions: %w", err)
	}
	return string(b), nil
}

// keptTools is tools without those a guardrail removed from body, which
// keeps the remaining definitions in order.
func keptTools(body string, tools []llms.Tool) ([]llms.Tool, error) {
	var parsed struct {
		Tools []llms.Tool `json:"tools"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return nil, fmt.Errorf("reading screened tool definitions: %w", err)
	}
	left := map[string]int{}
	for _, t := range parsed.Tools {
		left[toolName(t)]++
	}
	var out []llms.Tool
	for _, t := range tools {
		if n := toolName(t); left[n] > 0 {
			left[n]--
			out = append(out, t)
		}
	}
	return out, nil
}

func toolName(t llms.Tool) string {
	if t.Function == nil {
		return strings.TrimSpace(t.Type)
	}
	return t.Function.Name
}

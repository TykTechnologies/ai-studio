package proxy

import (
	"fmt"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// toolFilterPointers is the tool's filters as the runner takes them.
func toolFilterPointers(tool *models.Tool) []*models.Filter {
	out := make([]*models.Filter, len(tool.Filters))
	for i := range tool.Filters {
		out[i] = &tool.Filters[i]
	}
	return out
}

// screenMCPToolDefinitions screens the definitions of a Studio tool served
// over MCP with the tool's request guardrails that scan tool definitions,
// and returns the tools and operations (parallel slices) that remain. A
// guardrail that blocks refuses the whole server.
func (p *Proxy) screenMCPToolDefinitions(r *http.Request, tool *models.Tool, tools []llms.Tool, operations []string) ([]llms.Tool, []string, error) {
	filters := toolFilterPointers(tool)
	if len(scripting.DefinitionScreeners(filters)) == 0 {
		return tools, operations, nil
	}
	opOf := make(map[string]string, len(tools))
	for i, t := range tools {
		if t.Function != nil && i < len(operations) {
			opOf[t.Function.Name] = operations[i]
		}
	}
	scope := map[string]interface{}{"tool_id": int64(tool.ID), "tool_name": tool.Name}
	record := scripting.EventRecorder("tool_definitions", 0, 0, 0, "", "")
	kept, err := scripting.ScreenToolDefinitions(r.Context(), filters, tools, scope, record, p.gatewayService)
	if err != nil {
		return nil, nil, fmt.Errorf("tool definitions refused: %w", err)
	}
	ops := make([]string, 0, len(kept))
	for _, t := range kept {
		if t.Function != nil {
			ops = append(ops, opOf[t.Function.Name])
		}
	}
	if len(kept) == 0 {
		return nil, nil, fmt.Errorf("tool definitions refused: every operation was removed by policy")
	}
	return kept, ops, nil
}

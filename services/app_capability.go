package services

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/mcpannotations"
	"github.com/TykTechnologies/midsommar/v2/universalclient"
)

// The app guardrail capability (features/GuardrailExceptions.md): what a full
// administrator attests an app can do, which bounds how far a guardrail
// exception may soften a verdict for it. Studio suggests a value from what it
// can see of the app (its tools, MCP servers and datasources); the
// administrator attests, since the gateway cannot see everything the app can
// do (a CI job's own GitHub token, say).

// ErrInvalidGuardrailCapability is a capability value the service refuses.
var ErrInvalidGuardrailCapability = errors.New("invalid guardrail capability")

// CapabilitySuggestion is Studio's suggested capability for an app and why.
type CapabilitySuggestion struct {
	Suggestion guardrails.Capability `json:"suggestion"`
	// Reasons explain the suggestion. For acting they name each tool
	// operation or MCP primitive that can act; otherwise what was seen.
	Reasons []string `json:"reasons"`
}

// SuggestAppCapability suggests a capability from the app's tools, MCP
// servers and datasources (load the app with App.Get, which preloads them):
//   - read_only when it has none of them;
//   - read_tools when every tool operation the app can call is read-only
//     (GET/HEAD/OPTIONS, or x-mcp-read-only: true on the operation), every
//     MCP tool primitive is marked readOnlyHint, and datasources (retrieval)
//     are read-only by nature;
//   - acting otherwise, including whatever Studio cannot tell (a spec it
//     cannot parse, an MCP server whose primitives are not known).
func SuggestAppCapability(app *models.App) CapabilitySuggestion {
	var acting, reading []string

	for i := range app.Tools {
		a, r := toolCapabilityReasons(&app.Tools[i])
		acting = append(acting, a...)
		reading = append(reading, r...)
	}
	for i := range app.MCPServers {
		a, r := mcpServerCapabilityReasons(&app.MCPServers[i])
		acting = append(acting, a...)
		reading = append(reading, r...)
	}
	for _, ds := range app.Datasources {
		reading = append(reading, fmt.Sprintf("Datasource %q: retrieval only", ds.Name))
	}

	switch {
	case len(acting) > 0:
		return CapabilitySuggestion{Suggestion: guardrails.CapabilityActing, Reasons: acting}
	case len(reading) > 0:
		return CapabilitySuggestion{Suggestion: guardrails.CapabilityReadTools, Reasons: reading}
	default:
		return CapabilitySuggestion{
			Suggestion: guardrails.CapabilityReadOnly,
			Reasons:    []string{"No tools, datasources or MCP servers attached"},
		}
	}
}

// toolCapabilityReasons classifies the operations of a tool the app can call.
// A tool the app cannot reach (a chat-only tool) adds nothing.
func toolCapabilityReasons(t *models.Tool) (acting, reading []string) {
	if !t.AppGrantable() {
		return nil, nil
	}
	ops := t.GetOperations()
	if len(ops) == 0 {
		return nil, []string{fmt.Sprintf("Tool %q: exposes no operations", t.Name)}
	}
	spec, err := base64.StdEncoding.DecodeString(t.OASSpec)
	if err != nil || len(spec) == 0 {
		return []string{fmt.Sprintf("Tool %q: its OpenAPI spec could not be read", t.Name)}, nil
	}
	client, err := universalclient.NewClient(spec, "")
	if err != nil {
		return []string{fmt.Sprintf("Tool %q: its OpenAPI spec could not be read", t.Name)}, nil
	}
	var readOps []string
	for _, op := range ops {
		op = strings.TrimSpace(op)
		if op == "" {
			continue
		}
		binding, err := client.GetOperationBinding(op)
		if err != nil {
			acting = append(acting, fmt.Sprintf("Tool %q: operation %s is not in its spec", t.Name, op))
			continue
		}
		ann := mcpannotations.ApplyOverrides(mcpannotations.ForMethod(binding.Method), binding.Extensions)
		if ann.ReadOnlyHint == nil || !*ann.ReadOnlyHint {
			acting = append(acting, fmt.Sprintf("Tool %q: operation %s (%s) is not read-only", t.Name, op, binding.Method))
			continue
		}
		readOps = append(readOps, op)
	}
	if len(acting) > 0 {
		return acting, nil
	}
	if len(readOps) == 0 {
		return nil, []string{fmt.Sprintf("Tool %q: exposes no operations", t.Name)}
	}
	return nil, []string{fmt.Sprintf("Tool %q: read-only operations only (%s)", t.Name, strings.Join(readOps, ", "))}
}

// mcpServerCapabilityReasons classifies an MCP server's known primitives.
// Resources and prompts only read; a tool reads only when it is annotated
// readOnlyHint: true (the MCP default is false).
func mcpServerCapabilityReasons(m *models.MCPServer) (acting, reading []string) {
	prims := m.Primitives()
	if len(prims) == 0 {
		return []string{fmt.Sprintf("MCP server %q: its primitives are not known", m.Name)}, nil
	}
	for _, p := range prims {
		if p.Type != "tool" {
			continue
		}
		if ro, _ := p.Annotations["readOnlyHint"].(bool); !ro {
			acting = append(acting, fmt.Sprintf("MCP server %q: tool %s is not marked read-only", m.Name, p.Name))
		}
	}
	if len(acting) > 0 {
		return acting, nil
	}
	return nil, []string{fmt.Sprintf("MCP server %q: read-only primitives only", m.Name)}
}

// SetAppGuardrailCapability records the capability a full administrator
// attests for an app ("" clears it back to unattested). Only the column is
// written. The API restricts it to full administrators.
func (s *Service) SetAppGuardrailCapability(appID uint, value string, userID uint) (*models.App, error) {
	c, err := guardrails.ParseCapability(value)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidGuardrailCapability, err)
	}
	app, err := s.GetAppByID(appID)
	if err != nil {
		return nil, err
	}
	if app.GuardrailCapability == string(c) {
		return app, nil
	}
	if err := s.DB.Model(&models.App{}).Where("id = ?", appID).
		Update("guardrail_capability", string(c)).Error; err != nil {
		return nil, err
	}
	app.GuardrailCapability = string(c)
	if s.SystemEvents != nil {
		s.SystemEvents.EmitAppUpdated(app, app.ID, userID)
	}
	return app, nil
}

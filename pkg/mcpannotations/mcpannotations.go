// Package mcpannotations derives MCP tool annotations (readOnlyHint,
// destructiveHint, ...) for an OpenAPI operation: from its HTTP method,
// overridden by x-mcp-* extensions on the operation. The proxy uses them on
// the tool MCP endpoint; the app guardrail capability suggestion uses them to
// tell read-only tools from acting ones.
package mcpannotations

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/universalclient"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCP annotation overrides an author can set on an OpenAPI operation. The HTTP
// method is only ever an approximation of what an operation does - a POST that
// runs a search is not destructive, and a GET that mints a resource is - so a
// spec can say so directly.
const (
	ExtReadOnlyHint    = "x-mcp-read-only"
	ExtDestructiveHint = "x-mcp-destructive"
	ExtIdempotentHint  = "x-mcp-idempotent"
	ExtOpenWorldHint   = "x-mcp-open-world"
)

// ForMethod derives MCP tool annotations from an operation's HTTP
// method. Without this every operation inherited mcp.NewTool's conservative
// defaults - readOnly false, destructive true - so a read-only GET was rendered
// with a red DESTRUCTIVE badge, and clients that gate on destructiveHint
// treated every tool as dangerous, devaluing the hint where it matters.
func ForMethod(method string) mcp.ToolAnnotation {
	annotation := mcp.ToolAnnotation{
		// Every operation calls out to a third-party API.
		OpenWorldHint: mcp.ToBoolPtr(true),
	}

	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		annotation.ReadOnlyHint = mcp.ToBoolPtr(true)
		annotation.DestructiveHint = mcp.ToBoolPtr(false)
		annotation.IdempotentHint = mcp.ToBoolPtr(true)
	case http.MethodPut, http.MethodDelete:
		annotation.ReadOnlyHint = mcp.ToBoolPtr(false)
		annotation.DestructiveHint = mcp.ToBoolPtr(true)
		annotation.IdempotentHint = mcp.ToBoolPtr(true)
	case http.MethodPost, http.MethodPatch:
		annotation.ReadOnlyHint = mcp.ToBoolPtr(false)
		annotation.DestructiveHint = mcp.ToBoolPtr(true)
		annotation.IdempotentHint = mcp.ToBoolPtr(false)
	default:
		// An unrecognised method gets the cautious defaults.
		annotation.ReadOnlyHint = mcp.ToBoolPtr(false)
		annotation.DestructiveHint = mcp.ToBoolPtr(true)
		annotation.IdempotentHint = mcp.ToBoolPtr(false)
	}

	return annotation
}

// ApplyOverrides lets the OpenAPI document override any hint the
// HTTP method implied. A non-boolean value is ignored rather than guessed at.
func ApplyOverrides(annotation mcp.ToolAnnotation, extensions map[string]string) mcp.ToolAnnotation {
	override := func(target **bool, name string) {
		raw, ok := extensions[name]
		if !ok {
			return
		}
		value, err := strconv.ParseBool(raw)
		if err != nil {
			logger.Warnf("MCP annotation %s: %q is not a boolean, ignoring", name, raw)
			return
		}
		*target = mcp.ToBoolPtr(value)
	}

	override(&annotation.ReadOnlyHint, ExtReadOnlyHint)
	override(&annotation.DestructiveHint, ExtDestructiveHint)
	override(&annotation.IdempotentHint, ExtIdempotentHint)
	override(&annotation.OpenWorldHint, ExtOpenWorldHint)

	return annotation
}

func ForOperation(client *universalclient.Client, operationID string) mcp.ToolAnnotation {
	binding, err := client.GetOperationBinding(operationID)
	if err != nil {
		logger.Warnf("MCP operation %s: could not resolve HTTP binding, using default annotations: %v", operationID, err)
		return ForMethod("")
	}
	return ApplyOverrides(ForMethod(binding.Method), binding.Extensions)
}

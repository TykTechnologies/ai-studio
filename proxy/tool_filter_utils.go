package proxy

import (
	"context"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
)

// toolBlockedMessage is what a caller sees when a governance filter stops a
// tool call. It is deliberately identical for input and output blocks: the
// filter's own reason goes to the logs and to compliance reporting, and an
// identical response either way means a caller cannot probe the filters to
// learn whether the downstream tool was reached.
const toolBlockedMessage = "blocked by policy"

// resolvedToolCtxKey carries the tool loaded for the current MCP request.
//
// The per-tool MCP server is cached and its handler closures capture the tool
// as it was when the cache was filled, so filters read from that copy would go
// stale whenever a filter is edited. Rebuilding the MCP server instead would
// drop every live SSE session on a filter edit, so the freshly loaded tool
// rides on the request context and the handler prefers it.
type resolvedToolCtxKeyType struct{}

var resolvedToolCtxKey = resolvedToolCtxKeyType{}

// withResolvedTool attaches the tool loaded for this request to its context.
func withResolvedTool(r *http.Request, tool *models.Tool) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), resolvedToolCtxKey, tool))
}

// resolvedToolFromContext returns the tool loaded for the current request,
// falling back to the one supplied by the caller when the context has none.
func resolvedToolFromContext(ctx context.Context, fallback *models.Tool) *models.Tool {
	if tool, ok := ctx.Value(resolvedToolCtxKey).(*models.Tool); ok && tool != nil {
		return tool
	}
	return fallback
}

// toolFilterIdentity builds the identity a tool filter script sees, from
// whatever the credential validator managed to establish. App and user are both
// best effort - an OAuth bearer caller on the MCP path has a user and no app,
// while an app credential has an app and no user.
func toolFilterIdentity(ctx context.Context, tool *models.Tool) scripting.ToolFilterIdentity {
	id := scripting.ToolFilterIdentity{
		ToolID:   tool.ID,
		ToolName: tool.Name,
	}

	if app, ok := ctx.Value("app").(*models.App); ok && app != nil {
		id.AppID = app.ID
	}

	if user, ok := ctx.Value("user").(*models.User); ok && user != nil {
		id.UserID = user.ID
	}

	return id
}

// recordToolPolicyBlock reports a blocked tool call to the metrics pipeline.
func recordToolPolicyBlock(ctx context.Context, scope string) {
	metrics.RecordPolicyBlock(ctx, scope, "filter")
}

package proxy

import (
	"path"
	"strings"
)

// joinUpstreamPath composes the path sent to the vendor from the path of the
// LLM's configured endpoint and the path the caller put after the route slug.
//
// Operators configure endpoints in every shape: the bare host
// (https://api.anthropic.com), the versioned root (https://api.anthropic.com/v1),
// and corporate proxies with or without a version segment
// (https://gw/anthropic, https://gw/anthropic/v1). Callers likewise arrive both
// with and without a version segment: native SDK clients send /v1/messages, and
// so does the OpenAI-compatible bridge. Concatenating naively yields
// /anthropic/v1/v1/messages for the last shape; only passing the caller's path
// through when the endpoint path is a prefix of it yields /messages for the first.
//
// So the two are joined with their overlap removed: the longest run of whole
// segments that ends the endpoint path and begins the caller's path appears
// once. Every shape above then resolves to .../v1/messages.
func joinUpstreamPath(upstreamPath, remainingPath string) string {
	base := strings.TrimSuffix(upstreamPath, "/")
	if base == "" {
		return remainingPath
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}

	segments := strings.Split(strings.TrimPrefix(base, "/"), "/")
	for i := range segments {
		overlap := "/" + strings.Join(segments[i:], "/")
		if remainingPath == overlap || strings.HasPrefix(remainingPath, overlap+"/") {
			return path.Join(base[:len(base)-len(overlap)], remainingPath)
		}
	}
	return path.Join(base, remainingPath)
}

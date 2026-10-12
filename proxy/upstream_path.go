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
// hasTraversalSegment reports whether a request path carries a ".." segment.
// joinUpstreamPath resolves those, so a path such as /v1/../../admin would
// climb out of the configured endpoint prefix on the vendor host. The routers
// this proxy is mounted in clean such paths before any handler runs, but the
// handlers reject them explicitly so that guarantee does not depend on how the
// proxy happens to be mounted.
func hasTraversalSegment(requestPath string) bool {
	for _, seg := range strings.Split(requestPath, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

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

// canonicalVendorPath is the vendor's own path for a request: the path
// joinUpstreamPath sends upstream, less any corporate prefix the LLM's
// endpoint carries. A version segment that ends the endpoint (the /v1 of
// https://gw/anthropic/v1) belongs to the vendor's API and is kept; whatever
// precedes it (the /anthropic) is the proxy's mount point and is dropped. So
// every endpoint shape that sends .../v1/messages/batches upstream names the
// same canonical /v1/messages/batches, which native operations are matched on.
func canonicalVendorPath(upstreamPath, remainingPath string) string {
	joined := joinUpstreamPath(upstreamPath, remainingPath)
	base := strings.Trim(upstreamPath, "/")
	if base == "" {
		return joined
	}
	segments := strings.Split(base, "/")
	if isVersionSegment(segments[len(segments)-1]) {
		segments = segments[:len(segments)-1]
	}
	prefix := ""
	if len(segments) > 0 {
		prefix = "/" + strings.Join(segments, "/")
	}
	if prefix == "" {
		return joined
	}
	if joined == prefix {
		return "/"
	}
	if strings.HasPrefix(joined, prefix+"/") {
		return joined[len(prefix):]
	}
	return joined
}

// isVersionSegment reports whether seg names an API version: v1, v2, v1beta,
// v1alpha2 and the like.
func isVersionSegment(seg string) bool {
	if len(seg) < 2 || seg[0] != 'v' || seg[1] < '0' || seg[1] > '9' {
		return false
	}
	for _, c := range seg[2:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z') {
			return false
		}
	}
	return true
}

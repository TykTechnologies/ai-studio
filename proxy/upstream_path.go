package proxy

import (
	"net/url"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/pkg/requestformat"
)

// joinUpstreamPath composes the path sent to the vendor from the LLM's
// endpoint path and the caller's path after the route slug, with their
// overlap removed (requestformat.JoinUpstreamPath).
func joinUpstreamPath(upstreamPath, remainingPath string) string {
	return requestformat.JoinUpstreamPath(upstreamPath, remainingPath)
}

// canonicalVendorPath is the vendor's own path for a request, less any
// corporate prefix the LLM's endpoint carries
// (requestformat.CanonicalVendorPath). Native operations are matched on it.
func canonicalVendorPath(upstreamPath, remainingPath string) string {
	return requestformat.CanonicalVendorPath(upstreamPath, remainingPath)
}

// rewriteLLMPathPrefix moves a request from one /llm/ route to another
// (/llm/call/{slug} to /llm/rest/{slug}, say), in its escaped form too. A
// Bedrock model ID arrives escaped (arn%3Aaws%3A...%2F...); rewriting only
// Path would drop that escaping, so the ARN's "/" would split the segment.
func rewriteLLMPathPrefix(u *url.URL, from, to string) {
	u.Path = strings.Replace(u.Path, from, to, 1)
	if u.RawPath != "" {
		u.RawPath = strings.Replace(u.RawPath, from, to, 1)
	}
}

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

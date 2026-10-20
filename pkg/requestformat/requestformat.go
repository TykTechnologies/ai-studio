// Package requestformat names the wire shape of an LLM request body (its
// filter format) from the LLM's vendor and the request's method and path. The
// proxy reads request filters, guardrails and buffered responses in this
// format, and gateway plugins that parse or rewrite bodies use the same
// function so they agree with the proxy.
//
// The format is a key of the proxy's message extractor and reconstructor
// registries and of guardrails/segments: a vendor name ("openai",
// "anthropic", "bedrock", ...) for the vendor's own chat shape, or one of the
// constants below for a body that is not.
//
// For resolves requests on the native /llm/{mode}/{slug}/ pass-through only,
// which is also the only route post_auth plugins see. The bridges carry
// their own formats and never reach post_auth with them:
//   - /ai/{slug} and /v1 (unified) take OpenAI-shaped bodies, and
//     /anthropic/{slug} Anthropic-shaped ones. To a non-Bedrock LLM they are
//     translated and re-enter the proxy on /llm/call/{slug}/, where post_auth
//     sees the vendor-native body this package resolves.
//   - To a Bedrock LLM they call AWS directly with OpenAI or Anthropic as the
//     filter format, and run no post_auth.
//
// The package depends on nothing in this module but pkg/decisions, so
// plugins can import it without the proxy, models or GORM.
package requestformat

import (
	"net/url"
	"path"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/pkg/decisions"
)

// Filter formats besides the vendors' own chat shapes.
const (
	// OpenAI and Anthropic are the bridges' formats (and those vendors' own).
	OpenAI    = "openai"
	Anthropic = "anthropic"
	// OpenAIResponses is an OpenAI Responses API body: "input" items and
	// "instructions" instead of "messages".
	OpenAIResponses = "openai_responses"
	// OpenAIInput is an embeddings, completions or moderations body, read
	// through "input" or "prompt".
	OpenAIInput = "openai_input"
	// OpenAIDecisions and SystemOne are decision-model bodies (see
	// pkg/decisions), whatever the LLM's vendor.
	OpenAIDecisions = "openai_decisions"
	SystemOne       = "systemone"
)

// ChainFormat is a vendor operation whose body is not the vendor's chat shape:
// requests with Method to the vendor-canonical Path are read as Format. The
// proxy's native chain hooks (pkg/nativeapi.ChainHook.FilterFormat) declare
// the same operations; a proxy test keeps the two in step.
type ChainFormat struct {
	Method string
	Path   string
	Format string
}

// ChainFormats lists, per vendor, the operations with their own format.
var ChainFormats = map[string][]ChainFormat{
	"openai": {
		{Method: "POST", Path: "/v1/responses", Format: OpenAIResponses},
		{Method: "POST", Path: "/v1/responses/input_tokens", Format: OpenAIResponses},
		{Method: "POST", Path: "/v1/responses/compact", Format: OpenAIResponses},
		{Method: "POST", Path: "/v1/embeddings", Format: OpenAIInput},
		{Method: "POST", Path: "/v1/completions", Format: OpenAIInput},
		{Method: "POST", Path: "/v1/moderations", Format: OpenAIInput},
	},
}

// For is the filter format of a request on the native pass-through: the
// vendor's own, a decision format for a decision request, or the format
// ChainFormats names for the operation. requestPath is the gateway path
// (/llm/rest/my-llm/v1/responses); upstreamEndpoint is the LLM's API endpoint
// (a URL or a bare path, "" when it has none), which decides the operation's
// canonical path. Edge plugins get it from GetLLM (LLMInfo.Endpoint).
func For(vendor, method, requestPath, upstreamEndpoint string) string {
	format := vendor
	switch f, _ := decisions.FormatForPath(requestPath); f {
	case decisions.FormatOpenAI:
		format = OpenAIDecisions
	case decisions.FormatSystemOne:
		format = SystemOne
	}
	if f := chainFormat(vendor, method, requestPath, upstreamEndpoint); f != "" {
		format = f
	}
	return format
}

func chainFormat(vendor, method, requestPath, upstreamEndpoint string) string {
	ops := ChainFormats[vendor]
	if len(ops) == 0 {
		return ""
	}
	_, _, rest, ok := SplitNativePath(requestPath)
	if !ok {
		return ""
	}
	endpoint, err := url.Parse(upstreamEndpoint)
	if err != nil {
		return ""
	}
	canonical := CanonicalVendorPath(endpoint.Path, rest)
	for _, op := range ops {
		if op.Method == method && op.Path == canonical {
			return op.Format
		}
	}
	return ""
}

// SplitNativePath splits a native pass-through path /llm/{mode}/{slug}/{rest}
// into its mode (rest, stream or call), slug and "/"+rest.
func SplitNativePath(p string) (mode, slug, rest string, ok bool) {
	tail, found := strings.CutPrefix(p, "/llm/")
	if !found {
		return "", "", "", false
	}
	mode, tail, found = strings.Cut(tail, "/")
	if !found || (mode != "rest" && mode != "stream" && mode != "call") {
		return "", "", "", false
	}
	slug, tail, found = strings.Cut(tail, "/")
	if !found || slug == "" || tail == "" {
		return "", "", "", false
	}
	return mode, slug, "/" + tail, true
}

// JoinUpstreamPath composes the path sent to the vendor from the path of the
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
func JoinUpstreamPath(upstreamPath, remainingPath string) string {
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

// CanonicalVendorPath is the vendor's own path for a request: the path
// JoinUpstreamPath sends upstream, less any corporate prefix the LLM's
// endpoint carries. A version segment that ends the endpoint (the /v1 of
// https://gw/anthropic/v1) belongs to the vendor's API and is kept; whatever
// precedes it (the /anthropic) is the proxy's mount point and is dropped. So
// every endpoint shape that sends .../v1/messages/batches upstream names the
// same canonical /v1/messages/batches, which native operations are matched on.
func CanonicalVendorPath(upstreamPath, remainingPath string) string {
	joined := JoinUpstreamPath(upstreamPath, remainingPath)
	base := strings.Trim(upstreamPath, "/")
	if base == "" {
		return joined
	}
	segments := strings.Split(base, "/")
	if IsVersionSegment(segments[len(segments)-1]) {
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

// IsVersionSegment reports whether seg names an API version: v1, v2, v1beta,
// v1alpha2 and the like.
func IsVersionSegment(seg string) bool {
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

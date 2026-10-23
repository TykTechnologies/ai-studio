package plugin_sdk

import "github.com/TykTechnologies/midsommar/v2/pkg/requestformat"

// Vendor-aware readers for the model, streaming flag and token usage of an
// LLM exchange. Vendors put these in different places (Bedrock Converse has
// no body "model", streams by path rather than "stream": true, and reports
// usage in camelCase), so a plugin that parses them by hand tends to miss
// some vendors. Pass ctx.Vendor and the path and body the hook received.
// They are plain functions compiled into the plugin, so they work on any
// host version.

// Usage is the token usage a response reports. See ResponseUsage.
type Usage = requestformat.Usage

// RequestModel is the model a request names, "" when it names none: the
// Bedrock /model/{modelId}/ path segment (unescaped, so ARNs and inference
// profile IDs come back whole), the Google AI / Vertex models/{model}:
// segment, or the body's "model" field.
func RequestModel(vendor, path string, body []byte) string {
	return requestformat.Model(vendor, path, body)
}

// IsStreamingRequest reports whether a request asks for a streamed answer:
// Bedrock converse-stream / invoke-with-response-stream, Google
// :streamGenerateContent (or alt=sse), or "stream": true in the body.
func IsStreamingRequest(vendor, path string, body []byte) bool {
	return requestformat.IsStreaming(vendor, path, body)
}

// ResponseUsage reads the token usage from a response body: a REST response
// (OnBeforeWrite) or a whole stream (OnStreamComplete's accumulated
// response), for OpenAI, Anthropic, Google AI, Vertex, Bedrock Converse and
// Ollama. Usage.Reported is false when the body carries no usage.
func ResponseUsage(vendor string, body []byte) Usage {
	return requestformat.ParseUsage(vendor, body)
}

package requestformat

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/url"
	"strings"
)

// Vendor names the readers below treat specially. They match models.Vendor.
const (
	vendorBedrock  = "bedrock"
	vendorGoogleAI = "google_ai"
	vendorVertex   = "vertex"
	vendorOllama   = "ollama"
)

// Model is the model a request names: for Bedrock the {modelId} of its
// /model/{modelId}/{operation} path, for Google AI and Vertex the {model} of
// .../models/{model}:{method}, and otherwise (or when the path names none)
// the body's "model" field. "" when the request names none.
//
// path may be the gateway path (/llm/rest/{slug}/model/...) or the vendor's,
// escaped or not: the AWS SDK escapes the model ID, so an ARN or inference
// profile arrives as arn%3Aaws%3Abedrock%3A...%2F..., and an unescaped ARN
// contains a "/". Both come back unescaped and whole.
func Model(vendor, path string, body []byte) string {
	switch vendor {
	case vendorBedrock:
		if m := bedrockPathModel(path); m != "" {
			return m
		}
	case vendorGoogleAI, vendorVertex:
		if m, _ := googlePathModel(path); m != "" {
			return m
		}
	}
	m, _ := BodyFields(body)
	return m
}

// IsStreaming reports whether a request asks for a streamed answer. Where
// the vendor's own API decides it in the path, the path alone does, and the
// body is not read: Bedrock's /model/{modelId}/{operation} (converse-stream
// and invoke-with-response-stream stream), and Google AI's and Vertex's
// .../models/{model}:{method} (:streamGenerateContent, or alt=sse). Bedrock's
// API has no body "stream" field, nor has Gemini's. Everywhere else, which
// includes those vendors' OpenAI-compatible paths and Vertex's
// :rawPredict / :streamRawPredict (partner models such as Anthropic's, whose
// bodies say "stream"), it is "stream": true in the body (or alt=sse).
func IsStreaming(vendor, path string, body []byte) bool {
	_, s := ModelAndStreamingFunc(vendor, path, func() (string, bool) { return BodyFields(body) })
	return s
}

// ModelAndStreaming is Model and IsStreaming together, reading the body at
// most once: a caller that needs both on every request pays for one JSON
// parse of a body that may be megabytes, not two, and none on a path that
// decides both.
func ModelAndStreaming(vendor, path string, body []byte) (model string, stream bool) {
	return ModelAndStreamingFunc(vendor, path, func() (string, bool) { return BodyFields(body) })
}

// ModelAndStreamingFunc is ModelAndStreaming with the body's fields from
// bodyFields, called at most once and only when the path does not decide
// both: a caller that has already decoded the body passes what it read
// (ObjectFields) rather than parse it again.
func ModelAndStreamingFunc(vendor, path string, bodyFields func() (model string, stream bool)) (model string, stream bool) {
	p, query, _ := strings.Cut(path, "?")
	switch vendor {
	case vendorBedrock:
		if id, op := splitBedrockPath(p); id != "" {
			switch strings.ToLower(op) {
			case "converse-stream", "invoke-with-response-stream":
				stream = true
			}
			return unescape(id), stream
		}
	case vendorGoogleAI, vendorVertex:
		model, method := googlePathModel(p)
		if strings.Contains(strings.ToLower(p), ":streamgeneratecontent") {
			stream = true
		} else if q, err := url.ParseQuery(query); err == nil && q.Get("alt") == "sse" {
			stream = true
		}
		if model != "" && !googleBodyStreams(method) {
			return model, stream
		}
		bodyModel, bodyStream := bodyFields()
		if model == "" {
			model = bodyModel
		}
		return model, stream || bodyStream
	}
	return bodyFields()
}

// googleBodyStreams reports a Vertex method whose body says whether it
// streams: rawPredict and streamRawPredict carry a partner model's own
// request (Anthropic's, Mistral's), which has "stream".
func googleBodyStreams(method string) bool {
	switch strings.ToLower(method) {
	case "rawpredict", "streamrawpredict":
		return true
	}
	return false
}

// BodyFields reads the "model" (a string) and "stream" (a bool) fields of a
// JSON request body; a field of another type, or a body that is not JSON,
// reads as absent. Keys match as encoding/json matches struct fields: an
// exact match, or else one that differs only in case.
func BodyFields(body []byte) (model string, stream bool) {
	var fields struct {
		Model  json.RawMessage `json:"model"`
		Stream json.RawMessage `json:"stream"`
	}
	if json.Unmarshal(body, &fields) != nil {
		return "", false
	}
	_ = json.Unmarshal(fields.Model, &model)
	_ = json.Unmarshal(fields.Stream, &stream)
	return model, stream
}

// ObjectFields is BodyFields of a body already decoded into obj (as
// encoding/json decodes a JSON object into map[string]any), so a caller that
// has decoded it need not parse it again. ok is false when obj cannot say:
// when it has two keys that match "model" (or "stream") but for case,
// encoding/json keeps the later one in the body, an order a map has lost.
// Call BodyFields then.
func ObjectFields(obj map[string]any) (model string, stream bool, ok bool) {
	var modelKeys, streamKeys int
	for k, v := range obj {
		switch {
		case strings.EqualFold(k, "model"):
			modelKeys++
			model, _ = v.(string)
		case strings.EqualFold(k, "stream"):
			streamKeys++
			stream, _ = v.(bool)
		}
	}
	if modelKeys > 1 || streamKeys > 1 {
		return "", false, false
	}
	return model, stream, true
}

func unescape(s string) string {
	if u, err := url.PathUnescape(s); err == nil {
		return u
	}
	return s
}

// bedrockPathModel is the unescaped model ID of a Bedrock runtime path.
func bedrockPathModel(p string) string {
	p, _, _ = strings.Cut(p, "?")
	id, _ := splitBedrockPath(p)
	if id == "" {
		return ""
	}
	return unescape(id)
}

// splitBedrockPath splits .../model/{modelId}/{operation} into the (still
// escaped) model ID and the operation. The operation is the last segment, so
// an unescaped ARN's own "/" stays in the ID.
func splitBedrockPath(p string) (id, op string) {
	if _, _, rest, ok := SplitNativePath(p); ok {
		p = rest
	}
	_, after, found := strings.Cut(p, "/model/")
	if !found {
		return "", ""
	}
	after = strings.TrimSuffix(after, "/")
	i := strings.LastIndex(after, "/")
	if i < 0 {
		return after, ""
	}
	return after[:i], after[i+1:]
}

// googlePathModel is the {model} and {method} of .../models/{model}:{method}
// (method "" when there is none).
func googlePathModel(p string) (model, method string) {
	p, _, _ = strings.Cut(p, "?")
	i := strings.LastIndex(p, "/models/")
	if i < 0 {
		return "", ""
	}
	m := p[i+len("/models/"):]
	if j := strings.IndexAny(m, ":/"); j >= 0 {
		if m[j] == ':' {
			method, _, _ = strings.Cut(m[j+1:], "/")
		}
		m = m[:j]
	}
	return unescape(m), method
}

// Usage is the token usage a response reports.
//
// InputTokens is the vendor's own input count. OpenAI and Google count
// cached tokens inside it (CacheReadTokens is a part of it); Anthropic and
// Bedrock count them apart (CacheReadTokens and CacheWriteTokens come on top).
// TotalTokens is the vendor's total when it reports one, and never less than
// InputTokens + OutputTokens.
type Usage struct {
	InputTokens      int
	OutputTokens     int
	TotalTokens      int
	CacheReadTokens  int
	CacheWriteTokens int
	// Reported is true when the body carried usage at all, so a response
	// that reports none can be told from one that reports zero.
	Reported bool
}

// ParseUsage reads the token usage from a response body: a REST response,
// or a whole streamed one as OnStreamComplete gets it (SSE "data:" lines,
// newline-delimited JSON, or Google's JSON array), for OpenAI (Chat
// Completions and Responses), Anthropic, Google AI, Vertex, Bedrock Converse
// (camelCase usage) and Ollama. For Bedrock it also reads raw eventstream
// frames, the stream chunks response hooks get. In a stream, counts reported
// later replace earlier ones, as vendors report running totals.
func ParseUsage(vendor string, body []byte) Usage {
	var u Usage
	trimmed := bytes.TrimSpace(body)
	switch {
	case len(trimmed) == 0:
	case trimmed[0] == '{' && json.Valid(trimmed):
		u.merge(trimmed)
	case trimmed[0] == '[':
		var events []json.RawMessage
		if json.Unmarshal(trimmed, &events) == nil {
			for _, e := range events {
				u.merge(e)
			}
		}
	case vendor == vendorBedrock && !isTextStream(trimmed):
		for _, payload := range eventstreamPayloads(body) {
			u.merge(payload)
		}
	default:
		for _, line := range bytes.Split(trimmed, []byte("\n")) {
			line = bytes.TrimSpace(line)
			if data, ok := bytes.CutPrefix(line, []byte("data:")); ok {
				line = bytes.TrimSpace(data)
			}
			if len(line) > 0 && line[0] == '{' {
				u.merge(line)
			}
		}
	}
	if sum := u.InputTokens + u.OutputTokens; u.TotalTokens < sum {
		u.TotalTokens = sum
	}
	return u
}

func isTextStream(b []byte) bool {
	for _, prefix := range []string{"{", "data:", "event:", ":", "id:"} {
		if bytes.HasPrefix(b, []byte(prefix)) {
			return true
		}
	}
	return false
}

// eventstreamPayloads splits AWS eventstream frames (total length, headers
// length, prelude CRC, headers, payload, message CRC) into their payloads.
// The CRCs are not checked; a truncated frame ends the walk.
func eventstreamPayloads(b []byte) [][]byte {
	var payloads [][]byte
	for len(b) >= 16 {
		total := int(binary.BigEndian.Uint32(b[0:4]))
		headers := int(binary.BigEndian.Uint32(b[4:8]))
		if total < 16 || total > len(b) || 12+headers > total-4 {
			break
		}
		payloads = append(payloads, b[12+headers:total-4])
		b = b[total:]
	}
	return payloads
}

// count decodes a JSON number, integral or not.
type count float64

// wireUsage holds every vendor's usage keys. Go matches JSON keys without
// regard to case but not to underscores, so the snake_case and camelCase
// spellings land in different fields.
type wireUsage struct {
	// OpenAI Chat Completions.
	PromptTokens        *count `json:"prompt_tokens"`
	CompletionTokens    *count `json:"completion_tokens"`
	TotalTokens         *count `json:"total_tokens"`
	PromptTokensDetails *struct {
		CachedTokens *count `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	// OpenAI Responses and Anthropic.
	InputTokens        *count `json:"input_tokens"`
	OutputTokens       *count `json:"output_tokens"`
	InputTokensDetails *struct {
		CachedTokens *count `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	CacheCreationInputTokens *count `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *count `json:"cache_read_input_tokens"`
	// Bedrock Converse.
	InputTokensCamel      *count `json:"inputTokens"`
	OutputTokensCamel     *count `json:"outputTokens"`
	TotalTokensCamel      *count `json:"totalTokens"`
	CacheReadInputTokensC *count `json:"cacheReadInputTokens"`
	CacheWriteInputTokens *count `json:"cacheWriteInputTokens"`
	// Google usageMetadata.
	PromptTokenCount        *count `json:"promptTokenCount"`
	CandidatesTokenCount    *count `json:"candidatesTokenCount"`
	TotalTokenCount         *count `json:"totalTokenCount"`
	CachedContentTokenCount *count `json:"cachedContentTokenCount"`
}

// merge folds one JSON object's usage into u. It looks where each vendor
// puts usage: "usage" (OpenAI, Anthropic, Bedrock Converse and its stream
// metadata event), "usageMetadata" (Google), "message.usage" (Anthropic
// message_start), "response.usage" (OpenAI Responses response.completed),
// "metadata.usage" (a Bedrock stream event in its union form) and Ollama's
// top-level prompt_eval_count / eval_count. Each part decodes on its own, so
// an unrelated field of an unexpected type ("metadata" on an OpenAI response
// is the caller's own map) costs nothing.
func (u *Usage) merge(obj []byte) {
	var top map[string]json.RawMessage
	if json.Unmarshal(obj, &top) != nil {
		return
	}
	for _, key := range []string{"usage", "usageMetadata"} {
		u.mergeWire(top[key])
	}
	for _, key := range []string{"message", "response", "metadata"} {
		var nested struct {
			Usage json.RawMessage `json:"usage"`
		}
		if raw := top[key]; len(raw) > 0 && raw[0] == '{' && json.Unmarshal(raw, &nested) == nil {
			u.mergeWire(nested.Usage)
		}
	}
	var ollama struct {
		PromptEvalCount *count `json:"prompt_eval_count"`
		EvalCount       *count `json:"eval_count"`
	}
	if json.Unmarshal(obj, &ollama) == nil && (ollama.PromptEvalCount != nil || ollama.EvalCount != nil) {
		u.Reported = true
		set(&u.InputTokens, ollama.PromptEvalCount)
		set(&u.OutputTokens, ollama.EvalCount)
	}
}

func (u *Usage) mergeWire(raw json.RawMessage) {
	if len(raw) == 0 || raw[0] != '{' {
		return // absent, or null as in OpenAI stream chunks before the last
	}
	var w wireUsage
	if json.Unmarshal(raw, &w) != nil {
		return
	}
	u.Reported = true
	set(&u.InputTokens, w.PromptTokens, w.InputTokens, w.InputTokensCamel, w.PromptTokenCount)
	set(&u.OutputTokens, w.CompletionTokens, w.OutputTokens, w.OutputTokensCamel, w.CandidatesTokenCount)
	set(&u.TotalTokens, w.TotalTokens, w.TotalTokensCamel, w.TotalTokenCount)
	set(&u.CacheReadTokens, w.CacheReadInputTokens, w.CacheReadInputTokensC, w.CachedContentTokenCount)
	if w.PromptTokensDetails != nil {
		set(&u.CacheReadTokens, w.PromptTokensDetails.CachedTokens)
	}
	if w.InputTokensDetails != nil {
		set(&u.CacheReadTokens, w.InputTokensDetails.CachedTokens)
	}
	set(&u.CacheWriteTokens, w.CacheCreationInputTokens, w.CacheWriteInputTokens)
}

// set stores the first non-zero count into dst. Zero never overwrites: an
// Anthropic message_delta reports input_tokens 0 after message_start
// reported the real count.
func set(dst *int, counts ...*count) {
	for _, c := range counts {
		if c != nil && *c > 0 {
			*dst = int(*c)
			return
		}
	}
}

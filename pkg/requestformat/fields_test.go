package requestformat

import (
	"encoding/binary"
	"testing"
)

const (
	profileARN        = "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-3-5-sonnet-20240620-v1:0"
	profileARNEscaped = "arn%3Aaws%3Abedrock%3Aus-east-1%3A123456789012%3Ainference-profile%2Fus.anthropic.claude-3-5-sonnet-20240620-v1%3A0"
)

func TestModel(t *testing.T) {
	for _, tc := range []struct {
		name, vendor, path, body, want string
	}{
		{"openai body", "openai", "/llm/rest/gpt/v1/chat/completions", `{"model":"gpt-4o","messages":[]}`, "gpt-4o"},
		{"anthropic body", "anthropic", "/llm/call/claude/v1/messages", `{"model":"claude-sonnet-4-5","max_tokens":10}`, "claude-sonnet-4-5"},
		{"no model", "openai", "/llm/rest/gpt/v1/chat/completions", `{"messages":[]}`, ""},
		{"model not a string", "openai", "/v1/chat/completions", `{"model":42}`, ""},
		{"not json", "openai", "/v1/chat/completions", `nope`, ""},
		{"bedrock converse", "bedrock", "/llm/rest/br/model/anthropic.claude-3-haiku-20240307-v1:0/converse", `{"messages":[]}`, "anthropic.claude-3-haiku-20240307-v1:0"},
		{"bedrock vendor path", "bedrock", "/model/amazon.nova-pro-v1:0/converse-stream", ``, "amazon.nova-pro-v1:0"},
		{"bedrock escaped colon", "bedrock", "/llm/call/br/model/anthropic.claude-3-haiku-20240307-v1%3A0/converse", ``, "anthropic.claude-3-haiku-20240307-v1:0"},
		{"bedrock escaped ARN", "bedrock", "/llm/call/br/model/" + profileARNEscaped + "/converse", ``, profileARN},
		{"bedrock unescaped ARN", "bedrock", "/llm/rest/br/model/" + profileARN + "/converse-stream", ``, profileARN},
		{"bedrock inference profile ID", "bedrock", "/model/us.anthropic.claude-3-5-sonnet-20240620-v1:0/invoke", ``, "us.anthropic.claude-3-5-sonnet-20240620-v1:0"},
		{"bedrock query string", "bedrock", "/model/amazon.nova-lite-v1:0/converse?x=1", ``, "amazon.nova-lite-v1:0"},
		// A slug named "model" is the route's, not Bedrock's.
		{"bedrock slug named model", "bedrock", "/llm/rest/model/model/amazon.nova-pro-v1:0/converse", ``, "amazon.nova-pro-v1:0"},
		// Bedrock's OpenAI-compatible endpoint names the model in the body.
		{"bedrock openai-compatible", "bedrock", "/llm/rest/br/openai/v1/chat/completions", `{"model":"openai.gpt-oss-20b-1:0"}`, "openai.gpt-oss-20b-1:0"},
		{"google ai", "google_ai", "/llm/rest/gem/v1beta/models/gemini-2.5-flash:generateContent", `{"contents":[]}`, "gemini-2.5-flash"},
		{"google ai stream", "google_ai", "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse", ``, "gemini-2.5-pro"},
		{"vertex", "vertex", "/v1/projects/p/locations/us-central1/publishers/google/models/gemini-2.0-flash:generateContent", ``, "gemini-2.0-flash"},
		{"google openai-compatible", "google_ai", "/v1beta/openai/chat/completions", `{"model":"gemini-2.5-flash"}`, "gemini-2.5-flash"},
		// Path rules are per vendor: an OpenAI path's /model/ is not a model.
		{"openai ignores model path", "openai", "/llm/rest/gpt/model/x/converse", `{"model":"gpt-4o"}`, "gpt-4o"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Model(tc.vendor, tc.path, []byte(tc.body)); got != tc.want {
				t.Errorf("Model(%q, %q) = %q, want %q", tc.vendor, tc.path, got, tc.want)
			}
		})
	}
}

func TestIsStreaming(t *testing.T) {
	for _, tc := range []struct {
		name, vendor, path, body string
		want                     bool
	}{
		{"openai stream", "openai", "/v1/chat/completions", `{"model":"gpt-4o","stream":true}`, true},
		{"openai rest", "openai", "/v1/chat/completions", `{"model":"gpt-4o","stream":false}`, false},
		{"openai no field", "openai", "/v1/chat/completions", `{"model":"gpt-4o"}`, false},
		{"stream not a bool", "openai", "/v1/chat/completions", `{"stream":"yes"}`, false},
		{"anthropic stream", "anthropic", "/v1/messages", `{"stream":true}`, true},
		{"ollama stream", "ollama", "/api/chat", `{"stream":true}`, true},
		{"empty body", "openai", "/v1/chat/completions", ``, false},
		{"bedrock converse-stream", "bedrock", "/llm/rest/br/model/amazon.nova-pro-v1:0/converse-stream", `{"messages":[]}`, true},
		{"bedrock converse", "bedrock", "/llm/rest/br/model/amazon.nova-pro-v1:0/converse", `{"messages":[]}`, false},
		{"bedrock invoke stream", "bedrock", "/model/anthropic.claude-v2/invoke-with-response-stream", `{}`, true},
		{"bedrock invoke", "bedrock", "/model/anthropic.claude-v2/invoke", `{}`, false},
		{"bedrock escaped ARN stream", "bedrock", "/llm/call/br/model/" + profileARNEscaped + "/converse-stream", ``, true},
		{"bedrock unescaped ARN stream", "bedrock", "/model/" + profileARN + "/converse-stream", ``, true},
		{"bedrock openai-compatible", "bedrock", "/openai/v1/chat/completions", `{"stream":true}`, true},
		{"google stream", "google_ai", "/v1beta/models/gemini-2.5-flash:streamGenerateContent", `{}`, true},
		{"google alt=sse", "vertex", "/v1/projects/p/locations/l/publishers/google/models/g:generateContent?alt=sse", `{}`, true},
		{"google rest", "google_ai", "/v1beta/models/gemini-2.5-flash:generateContent", `{}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsStreaming(tc.vendor, tc.path, []byte(tc.body)); got != tc.want {
				t.Errorf("IsStreaming(%q, %q, %s) = %v, want %v", tc.vendor, tc.path, tc.body, got, tc.want)
			}
		})
	}
}

// eventstreamFrame encodes one AWS eventstream message with an :event-type
// header (CRCs left zero; ParseUsage does not check them).
func eventstreamFrame(eventType, payload string) []byte {
	var headers []byte
	name := ":event-type"
	headers = append(headers, byte(len(name)))
	headers = append(headers, name...)
	headers = append(headers, 7) // string
	headers = binary.BigEndian.AppendUint16(headers, uint16(len(eventType)))
	headers = append(headers, eventType...)

	total := 12 + len(headers) + len(payload) + 4
	frame := binary.BigEndian.AppendUint32(nil, uint32(total))
	frame = binary.BigEndian.AppendUint32(frame, uint32(len(headers)))
	frame = binary.BigEndian.AppendUint32(frame, 0)
	frame = append(frame, headers...)
	frame = append(frame, payload...)
	return binary.BigEndian.AppendUint32(frame, 0)
}

func TestParseUsage(t *testing.T) {
	var binaryStream []byte
	binaryStream = append(binaryStream, eventstreamFrame("messageStart", `{"role":"assistant"}`)...)
	binaryStream = append(binaryStream, eventstreamFrame("contentBlockDelta", `{"contentBlockIndex":0,"delta":{"text":"Hi"}}`)...)
	binaryStream = append(binaryStream, eventstreamFrame("metadata", `{"usage":{"inputTokens":21,"outputTokens":4,"totalTokens":25,"cacheReadInputTokens":7},"metrics":{"latencyMs":300}}`)...)

	for _, tc := range []struct {
		name, vendor, body string
		want               Usage
	}{
		{"openai chat", "openai",
			`{"id":"c","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":5,"total_tokens":17,"prompt_tokens_details":{"cached_tokens":4}}}`,
			Usage{InputTokens: 12, OutputTokens: 5, TotalTokens: 17, CacheReadTokens: 4, Reported: true}},
		{"openai chat stream", "openai",
			"data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}],\"usage\":null}\n\n" +
				"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":2,\"total_tokens\":11}}\n\n" +
				"data: [DONE]\n\n",
			Usage{InputTokens: 9, OutputTokens: 2, TotalTokens: 11, Reported: true}},
		{"openai stream without usage", "openai",
			"data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\ndata: [DONE]\n\n",
			Usage{}},
		{"openai responses", "openai",
			`{"object":"response","metadata":{"usage":"mine"},"usage":{"input_tokens":30,"output_tokens":8,"total_tokens":38,"input_tokens_details":{"cached_tokens":10}}}`,
			Usage{InputTokens: 30, OutputTokens: 8, TotalTokens: 38, CacheReadTokens: 10, Reported: true}},
		{"openai responses stream", "openai",
			"event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"usage\":null}}\n\n" +
				"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":30,\"output_tokens\":8,\"total_tokens\":38}}}\n\n",
			Usage{InputTokens: 30, OutputTokens: 8, TotalTokens: 38, Reported: true}},
		{"anthropic", "anthropic",
			`{"type":"message","usage":{"input_tokens":20,"output_tokens":6,"cache_creation_input_tokens":100,"cache_read_input_tokens":50}}`,
			Usage{InputTokens: 20, OutputTokens: 6, TotalTokens: 26, CacheReadTokens: 50, CacheWriteTokens: 100, Reported: true}},
		{"anthropic stream", "anthropic",
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"model\":\"claude\",\"usage\":{\"input_tokens\":20,\"output_tokens\":1,\"cache_read_input_tokens\":50}}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"Hi\"}}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":0,\"output_tokens\":15}}\n\n" +
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
			Usage{InputTokens: 20, OutputTokens: 15, TotalTokens: 35, CacheReadTokens: 50, Reported: true}},
		{"google", "google_ai",
			`{"candidates":[],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":18,"cachedContentTokenCount":3}}`,
			Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 18, CacheReadTokens: 3, Reported: true}},
		{"google sse stream", "google_ai",
			"data: {\"candidates\":[],\"usageMetadata\":{\"promptTokenCount\":10,\"candidatesTokenCount\":2,\"totalTokenCount\":12}}\r\n\r\n" +
				"data: {\"candidates\":[],\"usageMetadata\":{\"promptTokenCount\":10,\"candidatesTokenCount\":7,\"totalTokenCount\":17}}\r\n\r\n",
			Usage{InputTokens: 10, OutputTokens: 7, TotalTokens: 17, Reported: true}},
		{"google json array stream", "vertex",
			"[{\"usageMetadata\":{\"promptTokenCount\":4,\"candidatesTokenCount\":1}}\n,{\"usageMetadata\":{\"promptTokenCount\":4,\"candidatesTokenCount\":9,\"totalTokenCount\":13}}]",
			Usage{InputTokens: 4, OutputTokens: 9, TotalTokens: 13, Reported: true}},
		{"bedrock converse", "bedrock",
			`{"output":{"message":{"role":"assistant","content":[{"text":"Hi"}]}},"stopReason":"end_turn","usage":{"inputTokens":21,"outputTokens":4,"totalTokens":25,"cacheReadInputTokens":7,"cacheWriteInputTokens":2},"metrics":{"latencyMs":300}}`,
			Usage{InputTokens: 21, OutputTokens: 4, TotalTokens: 25, CacheReadTokens: 7, CacheWriteTokens: 2, Reported: true}},
		{"bedrock converse-stream data lines", "bedrock",
			"data: {\"role\":\"assistant\"}\n\n" +
				"data: {\"contentBlockIndex\":0,\"delta\":{\"text\":\"Hi\"}}\n\n" +
				"data: {\"stopReason\":\"end_turn\"}\n\n" +
				"data: {\"usage\":{\"inputTokens\":21,\"outputTokens\":4,\"totalTokens\":25},\"metrics\":{\"latencyMs\":300}}\n\n",
			Usage{InputTokens: 21, OutputTokens: 4, TotalTokens: 25, Reported: true}},
		{"bedrock stream union form", "bedrock",
			"data: {\"metadata\":{\"usage\":{\"inputTokens\":3,\"outputTokens\":2,\"totalTokens\":5,\"cacheWriteInputTokens\":9}}}\n",
			Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5, CacheWriteTokens: 9, Reported: true}},
		{"bedrock eventstream frames", "bedrock", string(binaryStream),
			Usage{InputTokens: 21, OutputTokens: 4, TotalTokens: 25, CacheReadTokens: 7, Reported: true}},
		{"bedrock truncated frame", "bedrock", string(binaryStream[:len(binaryStream)-5]),
			Usage{}},
		{"ollama native", "ollama",
			`{"model":"llama3","done":true,"prompt_eval_count":26,"eval_count":290}`,
			Usage{InputTokens: 26, OutputTokens: 290, TotalTokens: 316, Reported: true}},
		{"ollama ndjson stream", "ollama",
			"{\"message\":{\"content\":\"Hi\"},\"done\":false}\n{\"done\":true,\"prompt_eval_count\":26,\"eval_count\":3}\n",
			Usage{InputTokens: 26, OutputTokens: 3, TotalTokens: 29, Reported: true}},
		{"float counts", "openai", `{"usage":{"prompt_tokens":3.0,"completion_tokens":1}}`,
			Usage{InputTokens: 3, OutputTokens: 1, TotalTokens: 4, Reported: true}},
		{"error body", "openai", `{"error":{"message":"bad","type":"invalid_request_error"}}`, Usage{}},
		{"empty", "bedrock", "", Usage{}},
		{"garbage", "bedrock", "\x00\x00", Usage{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseUsage(tc.vendor, []byte(tc.body)); got != tc.want {
				t.Errorf("ParseUsage = %+v, want %+v", got, tc.want)
			}
		})
	}
}

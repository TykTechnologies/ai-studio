package extract

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamText(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		ok               bool
	}{
		{"openai chat", "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"It is \"}}]}\n\n" +
			"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"sunny.\"}}]}\n\n" +
			"data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", "It is sunny.", true},
		{"openai chat, tool calls only", "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":null,\"tool_calls\":[{\"index\":0,\"id\":\"c\",\"function\":{\"name\":\"f\",\"arguments\":\"{}\"}}]}}]}\n\ndata: [DONE]\n\n", "", true},
		{"responses", "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hel\"}\n\n" +
			"event: response.function_call_arguments.delta\ndata: {\"type\":\"response.function_call_arguments.delta\",\"delta\":\"{\\\"x\\\"\"}\n\n" +
			"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"lo\"}\n\n", "Hello", true},
		{"anthropic, thinking left out", "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"hmm\"}}\n\n" +
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"text_delta\",\"text\":\"Done.\"}}\n\n" +
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":2,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\"}}\n\n", "Done.", true},
		{"gemini", "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"A\"},{\"text\":\"B\"}]}}]}\n\ndata: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"C\"}]}}]}\n\n", "ABC", true},
		{"ollama ndjson", "{\"message\":{\"role\":\"assistant\",\"content\":\"Hi \"},\"done\":false}\n{\"message\":{\"role\":\"assistant\",\"content\":\"there\"},\"done\":true}\n", "Hi there", true},
		{"a frame cut by the capture limit", "data: {\"choices\":[{\"delta\":{\"content\":\"kept\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"con", "kept", true},
		{"one JSON document is not a stream", `{"choices":[{"message":{"content":"x"}}]}`, "", false},
		{"plain text", "an error page", "", false},
		{"empty", "  ", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := StreamText([]byte(tc.body))
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

// The recorded streams: text is assembled, tool-call argument deltas never
// leak into it.
func TestStreamText_Fixtures(t *testing.T) {
	read := func(name string) []byte {
		b, err := os.ReadFile("../../../testdata/agent_tracing/streams/" + name)
		require.NoError(t, err)
		return b
	}
	text, ok := StreamText(read("anthropic_messages_stream_parallel.sse"))
	assert.True(t, ok)
	assert.Contains(t, text, "Checking both cities.")
	assert.NotContains(t, text, "{")

	for _, f := range []string{"openai_chat_parallel_stream.sse", "openai_responses_stream.sse", "stream_abort.sse"} {
		text, ok := StreamText(read(f))
		assert.True(t, ok, f)
		assert.False(t, strings.ContainsAny(text, "{}"), "%s: no argument deltas in %q", f, text)
	}
}

// Linear in the body: a 4 MiB stream of small deltas.
func BenchmarkStreamText(b *testing.B) {
	frame := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"word \"}}]}\n\n"
	body := []byte(strings.Repeat(frame, (4<<20)/len(frame)))
	b.SetBytes(int64(len(body)))
	for i := 0; i < b.N; i++ {
		StreamText(body)
	}
}

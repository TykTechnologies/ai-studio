package proxy

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace/extract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var captureFP = agenttrace.StaticFingerprinter{ID: 1, Key: []byte("stream-capture-test")}

func streamFixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "agent_tracing", "streams", name))
	require.NoError(t, err)
	return b
}

// capture feeds body in chunks of size n (0: whole).
func capture(dialect streamDialect, contentType string, body []byte, n, maxArgs int) streamCaptureResult {
	c := newStreamCapture(contentType, dialect, maxArgs, captureFP)
	if n <= 0 {
		c.Feed(body)
	} else {
		for i := 0; i < len(body); i += n {
			end := i + n
			if end > len(body) {
				end = len(body)
			}
			c.Feed(body[i:end])
		}
	}
	return c.Finish()
}

// TestStreamCaptureMatchesNonStreamed: a streamed response yields the tool
// calls, ids, names and argument fingerprints, response id, finish reasons
// and server tools that extract reads from the same response unstreamed,
// whatever the read sizes.
func TestStreamCaptureMatchesNonStreamed(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		dialect streamDialect
		format  string
		calls   int
		servers int
	}{
		{"openai_chat_parallel_stream", "openai", extract.FormatOpenAI, 3, 0},
		{"anthropic_messages_stream_parallel", "anthropic", extract.FormatAnthropic, 2, 1},
		{"openai_responses_stream", "openai_responses", extract.FormatOpenAIResponses, 2, 1},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			want, err := extract.ParseResponse(tc.format, streamFixture(t, tc.fixture+".json"), captureFP)
			require.NoError(t, err)
			require.Len(t, want.ToolCalls, tc.calls)
			require.Len(t, want.ServerToolCalls, tc.servers)
			var wantX agenttrace.Extracts
			want.Apply(&wantX)

			stream := streamFixture(t, tc.fixture+".sse")
			for _, n := range []int{0, 1, 3, 7, 64, 1024} {
				got := capture(tc.dialect, "text/event-stream", stream, n, 0)
				x := got.Extracts
				assert.True(t, x.Stream)
				assert.False(t, x.StreamIncomplete, "read size %d", n)
				assert.Equal(t, wantX.ResponseID, x.ResponseID, "read size %d", n)
				assert.Equal(t, wantX.FinishReasons, x.FinishReasons, "read size %d", n)
				require.Len(t, x.ToolCalls, len(wantX.ToolCalls), "read size %d", n)
				for i, w := range wantX.ToolCalls {
					g := x.ToolCalls[i]
					assert.Equal(t, w.ID, g.ID, "read size %d call %d", n, i)
					assert.Equal(t, w.Name, g.Name, "read size %d call %d", n, i)
					require.NotNil(t, g.ArgsFP, "read size %d call %d has no args fingerprint", n, i)
					assert.Equal(t, w.ArgsFP, g.ArgsFP, "read size %d call %d args differ", n, i)
					assert.False(t, g.ArgsUnavailable)
				}
				require.Len(t, got.ServerToolCalls, len(want.ServerToolCalls))
				for i, w := range want.ServerToolCalls {
					assert.Equal(t, w.ID, got.ServerToolCalls[i].ID)
					assert.Equal(t, w.Name, got.ServerToolCalls[i].Name)
				}
				assert.False(t, got.ArgsOverflow)
			}
		})
	}
}

func TestStreamCaptureAbort(t *testing.T) {
	for _, n := range []int{0, 1, 5} {
		got := capture("openai", "text/event-stream", streamFixture(t, "stream_abort.sse"), n, 0)
		x := got.Extracts
		assert.True(t, x.StreamIncomplete, "read size %d", n)
		assert.Equal(t, []string{"error"}, x.FinishReasons)
		assert.Equal(t, "chatcmpl-abort1", x.ResponseID)
		require.Len(t, x.ToolCalls, 2)
		// The call that arrived whole before the cut keeps its fingerprint
		// only if it was closed; in Chat Completions calls close at the
		// finish reason, which never came, so both are open at the cut.
		for i, id := range []string{"call_done", "call_cut"} {
			assert.Equal(t, id, x.ToolCalls[i].ID)
			assert.True(t, x.ToolCalls[i].ArgsUnavailable, "%s", id)
			assert.Nil(t, x.ToolCalls[i].ArgsFP)
		}
		assert.Equal(t, "write_file", x.ToolCalls[1].Name)
	}
}

func TestStreamCaptureArgumentCap(t *testing.T) {
	stream := streamFixture(t, "openai_chat_parallel_stream.sse")
	// The first call's arguments fit in 40 bytes; the second goes over.
	got := capture("openai", "text/event-stream", stream, 0, 40)
	x := got.Extracts
	require.Len(t, x.ToolCalls, 3)
	assert.True(t, got.ArgsOverflow)
	assert.False(t, x.ToolCalls[0].ArgsUnavailable)
	assert.NotNil(t, x.ToolCalls[0].ArgsFP)
	for _, c := range x.ToolCalls[1:] {
		assert.True(t, c.ArgsUnavailable, c.ID)
		assert.Nil(t, c.ArgsFP)
		assert.NotEmpty(t, c.ID)
		assert.NotEmpty(t, c.Name)
	}
	assert.False(t, x.StreamIncomplete, "an overflow is not an abort")
}

func TestStreamCaptureOtherDialects(t *testing.T) {
	t.Run("ollama ndjson", func(t *testing.T) {
		body := []byte(`{"model":"llama3","message":{"role":"assistant","content":""},"done":false}
{"model":"llama3","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"get_time","arguments":{"tz":"UTC"}}}]},"done":false}
{"model":"llama3","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}
`)
		want, err := extract.ParseResponse(extract.FormatOllama, []byte(`{"message":{"tool_calls":[{"function":{"name":"get_time","arguments":{"tz":"UTC"}}}]},"done_reason":"stop"}`), captureFP)
		require.NoError(t, err)
		got := capture("ollama", "application/x-ndjson", body, 1, 0)
		require.Len(t, got.Extracts.ToolCalls, 1)
		assert.Equal(t, want.ToolCalls[0].ArgsFP, got.Extracts.ToolCalls[0].ArgsFP)
		assert.Equal(t, []string{"stop"}, got.Extracts.FinishReasons)
		assert.False(t, got.Extracts.StreamIncomplete)
	})
	t.Run("google sse", func(t *testing.T) {
		body := []byte("data: {\"responseId\":\"g-1\",\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hi\"}]}}]}\r\n\r\n" +
			"data: {\"responseId\":\"g-1\",\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"lookup\",\"args\":{\"k\":1}}}]},\"finishReason\":\"STOP\"}]}\r\n\r\n")
		want, err := extract.ParseResponse(extract.FormatGoogleAI, []byte(`{"responseId":"g-1","candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"k":1}}}]},"finishReason":"STOP"}]}`), captureFP)
		require.NoError(t, err)
		got := capture("google_ai", "text/event-stream", body, 2, 0)
		require.Len(t, got.Extracts.ToolCalls, 1)
		assert.Equal(t, want.ToolCalls[0].ArgsFP, got.Extracts.ToolCalls[0].ArgsFP)
		assert.Equal(t, "g-1", got.Extracts.ResponseID)
		assert.Equal(t, []string{"STOP"}, got.Extracts.FinishReasons)
		assert.False(t, got.Extracts.StreamIncomplete)
	})
	t.Run("responses events under the openai dialect", func(t *testing.T) {
		got := capture("openai", "text/event-stream", streamFixture(t, "openai_responses_stream.sse"), 0, 0)
		assert.Len(t, got.Extracts.ToolCalls, 2)
		assert.Equal(t, "resp_01RS", got.Extracts.ResponseID)
	})
	t.Run("no fingerprinter", func(t *testing.T) {
		c := newStreamCapture("text/event-stream", "openai", 0, nil)
		c.Feed(streamFixture(t, "openai_chat_parallel_stream.sse"))
		x := c.Finish().Extracts
		require.Len(t, x.ToolCalls, 3)
		assert.Equal(t, "call_search", x.ToolCalls[0].ID)
		assert.Nil(t, x.ToolCalls[0].ArgsFP)
		assert.False(t, x.ToolCalls[0].ArgsUnavailable)
	})
	t.Run("garbage", func(t *testing.T) {
		got := capture("anthropic", "text/event-stream", []byte("data: {not json\n\ndata: [1,2]\n\nevent: x\n"), 1, 0)
		assert.Empty(t, got.Extracts.ToolCalls)
		assert.True(t, got.Extracts.StreamIncomplete)
	})
}

// TestStreamCaptureBounds: a stream is bounded like a buffered body (jyx.62).
func TestStreamCaptureBounds(t *testing.T) {
	var b strings.Builder
	longID := strings.Repeat("i", extract.MaxIDBytes+1)
	longName := strings.Repeat("n", extract.MaxNameBytes+10)
	b.WriteString(`data: {"id":"` + longID + `","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"` + longID + `","function":{"name":"` + longName + `","arguments":"{}"}}]}}]}` + "\n\n")
	for i := 1; i <= extract.MaxToolCalls+5; i++ {
		b.WriteString(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":` + strconv.Itoa(i) + `,"id":"call_` + strconv.Itoa(i) + `","function":{"name":"t","arguments":"{}"}}]}}]}` + "\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	got := capture("openai", "text/event-stream", []byte(b.String()), 0, 0)
	x := got.Extracts
	assert.Empty(t, x.ResponseID, "an over-long response id is dropped")
	require.Len(t, x.ToolCalls, extract.MaxToolCalls)
	assert.Empty(t, x.ToolCalls[0].ID, "an over-long call id is dropped, not cut")
	assert.Len(t, x.ToolCalls[0].Name, extract.MaxNameBytes)
	assert.Equal(t, "call_1", x.ToolCalls[1].ID)
}

// FuzzStreamCaptureSplits: any split of a stream into reads gives the same
// capture as reading it whole.
func FuzzStreamCaptureSplits(f *testing.F) {
	fixtures := []struct {
		name    string
		dialect streamDialect
	}{
		{"openai_chat_parallel_stream.sse", "openai"},
		{"anthropic_messages_stream_parallel.sse", "anthropic"},
		{"openai_responses_stream.sse", "openai_responses"},
		{"stream_abort.sse", "openai"},
	}
	bodies := make([][]byte, len(fixtures))
	for i, fx := range fixtures {
		bodies[i] = streamFixture(f, fx.name)
	}
	f.Add(uint8(0), []byte{1, 2, 3})
	f.Add(uint8(1), []byte{200, 1, 1, 50})
	f.Add(uint8(2), []byte{7})
	f.Fuzz(func(t *testing.T, which uint8, cuts []byte) {
		i := int(which) % len(fixtures)
		body := bodies[i]
		whole := capture(fixtures[i].dialect, "text/event-stream", body, 0, 0)

		c := newStreamCapture("text/event-stream", fixtures[i].dialect, 0, captureFP)
		pos := 0
		for _, cut := range cuts {
			end := pos + int(cut)
			if end > len(body) {
				end = len(body)
			}
			c.Feed(body[pos:end])
			pos = end
		}
		c.Feed(body[pos:])
		split := c.Finish()
		if !sameCapture(whole, split) {
			t.Fatalf("split %v changed the capture:\n%+v\n%+v", cuts, whole, split)
		}
	})
}

func sameCapture(a, b streamCaptureResult) bool {
	if a.Extracts.ResponseID != b.Extracts.ResponseID || a.Extracts.StreamIncomplete != b.Extracts.StreamIncomplete ||
		strings.Join(a.Extracts.FinishReasons, ",") != strings.Join(b.Extracts.FinishReasons, ",") ||
		len(a.Extracts.ToolCalls) != len(b.Extracts.ToolCalls) || len(a.ServerToolCalls) != len(b.ServerToolCalls) {
		return false
	}
	for i := range a.Extracts.ToolCalls {
		x, y := a.Extracts.ToolCalls[i], b.Extracts.ToolCalls[i]
		if x.ID != y.ID || x.Name != y.Name || !bytes.Equal(x.ArgsFP, y.ArgsFP) || x.ArgsUnavailable != y.ArgsUnavailable {
			return false
		}
	}
	return true
}

// textHeavyStream is a realistic agent response: 1,500 text deltas, then
// the parallel tool calls.
func textHeavyStream(t testing.TB) []byte {
	var b bytes.Buffer
	for i := 0; i < 1500; i++ {
		b.WriteString(`data: {"id":"chatcmpl-par1","object":"chat.completion.chunk","created":1792491768,"model":"gpt-4o-2024-08-06","choices":[{"index":0,"delta":{"content":" token"},"logprobs":null,"finish_reason":null}]}` + "\n\n")
	}
	b.Write(streamFixture(t, "openai_chat_parallel_stream.sse"))
	return b.Bytes()
}

func TestStreamCaptureTextHeavyMatches(t *testing.T) {
	got := capture("openai", "text/event-stream", textHeavyStream(t), 0, 0)
	want := capture("openai", "text/event-stream", streamFixture(t, "openai_chat_parallel_stream.sse"), 0, 0)
	assert.True(t, sameCapture(want, got))
}

// BenchmarkStreamCapture is the off-path cost of capturing a streamed
// response (run by the analytics goroutine, after the client has it).
func BenchmarkStreamCapture(b *testing.B) {
	for _, bc := range []struct {
		name string
		body []byte
	}{
		{"tool_calls_only", streamFixture(b, "openai_chat_parallel_stream.sse")},
		{"text_heavy", textHeavyStream(b)},
		{"anthropic", streamFixture(b, "anthropic_messages_stream_parallel.sse")},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(bc.body)))
			for i := 0; i < b.N; i++ {
				c := newStreamCapture("text/event-stream", "openai", 0, captureFP)
				c.Feed(bc.body)
				c.Finish()
			}
		})
	}
}

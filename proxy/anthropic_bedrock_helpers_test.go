package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// makeConverseOutput builds a Converse response for response-mapping tests.
func makeConverseOutput(blocks []types.ContentBlock, stop types.StopReason, in, out, cacheWrite, cacheRead int32) *bedrockruntime.ConverseOutput {
	return &bedrockruntime.ConverseOutput{
		Output: &types.ConverseOutputMemberMessage{
			Value: types.Message{Role: types.ConversationRoleAssistant, Content: blocks},
		},
		StopReason: stop,
		Usage: &types.TokenUsage{
			InputTokens:           aws.Int32(in),
			OutputTokens:          aws.Int32(out),
			CacheWriteInputTokens: aws.Int32(cacheWrite),
			CacheReadInputTokens:  aws.Int32(cacheRead),
		},
	}
}

func lazyDoc(m map[string]any) document.Interface {
	return document.NewLazyDocument(m)
}

// sseRecorder is an http.ResponseWriter + Flusher that captures SSE output so tests
// can assert the exact event sequence and payloads.
type sseRecorder struct {
	header http.Header
	buf    bytes.Buffer
	status int
}

func newSSERecorder() *sseRecorder {
	return &sseRecorder{header: http.Header{}}
}

func (r *sseRecorder) Header() http.Header         { return r.header }
func (r *sseRecorder) Write(b []byte) (int, error) { return r.buf.Write(b) }
func (r *sseRecorder) WriteHeader(status int)      { r.status = status }
func (r *sseRecorder) Flush()                      {}

type sseFrame struct {
	event string
	data  map[string]any
}

// frames parses the recorded "event: X\ndata: {json}\n\n" stream.
func (r *sseRecorder) frames() []sseFrame {
	var frames []sseFrame
	for _, chunk := range strings.Split(r.buf.String(), "\n\n") {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		var f sseFrame
		for _, line := range strings.Split(chunk, "\n") {
			if strings.HasPrefix(line, "event: ") {
				f.event = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &f.data)
			}
		}
		frames = append(frames, f)
	}
	return frames
}

func (r *sseRecorder) eventTypes() []string {
	var out []string
	for _, f := range r.frames() {
		out = append(out, f.event)
	}
	return out
}

func (r *sseRecorder) dataFor(event string) []map[string]any {
	var out []map[string]any
	for _, f := range r.frames() {
		if f.event == event {
			out = append(out, f.data)
		}
	}
	return out
}

package agent_session

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// fakeStream replays scripted chunks then EOF.
type fakeStream struct {
	grpc.ClientStream
	chunks []*pb.AgentMessageChunk
	i      int
}

func (f *fakeStream) Recv() (*pb.AgentMessageChunk, error) {
	if f.i >= len(f.chunks) {
		return nil, io.EOF
	}
	c := f.chunks[f.i]
	f.i++
	return c, nil
}

// memQueue is the smallest MessageQueue that records what was published.
type memQueue struct {
	stream chan []byte
	errs   chan error
}

func (q *memQueue) PublishStream(ctx context.Context, data []byte) error {
	q.stream <- data
	return nil
}
func (q *memQueue) PublishError(ctx context.Context, err error) error { q.errs <- err; return nil }
func (q *memQueue) ConsumeStream(ctx context.Context) <-chan []byte   { return q.stream }
func (q *memQueue) ConsumeErrors(ctx context.Context) <-chan error    { return q.errs }
func (q *memQueue) Close() error                                      { return nil }

func TestRecordChunk_BuildsTranscriptAndTagsToolCalls(t *testing.T) {
	q := &memQueue{stream: make(chan []byte, 32), errs: make(chan error, 4)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	as := &AgentSession{id: "s", queue: q, ctx: ctx, cancel: cancel, agentConfig: &models.AgentConfig{}}

	as.mu.Lock()
	as.nextID++
	as.current = &TranscriptMessage{ID: "a1", Role: "assistant"}
	as.mu.Unlock()

	stream := &fakeStream{chunks: []*pb.AgentMessageChunk{
		{Type: pb.AgentMessageChunk_THINKING, Content: "planning"},
		{Type: pb.AgentMessageChunk_CONTENT, Content: "Let me "},
		{Type: pb.AgentMessageChunk_CONTENT, Content: "check."},
		{Type: pb.AgentMessageChunk_TOOL_CALL, MetadataJson: `{"tool_name":"getWeather","parameters":{"city":"Auckland"}}`},
		{Type: pb.AgentMessageChunk_TOOL_RESULT, Content: `{"temp":18}`, MetadataJson: `{"tool_name":"getWeather"}`},
		{Type: pb.AgentMessageChunk_CONTENT, Content: "18 degrees.", IsFinal: true},
	}}
	require.NoError(t, as.receiveChunks(stream))
	as.finishRun()

	tr := as.Transcript()
	require.Len(t, tr, 1)
	parts := tr[0].Parts
	require.Len(t, parts, 4)
	assert.Equal(t, "reasoning", parts[0].Type)
	assert.Equal(t, "Let me check.", parts[1].Text)
	assert.Equal(t, "tool-call", parts[2].Type)
	assert.Equal(t, "getWeather", parts[2].ToolName)
	assert.Equal(t, map[string]any{"city": "Auckland"}, parts[2].Args)
	assert.True(t, parts[2].HasResult)
	assert.Equal(t, map[string]any{"temp": float64(18)}, parts[2].Result)
	assert.Equal(t, "18 degrees.", parts[3].Text, "text after a tool call starts a new text part")

	// Published chunks carry the tool call id so consumers can pair them.
	close(q.stream)
	var ids []string
	for b := range q.stream {
		var c AgentMessageChunk
		require.NoError(t, json.Unmarshal(b, &c))
		if c.Type == ChunkToolCall || c.Type == ChunkToolResult {
			ids = append(ids, c.Metadata[MetadataToolCallID].(string))
		}
	}
	require.Len(t, ids, 2)
	assert.Equal(t, ids[0], ids[1])
	assert.Equal(t, parts[2].ToolCallID, ids[0])
}

func TestHistoryForPlugin_FlattensTextOnly(t *testing.T) {
	as := &AgentSession{}
	as.transcript = []TranscriptMessage{
		{Role: "user", Parts: []TranscriptPart{{Type: "text", Text: "hi"}}},
		{Role: "assistant", Parts: []TranscriptPart{{Type: "reasoning", Text: "x"}, {Type: "tool-call", ToolName: "t"}, {Type: "text", Text: "hello"}}},
		{Role: "assistant", Parts: []TranscriptPart{{Type: "tool-call", ToolName: "t"}}},
	}
	h := as.historyForPlugin()
	require.Len(t, h, 2)
	assert.Equal(t, "hi", h[0]["content"])
	assert.Equal(t, "hello", h[1]["content"])
}

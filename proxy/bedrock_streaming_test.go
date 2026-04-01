package proxy

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalEventForBinaryStream(t *testing.T) {
	t.Run("messageStart event", func(t *testing.T) {
		event := &types.ConverseStreamOutputMemberMessageStart{
			Value: types.MessageStartEvent{Role: types.ConversationRoleAssistant},
		}
		eventType, data, err := marshalEventForBinaryStream(event)
		require.NoError(t, err)
		assert.Equal(t, "messageStart", eventType)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "assistant", parsed["role"])
	})

	t.Run("contentBlockDelta text event", func(t *testing.T) {
		event := &types.ConverseStreamOutputMemberContentBlockDelta{
			Value: types.ContentBlockDeltaEvent{
				ContentBlockIndex: aws.Int32(0),
				Delta:             &types.ContentBlockDeltaMemberText{Value: "Hello"},
			},
		}
		eventType, data, err := marshalEventForBinaryStream(event)
		require.NoError(t, err)
		assert.Equal(t, "contentBlockDelta", eventType)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, float64(0), parsed["contentBlockIndex"])
	})

	t.Run("contentBlockStop event", func(t *testing.T) {
		event := &types.ConverseStreamOutputMemberContentBlockStop{
			Value: types.ContentBlockStopEvent{ContentBlockIndex: aws.Int32(0)},
		}
		eventType, _, err := marshalEventForBinaryStream(event)
		require.NoError(t, err)
		assert.Equal(t, "contentBlockStop", eventType)
	})

	t.Run("messageStop event", func(t *testing.T) {
		event := &types.ConverseStreamOutputMemberMessageStop{
			Value: types.MessageStopEvent{StopReason: types.StopReasonEndTurn},
		}
		eventType, data, err := marshalEventForBinaryStream(event)
		require.NoError(t, err)
		assert.Equal(t, "messageStop", eventType)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "end_turn", parsed["stopReason"])
	})

	t.Run("metadata event with usage", func(t *testing.T) {
		event := &types.ConverseStreamOutputMemberMetadata{
			Value: types.ConverseStreamMetadataEvent{
				Usage: &types.TokenUsage{
					InputTokens:  aws.Int32(10),
					OutputTokens: aws.Int32(5),
					TotalTokens:  aws.Int32(15),
				},
				Metrics: &types.ConverseStreamMetrics{
					LatencyMs: aws.Int64(200),
				},
			},
		}
		eventType, data, err := marshalEventForBinaryStream(event)
		require.NoError(t, err)
		assert.Equal(t, "metadata", eventType)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))
		usage := parsed["usage"].(map[string]interface{})
		assert.Equal(t, float64(10), usage["inputTokens"])
		assert.Equal(t, float64(5), usage["outputTokens"])
		assert.Equal(t, float64(15), usage["totalTokens"])
		metrics := parsed["metrics"].(map[string]interface{})
		assert.Equal(t, float64(200), metrics["latencyMs"])
	})

	t.Run("contentBlockStart event", func(t *testing.T) {
		event := &types.ConverseStreamOutputMemberContentBlockStart{
			Value: types.ContentBlockStartEvent{ContentBlockIndex: aws.Int32(0)},
		}
		eventType, _, err := marshalEventForBinaryStream(event)
		require.NoError(t, err)
		assert.Equal(t, "contentBlockStart", eventType)
	})
}

func TestEncodeBedrockEventStreamMessage(t *testing.T) {
	t.Run("encodes a valid binary event stream message", func(t *testing.T) {
		event := &types.ConverseStreamOutputMemberContentBlockDelta{
			Value: types.ContentBlockDeltaEvent{
				ContentBlockIndex: aws.Int32(0),
				Delta:             &types.ContentBlockDeltaMemberText{Value: "Hello"},
			},
		}

		var buf bytes.Buffer
		encoder := eventstream.NewEncoder()
		err := encodeBedrockEventStreamMessage(encoder, &buf, event)
		require.NoError(t, err)
		assert.Greater(t, buf.Len(), 0)
	})
}

func TestExtractTextFromStreamEvent(t *testing.T) {
	t.Run("extracts text from content block delta", func(t *testing.T) {
		event := &types.ConverseStreamOutputMemberContentBlockDelta{
			Value: types.ContentBlockDeltaEvent{
				Delta: &types.ContentBlockDeltaMemberText{Value: "Hello world"},
			},
		}
		text := extractTextFromStreamEvent(event)
		assert.Equal(t, "Hello world", text)
	})

	t.Run("returns empty for non-text events", func(t *testing.T) {
		event := &types.ConverseStreamOutputMemberMessageStart{
			Value: types.MessageStartEvent{Role: types.ConversationRoleAssistant},
		}
		text := extractTextFromStreamEvent(event)
		assert.Equal(t, "", text)
	})

	t.Run("returns empty for metadata events", func(t *testing.T) {
		event := &types.ConverseStreamOutputMemberMetadata{
			Value: types.ConverseStreamMetadataEvent{
				Usage: &types.TokenUsage{
					InputTokens:  aws.Int32(10),
					OutputTokens: aws.Int32(5),
					TotalTokens:  aws.Int32(15),
				},
			},
		}
		text := extractTextFromStreamEvent(event)
		assert.Equal(t, "", text)
	})
}

func TestBuildConverseStreamInput(t *testing.T) {
	t.Run("builds input with messages and system blocks", func(t *testing.T) {
		req := &bedrockConverseStreamRequest{
			Messages: []bedrockMessage{
				{
					Role:    "user",
					Content: []bedrockContentBlock{{Text: "Hello!"}},
				},
			},
			System: []bedrockSystemBlock{
				{Text: "You are a helpful assistant."},
			},
			InferenceConfig: &bedrockInferenceConfig{
				MaxTokens:   aws.Int32(1024),
				Temperature: aws.Float32(0.7),
			},
		}

		input := buildConverseStreamInput("anthropic.claude-3-sonnet", req)
		assert.Equal(t, "anthropic.claude-3-sonnet", *input.ModelId)
		require.Len(t, input.Messages, 1)
		require.Len(t, input.System, 1)
		require.NotNil(t, input.InferenceConfig)
		assert.Equal(t, int32(1024), *input.InferenceConfig.MaxTokens)
	})

	t.Run("handles empty request", func(t *testing.T) {
		req := &bedrockConverseStreamRequest{}
		input := buildConverseStreamInput("test-model", req)
		assert.Equal(t, "test-model", *input.ModelId)
		assert.Nil(t, input.Messages)
		assert.Nil(t, input.InferenceConfig)
	})
}

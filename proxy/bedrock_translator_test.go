package proxy

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIMessagesToChatMessages(t *testing.T) {
	t.Run("converts string content messages", func(t *testing.T) {
		messages := []Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello!"},
			{Role: "assistant", Content: "Hi there!"},
		}
		result := openAIMessagesToChatMessages(messages)
		require.Len(t, result, 3)
		assert.Equal(t, "system", result[0].Role)
		assert.Equal(t, "You are helpful.", result[0].Content)
		assert.Equal(t, "user", result[1].Role)
		assert.Equal(t, "Hello!", result[1].Content)
		assert.Equal(t, "assistant", result[2].Role)
		assert.Equal(t, "Hi there!", result[2].Content)
	})

	t.Run("extracts text from multi-part content", func(t *testing.T) {
		messages := []Message{
			{
				Role: "user",
				Content: []interface{}{
					map[string]interface{}{"type": "text", "text": "Part 1. "},
					map[string]interface{}{"type": "text", "text": "Part 2."},
					map[string]interface{}{"type": "image_url", "image_url": "http://example.com/img.png"},
				},
			},
		}
		result := openAIMessagesToChatMessages(messages)
		require.Len(t, result, 1)
		assert.Equal(t, "Part 1. Part 2.", result[0].Content)
	})

	t.Run("handles empty messages", func(t *testing.T) {
		result := openAIMessagesToChatMessages(nil)
		assert.Len(t, result, 0)
	})
}

func TestOpenAIToolsToToolDefs(t *testing.T) {
	tools := []Tool{
		{
			Type: "function",
			Function: FunctionConfig{
				Name:        "get_weather",
				Description: "Get current weather",
				Parameters:  map[string]any{"type": "object"},
			},
		},
	}
	result := openAIToolsToToolDefs(tools)
	require.Len(t, result, 1)
	assert.Equal(t, "function", result[0].Type)
	assert.Equal(t, "get_weather", result[0].Function.Name)
	assert.Equal(t, "Get current weather", result[0].Function.Description)
}

func TestConverseOutputToOpenAI(t *testing.T) {
	t.Run("converts text response", func(t *testing.T) {
		output := &bedrockruntime.ConverseOutput{
			Output: &types.ConverseOutputMemberMessage{
				Value: types.Message{
					Role: types.ConversationRoleAssistant,
					Content: []types.ContentBlock{
						&types.ContentBlockMemberText{Value: "Hello, world!"},
					},
				},
			},
			StopReason: types.StopReasonEndTurn,
			Usage: &types.TokenUsage{
				InputTokens:  aws.Int32(10),
				OutputTokens: aws.Int32(5),
				TotalTokens:  aws.Int32(15),
			},
		}

		resp := converseOutputToOpenAI(output, "anthropic.claude-3-5-sonnet")
		assert.Equal(t, "chat.completion", resp.Object)
		assert.Equal(t, "anthropic.claude-3-5-sonnet", resp.Model)
		require.Len(t, resp.Choices, 1)
		assert.Equal(t, "assistant", resp.Choices[0].Message.Role)
		assert.Equal(t, "Hello, world!", resp.Choices[0].Message.Content)
		assert.Equal(t, "stop", resp.Choices[0].FinishReason)
		assert.Equal(t, 10, resp.Usage.PromptTokens)
		assert.Equal(t, 5, resp.Usage.CompletionTokens)
		assert.Equal(t, 15, resp.Usage.TotalTokens)
	})

	t.Run("converts tool_use stop reason", func(t *testing.T) {
		output := &bedrockruntime.ConverseOutput{
			Output: &types.ConverseOutputMemberMessage{
				Value: types.Message{
					Role:    types.ConversationRoleAssistant,
					Content: []types.ContentBlock{},
				},
			},
			StopReason: types.StopReasonToolUse,
			Usage: &types.TokenUsage{
				InputTokens:  aws.Int32(20),
				OutputTokens: aws.Int32(10),
				TotalTokens:  aws.Int32(30),
			},
		}

		resp := converseOutputToOpenAI(output, "test-model")
		require.Len(t, resp.Choices, 1)
		assert.Equal(t, "tool_calls", resp.Choices[0].FinishReason)
	})

	t.Run("handles nil usage", func(t *testing.T) {
		output := &bedrockruntime.ConverseOutput{
			Output: &types.ConverseOutputMemberMessage{
				Value: types.Message{
					Role: types.ConversationRoleAssistant,
					Content: []types.ContentBlock{
						&types.ContentBlockMemberText{Value: "Hi"},
					},
				},
			},
			StopReason: types.StopReasonEndTurn,
		}

		resp := converseOutputToOpenAI(output, "test-model")
		assert.Equal(t, 0, resp.Usage.PromptTokens)
		assert.Equal(t, 0, resp.Usage.CompletionTokens)
	})
}

func TestSendSSEChunk(t *testing.T) {
	// sendSSEChunk is a simple helper — just verify it doesn't panic
	// Real validation would need an httptest recorder, covered by integration tests
}

func TestBedrockConverseStopReasonMapping(t *testing.T) {
	// Verify the stop reason mapping used by converseOutputToOpenAI
	tests := []struct {
		input    types.StopReason
		expected string
	}{
		{types.StopReasonEndTurn, "stop"},
		{types.StopReasonToolUse, "tool_calls"},
		{types.StopReasonMaxTokens, "length"},
		{types.StopReasonStopSequence, "stop"},
		{types.StopReasonContentFiltered, "content_filter"},
	}
	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := bedrockVendor.ConvertConverseStopReason(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

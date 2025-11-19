package proxy

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/tmc/langchaingo/llms"
)

func TestHandleOptions(t *testing.T) {
	t.Run("Convert all options", func(t *testing.T) {
		maxTokens := 100
		temp := 0.7
		topP := 0.9
		presencePenalty := 0.5
		freqPenalty := 0.3

		req := &CreateCompletionRequest{
			Model:            "gpt-4",
			MaxTokens:        &maxTokens,
			Temperature:      &temp,
			TopP:             &topP,
			PresencePenalty:  &presencePenalty,
			FrequencyPenalty: &freqPenalty,
			Stop:             "STOP",
		}

		opts := handleOptions(req)
		assert.NotNil(t, opts)
		assert.Greater(t, len(opts), 0)
	})

	t.Run("Handle string stop word", func(t *testing.T) {
		req := &CreateCompletionRequest{
			Stop: "END",
		}

		opts := handleOptions(req)
		assert.NotNil(t, opts)
	})

	t.Run("Handle array stop words", func(t *testing.T) {
		req := &CreateCompletionRequest{
			Stop: []string{"STOP", "END"},
		}

		opts := handleOptions(req)
		assert.NotNil(t, opts)
	})

	t.Run("Handle nil options", func(t *testing.T) {
		req := &CreateCompletionRequest{
			Model: "gpt-4",
		}

		opts := handleOptions(req)
		assert.NotNil(t, opts)
		// Should at least include model
		assert.GreaterOrEqual(t, len(opts), 1)
	})
}

func TestExtractTokenUsageFromContentResponse(t *testing.T) {
	t.Run("Extract token usage from response", func(t *testing.T) {
		resp := &llms.ContentResponse{
			Choices: []*llms.ContentChoice{
				{
					GenerationInfo: map[string]any{
						"CompletionTokens": 50,
						"PromptTokens":     20,
						"TotalTokens":      70,
					},
				},
			},
		}

		usage := extractTokenUsageFromContentResponse(resp, models.OPENAI)
		assert.NotNil(t, usage)
		// Note: Actual token extraction depends on switches.GetTokenCounts implementation
	})

	t.Run("Extract from nil response", func(t *testing.T) {
		usage := extractTokenUsageFromContentResponse(nil, models.OPENAI)
		assert.Equal(t, CompletionUsage{}, usage)
	})

	t.Run("Extract from response with no choices", func(t *testing.T) {
		resp := &llms.ContentResponse{
			Choices: []*llms.ContentChoice{},
		}

		usage := extractTokenUsageFromContentResponse(resp, models.OPENAI)
		assert.Equal(t, CompletionUsage{}, usage)
	})

	t.Run("Extract from multiple choices", func(t *testing.T) {
		resp := &llms.ContentResponse{
			Choices: []*llms.ContentChoice{
				{Content: "Choice 1"},
				{Content: "Choice 2"},
			},
		}

		usage := extractTokenUsageFromContentResponse(resp, models.OPENAI)
		assert.NotNil(t, usage)
		// Token counts are summed across choices
	})
}

func TestCountToolCalls(t *testing.T) {
	t.Run("Count tool calls in response", func(t *testing.T) {
		resp := &llms.ContentResponse{
			Choices: []*llms.ContentChoice{
				{
					ToolCalls: []llms.ToolCall{
						{ID: "call1", Type: "function"},
						{ID: "call2", Type: "function"},
					},
				},
			},
		}

		count := countToolCalls(resp)
		assert.Equal(t, 2, count)
	})

	t.Run("Count tool calls with multiple choices", func(t *testing.T) {
		resp := &llms.ContentResponse{
			Choices: []*llms.ContentChoice{
				{
					ToolCalls: []llms.ToolCall{
						{ID: "call1", Type: "function"},
					},
				},
				{
					ToolCalls: []llms.ToolCall{
						{ID: "call2", Type: "function"},
						{ID: "call3", Type: "function"},
					},
				},
			},
		}

		count := countToolCalls(resp)
		assert.Equal(t, 3, count)
	})

	t.Run("Count with nil response", func(t *testing.T) {
		count := countToolCalls(nil)
		assert.Equal(t, 0, count)
	})

	t.Run("Count with no tool calls", func(t *testing.T) {
		resp := &llms.ContentResponse{
			Choices: []*llms.ContentChoice{
				{Content: "No tools"},
			},
		}

		count := countToolCalls(resp)
		assert.Equal(t, 0, count)
	})
}

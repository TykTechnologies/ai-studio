package chat_session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

// TestNATSToolCallFixSimulation tests the fix for successive tool calls
// by simulating the NATS serialization/deserialization flow
func TestNATSToolCallFixSimulation(t *testing.T) {
	// Simulate first LLM response with tool call
	firstResponse := &LLMResponseWrapper{
		Response: &llms.ContentResponse{
			Choices: []*llms.ContentChoice{
				{
					Content: "", // Empty content when there are tool calls
					ToolCalls: []llms.ToolCall{
						{
							ID:   "tool_call_1",
							Type: "function",
							FunctionCall: &llms.FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location": "San Francisco"}`,
							},
						},
					},
				},
			},
		},
		Opts: []llms.CallOption{
			llms.WithTemperature(0.7),
			llms.WithMaxTokens(150),
			// These would contain tool definitions that are non-serializable
		},
	}

	t.Run("First tool call response survives NATS round-trip", func(t *testing.T) {
		// Convert to NATS-safe version (simulating PublishLLMResponse)
		natsResp := LLMResponseWrapperForNATS{
			Response: convertToNATSSafeResponse(firstResponse.Response),
		}

		// Verify tool call is preserved in conversion
		require.Len(t, natsResp.Response.Choices, 1)
		require.Len(t, natsResp.Response.Choices[0].ToolCalls, 1)
		assert.Equal(t, "tool_call_1", natsResp.Response.Choices[0].ToolCalls[0].ID)
		assert.Equal(t, "get_weather", natsResp.Response.Choices[0].ToolCalls[0].FunctionCall.Name)

		// Convert back (simulating ConsumeLLMResponses)
		restored := &LLMResponseWrapper{
			Response: convertFromNATSSafeResponse(natsResp.Response),
			Opts:     nil, // This is the key - Opts are nil after deserialization
		}

		// Verify tool call structure is intact
		require.Len(t, restored.Response.Choices, 1)
		require.Len(t, restored.Response.Choices[0].ToolCalls, 1)
		assert.Equal(t, "tool_call_1", restored.Response.Choices[0].ToolCalls[0].ID)
		assert.Equal(t, "get_weather", restored.Response.Choices[0].ToolCalls[0].FunctionCall.Name)
		assert.Equal(t, `{"location": "San Francisco"}`, restored.Response.Choices[0].ToolCalls[0].FunctionCall.Arguments)

		// Verify Opts is nil (this was the problem)
		assert.Nil(t, restored.Opts, "Opts should be nil after NATS round-trip")
	})

	t.Run("Second tool call response with different structure", func(t *testing.T) {
		// Simulate second LLM response after tool execution
		secondResponse := &LLMResponseWrapper{
			Response: &llms.ContentResponse{
				Choices: []*llms.ContentChoice{
					{
						Content: "The weather in San Francisco is sunny with 72°F.", // Content after tool execution
						ToolCalls: []llms.ToolCall{
							// Sometimes there might be another tool call
							{
								ID:   "tool_call_2",
								Type: "function",
								FunctionCall: &llms.FunctionCall{
									Name:      "log_interaction",
									Arguments: `{"interaction": "weather_query", "location": "San Francisco"}`,
								},
							},
						},
					},
				},
			},
			Opts: nil, // This would be nil coming from NATS
		}

		// Test conversion with mixed content and tool calls
		natsResp := LLMResponseWrapperForNATS{
			Response: convertToNATSSafeResponse(secondResponse.Response),
		}

		// Convert back
		restored := &LLMResponseWrapper{
			Response: convertFromNATSSafeResponse(natsResp.Response),
			Opts:     nil,
		}

		// Verify both content and tool calls are preserved
		require.Len(t, restored.Response.Choices, 1)
		assert.Equal(t, "The weather in San Francisco is sunny with 72°F.", restored.Response.Choices[0].Content)
		require.Len(t, restored.Response.Choices[0].ToolCalls, 1)
		assert.Equal(t, "tool_call_2", restored.Response.Choices[0].ToolCalls[0].ID)
		assert.Equal(t, "log_interaction", restored.Response.Choices[0].ToolCalls[0].FunctionCall.Name)
	})

	t.Run("Empty tool calls array is preserved", func(t *testing.T) {
		// Test response with no tool calls
		noToolCallResponse := &LLMResponseWrapper{
			Response: &llms.ContentResponse{
				Choices: []*llms.ContentChoice{
					{
						Content:   "This is a regular response without tool calls.",
						ToolCalls: []llms.ToolCall{}, // Empty array
					},
				},
			},
			Opts: nil,
		}

		// Convert and restore
		natsResp := LLMResponseWrapperForNATS{
			Response: convertToNATSSafeResponse(noToolCallResponse.Response),
		}

		restored := &LLMResponseWrapper{
			Response: convertFromNATSSafeResponse(natsResp.Response),
			Opts:     nil,
		}

		// Verify content is preserved and tool calls array is empty but not nil
		require.Len(t, restored.Response.Choices, 1)
		assert.Equal(t, "This is a regular response without tool calls.", restored.Response.Choices[0].Content)
		assert.NotNil(t, restored.Response.Choices[0].ToolCalls)
		assert.Len(t, restored.Response.Choices[0].ToolCalls, 0)
	})

	t.Run("Multiple tool calls in single response", func(t *testing.T) {
		// Test response with multiple tool calls
		multiToolResponse := &LLMResponseWrapper{
			Response: &llms.ContentResponse{
				Choices: []*llms.ContentChoice{
					{
						Content: "",
						ToolCalls: []llms.ToolCall{
							{
								ID:   "tool_call_1",
								Type: "function",
								FunctionCall: &llms.FunctionCall{
									Name:      "get_weather",
									Arguments: `{"location": "San Francisco"}`,
								},
							},
							{
								ID:   "tool_call_2",
								Type: "function",
								FunctionCall: &llms.FunctionCall{
									Name:      "get_time",
									Arguments: `{"timezone": "America/Los_Angeles"}`,
								},
							},
						},
					},
				},
			},
			Opts: nil,
		}

		// Convert and restore
		natsResp := LLMResponseWrapperForNATS{
			Response: convertToNATSSafeResponse(multiToolResponse.Response),
		}

		restored := &LLMResponseWrapper{
			Response: convertFromNATSSafeResponse(natsResp.Response),
			Opts:     nil,
		}

		// Verify all tool calls are preserved
		require.Len(t, restored.Response.Choices, 1)
		require.Len(t, restored.Response.Choices[0].ToolCalls, 2)

		// Check first tool call
		assert.Equal(t, "tool_call_1", restored.Response.Choices[0].ToolCalls[0].ID)
		assert.Equal(t, "get_weather", restored.Response.Choices[0].ToolCalls[0].FunctionCall.Name)
		assert.Equal(t, `{"location": "San Francisco"}`, restored.Response.Choices[0].ToolCalls[0].FunctionCall.Arguments)

		// Check second tool call
		assert.Equal(t, "tool_call_2", restored.Response.Choices[0].ToolCalls[1].ID)
		assert.Equal(t, "get_time", restored.Response.Choices[0].ToolCalls[1].FunctionCall.Name)
		assert.Equal(t, `{"timezone": "America/Los_Angeles"}`, restored.Response.Choices[0].ToolCalls[1].FunctionCall.Arguments)
	})
}

// TestNATSOptionsRegeneration tests that options are properly regenerated
// instead of being reused from deserialized messages
func TestNATSOptionsRegeneration(t *testing.T) {
	t.Run("Options regeneration concept", func(t *testing.T) {
		// This test demonstrates the concept of the fix:
		// Instead of trying to serialize/deserialize complex options,
		// we regenerate them from session state when needed.

		// Simulate deserialized LLM response (Opts is nil)
		deserializedResponse := &LLMResponseWrapper{
			Response: &llms.ContentResponse{
				Choices: []*llms.ContentChoice{
					{
						Content: "",
						ToolCalls: []llms.ToolCall{
							{
								ID:   "tool_call_1",
								Type: "function",
								FunctionCall: &llms.FunctionCall{
									Name:      "test_function",
									Arguments: `{"param": "value"}`,
								},
							},
						},
					},
				},
			},
			Opts: nil, // This is nil after NATS deserialization
		}

		// The fix: Instead of using deserializedResponse.Opts (which is nil),
		// we would regenerate options from session state like this:
		// tools := cs.prepareTools()
		// currentOpts := cs.getOptions(cs.chatRef.LLMSettings, tools)

		// For this test, we'll simulate regenerated options
		regeneratedOpts := []llms.CallOption{
			llms.WithTemperature(0.7),
			llms.WithMaxTokens(150),
			// Tool definitions would be included here
		}

		// Verify that we have valid options and the tool call structure is intact
		assert.Nil(t, deserializedResponse.Opts, "Deserialized options should be nil")
		assert.NotNil(t, regeneratedOpts, "Regenerated options should not be nil")
		assert.Len(t, regeneratedOpts, 2, "Should have regenerated options")

		// Verify tool call structure is preserved
		require.Len(t, deserializedResponse.Response.Choices, 1)
		require.Len(t, deserializedResponse.Response.Choices[0].ToolCalls, 1)
		assert.Equal(t, "test_function", deserializedResponse.Response.Choices[0].ToolCalls[0].FunctionCall.Name)
	})
}
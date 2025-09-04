package chat_session

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

// TestLLMResponseWrapperSerialization tests that we can properly serialize/deserialize
// LLMResponseWrapper with CallOptions without JSON marshaling errors
func TestLLMResponseWrapperSerialization(t *testing.T) {
	// Create a complex LLM response with various call options that would cause JSON errors
	originalResp := &LLMResponseWrapper{
		Response: &llms.ContentResponse{
			Choices: []*llms.ContentChoice{
				{
					Content: "Test response content",
				},
			},
		},
		Opts: []llms.CallOption{
			llms.WithTemperature(0.7),
			llms.WithMaxTokens(150),
			llms.WithModel("gpt-4"),
			// These options would contain function pointers and complex types
			// that cause JSON marshaling to fail with "unsupported type" errors
		},
	}

	t.Run("Original LLMResponseWrapper cannot be JSON marshaled", func(t *testing.T) {
		// This should fail due to function pointers in the Opts field
		_, err := json.Marshal(originalResp)
		assert.Error(t, err, "Expected JSON marshaling to fail with function pointers in Opts")
		assert.Contains(t, err.Error(), "unsupported type", "Error should mention unsupported type")
	})

	t.Run("LLMResponseWrapperForNATS can be JSON marshaled", func(t *testing.T) {
		// Convert to NATS-safe version
		natsResp := LLMResponseWrapperForNATS{
			Response: convertToNATSSafeResponse(originalResp.Response),
		}

		// This should succeed
		jsonData, err := json.Marshal(natsResp)
		require.NoError(t, err, "NATS-safe version should marshal without error")
		assert.NotEmpty(t, jsonData, "Marshaled JSON should not be empty")

		// Verify we can unmarshal it back
		var unmarshaled LLMResponseWrapperForNATS
		err = json.Unmarshal(jsonData, &unmarshaled)
		require.NoError(t, err, "Should be able to unmarshal NATS-safe version")

		// Verify the content is preserved
		assert.Equal(t, natsResp.Response.Choices[0].Content,
			unmarshaled.Response.Choices[0].Content,
			"Response content should be preserved")
	})

	t.Run("NATS serialization flow simulation", func(t *testing.T) {
		// Simulate the NATS publication flow
		natsResp := LLMResponseWrapperForNATS{
			Response: convertToNATSSafeResponse(originalResp.Response),
		}

		// Serialize (what happens in publishToNATS)
		dataBytes, err := json.Marshal(natsResp)
		require.NoError(t, err, "Serialization should succeed")

		// Create NATS message wrapper
		natsMsg := NATSMessage{
			Type:      MessageTypeLLMResponse,
			SessionID: "test-session",
			Data:      dataBytes,
		}

		msgBytes, err := json.Marshal(natsMsg)
		require.NoError(t, err, "NATS message should serialize")

		// Simulate deserialization (what happens in handleNATSMessage)
		var deserializedNATSMsg NATSMessage
		err = json.Unmarshal(msgBytes, &deserializedNATSMsg)
		require.NoError(t, err, "Should deserialize NATS message")

		// Deserialize the LLM response data
		var deserializedNATSResp LLMResponseWrapperForNATS
		err = json.Unmarshal(deserializedNATSMsg.Data, &deserializedNATSResp)
		require.NoError(t, err, "Should deserialize LLM response data")

		// Create the final LLMResponseWrapper (what the consumer receives)
		finalResp := &LLMResponseWrapper{
			Response: convertFromNATSSafeResponse(deserializedNATSResp.Response),
			Opts:     nil, // Empty opts - will be regenerated from session state
		}

		// Verify the response content is intact
		require.NotNil(t, finalResp.Response)
		require.Len(t, finalResp.Response.Choices, 1)
		assert.Equal(t, "Test response content", finalResp.Response.Choices[0].Content)

		// Verify Opts is nil (as expected after deserialization)
		assert.Nil(t, finalResp.Opts, "Opts should be nil after NATS round-trip")
	})

	t.Run("LLM response with tool calls serialization", func(t *testing.T) {
		// Create a response with tool calls to test the reported issue
		responseWithToolCalls := &LLMResponseWrapper{
			Response: &llms.ContentResponse{
				Choices: []*llms.ContentChoice{
					{
						Content: "", // Empty content when there are tool calls
						ToolCalls: []llms.ToolCall{
							{
								ID:   "test_tool_call_1",
								Type: "function",
								FunctionCall: &llms.FunctionCall{
									Name:      "test_function",
									Arguments: `{"param1": "value1", "param2": 42}`,
								},
							},
						},
					},
				},
			},
			Opts: []llms.CallOption{
				llms.WithTemperature(0.7),
			},
		}

		// Convert to NATS-safe version and serialize
		natsResp := LLMResponseWrapperForNATS{
			Response: convertToNATSSafeResponse(responseWithToolCalls.Response),
		}

		// This should succeed
		dataBytes, err := json.Marshal(natsResp)
		require.NoError(t, err, "Should serialize LLM response with tool calls")

		// Create NATS message wrapper
		natsMsg := NATSMessage{
			Type:      MessageTypeLLMResponse,
			SessionID: "test-session",
			Data:      dataBytes,
		}

		msgBytes, err := json.Marshal(natsMsg)
		require.NoError(t, err, "NATS message with tool calls should serialize")

		// Simulate deserialization
		var deserializedNATSMsg NATSMessage
		err = json.Unmarshal(msgBytes, &deserializedNATSMsg)
		require.NoError(t, err, "Should deserialize NATS message")

		// Deserialize the LLM response data
		var deserializedNATSResp LLMResponseWrapperForNATS
		err = json.Unmarshal(deserializedNATSMsg.Data, &deserializedNATSResp)
		require.NoError(t, err, "Should deserialize LLM response data with tool calls")

		// Create final response
		finalResp := &LLMResponseWrapper{
			Response: convertFromNATSSafeResponse(deserializedNATSResp.Response),
			Opts:     nil,
		}

		// Verify tool calls are preserved
		require.NotNil(t, finalResp.Response)
		require.Len(t, finalResp.Response.Choices, 1)
		require.Len(t, finalResp.Response.Choices[0].ToolCalls, 1)

		toolCall := finalResp.Response.Choices[0].ToolCalls[0]
		assert.Equal(t, "test_tool_call_1", toolCall.ID)
		assert.Equal(t, "function", toolCall.Type)
		assert.Equal(t, "test_function", toolCall.FunctionCall.Name)
		assert.Equal(t, `{"param1": "value1", "param2": 42}`, toolCall.FunctionCall.Arguments)
	})
}

// TestNATSMessageTypeParsing ensures our message type constants work correctly
func TestNATSMessageTypeParsing(t *testing.T) {
	testCases := []struct {
		name        string
		messageType string
		expected    string
	}{
		{"Chat Response", MessageTypeChatResponse, "chat_response"},
		{"Stream", MessageTypeStream, "stream"},
		{"Error", MessageTypeError, "error"},
		{"LLM Response", MessageTypeLLMResponse, "llm_response"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.messageType)
		})
	}
}

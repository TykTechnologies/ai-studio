package bedrockVendor

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

// --- ParseRegionFromEndpoint ---

func TestParseRegionFromEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  string
		expected  string
		expectErr bool
	}{
		{
			name:     "full Bedrock URL",
			endpoint: "https://bedrock-runtime.us-east-1.amazonaws.com",
			expected: "us-east-1",
		},
		{
			name:     "eu-west-1 URL",
			endpoint: "https://bedrock-runtime.eu-west-1.amazonaws.com",
			expected: "eu-west-1",
		},
		{
			name:     "plain region string",
			endpoint: "us-west-2",
			expected: "us-west-2",
		},
		{
			name:     "ap-southeast-1",
			endpoint: "ap-southeast-1",
			expected: "ap-southeast-1",
		},
		{
			name:      "empty string",
			endpoint:  "",
			expectErr: true,
		},
		{
			name:      "invalid URL without bedrock-runtime prefix",
			endpoint:  "https://api.example.com/bedrock",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			region, err := ParseRegionFromEndpoint(tt.endpoint)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, region)
			}
		})
	}
}

// --- GetModelID ---

func TestGetModelID(t *testing.T) {
	llm := &models.LLM{DefaultModel: "anthropic.claude-3-sonnet-20240229-v1:0"}

	t.Run("returns override when provided", func(t *testing.T) {
		result := GetModelID(llm, "anthropic.claude-3-5-sonnet-20241022-v2:0")
		assert.Equal(t, "anthropic.claude-3-5-sonnet-20241022-v2:0", result)
	})

	t.Run("returns default model when no override", func(t *testing.T) {
		result := GetModelID(llm, "")
		assert.Equal(t, "anthropic.claude-3-sonnet-20240229-v1:0", result)
	})
}

// --- extractAWSCredentials ---

func TestExtractAWSCredentials(t *testing.T) {
	t.Run("valid credentials with all fields in metadata", func(t *testing.T) {
		llm := &models.LLM{
			Metadata: models.JSONMap{
				"aws_access_key_id":     "AKIAIOSFODNN7EXAMPLE",
				"aws_secret_access_key": "secret-access-key",
				"aws_session_token":     "session-token-123",
			},
		}
		accessKey, secretKey, sessionToken, err := extractAWSCredentials(llm)
		assert.NoError(t, err)
		assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", accessKey)
		assert.Equal(t, "secret-access-key", secretKey)
		assert.Equal(t, "session-token-123", sessionToken)
	})

	t.Run("missing secret access key", func(t *testing.T) {
		llm := &models.LLM{
			Metadata: models.JSONMap{
				"aws_access_key_id": "AKIAIOSFODNN7EXAMPLE",
			},
		}
		_, _, _, err := extractAWSCredentials(llm)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Secret Access Key is required")
	})

	t.Run("missing access key ID", func(t *testing.T) {
		llm := &models.LLM{
			Metadata: models.JSONMap{
				"aws_secret_access_key": "secret-key",
			},
		}
		_, _, _, err := extractAWSCredentials(llm)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Access Key ID is required")
	})

	t.Run("nil metadata", func(t *testing.T) {
		llm := &models.LLM{
			Metadata: nil,
		}
		_, _, _, err := extractAWSCredentials(llm)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "metadata is required")
	})

	t.Run("optional session token", func(t *testing.T) {
		llm := &models.LLM{
			Metadata: models.JSONMap{
				"aws_access_key_id":     "AKIAIOSFODNN7EXAMPLE",
				"aws_secret_access_key": "secret-key",
			},
		}
		accessKey, secretKey, sessionToken, err := extractAWSCredentials(llm)
		assert.NoError(t, err)
		assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", accessKey)
		assert.Equal(t, "secret-key", secretKey)
		assert.Equal(t, "", sessionToken)
	})
}

// --- GetTokenCounts ---

func TestGetTokenCounts(t *testing.T) {
	v := &Bedrock{}

	t.Run("returns zeros (tokens come from Converse API directly)", func(t *testing.T) {
		choice := &llms.ContentChoice{
			GenerationInfo: map[string]interface{}{
				"input_tokens":  100,
				"output_tokens": 50,
			},
		}
		total, prompt, response := v.GetTokenCounts(choice)
		assert.Equal(t, 0, total)
		assert.Equal(t, 0, prompt)
		assert.Equal(t, 0, response)
	})
}

// --- GetDriver ---

func TestGetDriver(t *testing.T) {
	v := &Bedrock{}

	t.Run("returns error directing to Converse API handlers", func(t *testing.T) {
		llm := &models.LLM{}
		_, err := v.GetDriver(llm, nil, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Converse API")
	})
}

// --- ProvidesEmbedder ---

func TestProvidesEmbedder(t *testing.T) {
	v := &Bedrock{}
	assert.False(t, v.ProvidesEmbedder())
}

// --- ProxyScreenRequest ---

func TestProxyScreenRequest(t *testing.T) {
	v := &Bedrock{}
	llm := &models.LLM{}

	t.Run("allows non-streaming requests", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/model/test/converse", nil)
		err := v.ProxyScreenRequest(llm, req, false)
		assert.NoError(t, err)
	})

	t.Run("allows streaming requests (handled via SDK-mediated path)", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/model/test/converse-stream", nil)
		err := v.ProxyScreenRequest(llm, req, true)
		assert.NoError(t, err)
	})
}

// --- ProxySetAuthHeader ---

func TestProxySetAuthHeader(t *testing.T) {
	v := &Bedrock{}

	t.Run("adds SigV4 headers", func(t *testing.T) {
		llm := &models.LLM{
			APIEndpoint: "https://bedrock-runtime.us-east-1.amazonaws.com",
			Metadata: models.JSONMap{
				"aws_access_key_id":     "AKIAIOSFODNN7EXAMPLE",
				"aws_secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			},
		}

		body := []byte(`{"modelId":"anthropic.claude-3-sonnet","messages":[]}`)
		req, _ := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/model/test/converse", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		err := v.ProxySetAuthHeader(req, llm)
		assert.NoError(t, err)

		// Verify SigV4 headers are present
		assert.NotEmpty(t, req.Header.Get("Authorization"), "Authorization header should be set")
		assert.Contains(t, req.Header.Get("Authorization"), "AWS4-HMAC-SHA256")
		assert.NotEmpty(t, req.Header.Get("X-Amz-Date"), "X-Amz-Date header should be set")

		// Verify body is still readable after signing
		readBody, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		assert.Equal(t, body, readBody)
	})

	t.Run("fails with missing credentials", func(t *testing.T) {
		llm := &models.LLM{
			APIEndpoint: "https://bedrock-runtime.us-east-1.amazonaws.com",
			Metadata:    models.JSONMap{},
		}

		body := []byte(`{}`)
		req, _ := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/model/test/converse", bytes.NewReader(body))

		err := v.ProxySetAuthHeader(req, llm)
		assert.Error(t, err)
	})
}

// --- AnalyzeResponse ---

func TestAnalyzeResponse(t *testing.T) {
	v := &Bedrock{}
	llm := &models.LLM{Vendor: models.BEDROCK}
	app := &models.App{}

	t.Run("parses Converse API response", func(t *testing.T) {
		responseBody := `{
			"output": {
				"message": {
					"role": "assistant",
					"content": [{"text": "Hello, world!"}]
				}
			},
			"stopReason": "end_turn",
			"usage": {
				"inputTokens": 15,
				"outputTokens": 8,
				"totalTokens": 23
			},
			"metrics": {
				"latencyMs": 350
			}
		}`

		req, _ := http.NewRequest("POST", "/model/test/converse", nil)
		llmResult, appResult, tokenResp, err := v.AnalyzeResponse(llm, app, 200, []byte(responseBody), req)

		assert.NoError(t, err)
		assert.Equal(t, llm, llmResult)
		assert.Equal(t, app, appResult)
		require.NotNil(t, tokenResp)
		assert.Equal(t, 15, tokenResp.GetPromptTokens())
		assert.Equal(t, 8, tokenResp.GetResponseTokens())
		assert.Equal(t, 1, tokenResp.GetChoiceCount())
	})

	t.Run("handles invalid JSON gracefully", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/model/test/converse", nil)
		_, _, tokenResp, err := v.AnalyzeResponse(llm, app, 200, []byte("invalid json"), req)

		assert.NoError(t, err) // Should not return error, returns empty response
		assert.NotNil(t, tokenResp)
		assert.Equal(t, 0, tokenResp.GetPromptTokens())
	})
}

// --- AnalyzeStreamingResponse ---

func TestAnalyzeStreamingResponse(t *testing.T) {
	v := &Bedrock{}
	llm := &models.LLM{Vendor: models.BEDROCK}
	app := &models.App{}

	t.Run("extracts token usage from SSE metadata event", func(t *testing.T) {
		sseResponse := `data: {"type":"messageStart","messageStart":{"role":"assistant"}}

data: {"type":"contentBlockDelta","contentBlockIndex":0,"delta":{"text":"Hello"}}

data: {"type":"messageStop","stopReason":"end_turn"}

data: {"type":"metadata","usage":{"inputTokens":10,"outputTokens":5,"totalTokens":15},"metrics":{"latencyMs":200}}

data: [DONE]

`

		req, _ := http.NewRequest("POST", "/model/test/converse-stream", nil)
		_, _, tokenResp, err := v.AnalyzeStreamingResponse(llm, app, 200, []byte(sseResponse), req, nil)

		assert.NoError(t, err)
		require.NotNil(t, tokenResp)
		assert.Equal(t, 10, tokenResp.GetPromptTokens())
		assert.Equal(t, 5, tokenResp.GetResponseTokens())
	})

	t.Run("returns zeros when no metadata event", func(t *testing.T) {
		sseResponse := `data: {"type":"contentBlockDelta","delta":{"text":"Hello"}}

data: [DONE]

`

		req, _ := http.NewRequest("POST", "/model/test/converse-stream", nil)
		_, _, tokenResp, err := v.AnalyzeStreamingResponse(llm, app, 200, []byte(sseResponse), req, nil)

		assert.NoError(t, err)
		assert.NotNil(t, tokenResp)
		assert.Equal(t, 0, tokenResp.GetPromptTokens())
		assert.Equal(t, 0, tokenResp.GetResponseTokens())
	})
}

// --- ConvertChatMessagesToConverse ---

func TestConvertChatMessagesToConverse(t *testing.T) {
	t.Run("separates system messages", func(t *testing.T) {
		messages := []ChatMessage{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hello!"},
			{Role: "assistant", Content: "Hi there!"},
			{Role: "user", Content: "How are you?"},
		}

		converseMsgs, systemBlocks := ConvertChatMessagesToConverse(messages)

		assert.Len(t, systemBlocks, 1)
		assert.Len(t, converseMsgs, 3)

		// Verify system block
		sysBlock, ok := systemBlocks[0].(*types.SystemContentBlockMemberText)
		require.True(t, ok)
		assert.Equal(t, "You are a helpful assistant.", sysBlock.Value)

		// Verify message roles
		assert.Equal(t, types.ConversationRoleUser, converseMsgs[0].Role)
		assert.Equal(t, types.ConversationRoleAssistant, converseMsgs[1].Role)
		assert.Equal(t, types.ConversationRoleUser, converseMsgs[2].Role)

		// Verify content
		textBlock, ok := converseMsgs[0].Content[0].(*types.ContentBlockMemberText)
		require.True(t, ok)
		assert.Equal(t, "Hello!", textBlock.Value)
	})

	t.Run("handles no system messages", func(t *testing.T) {
		messages := []ChatMessage{
			{Role: "user", Content: "Hello!"},
		}

		converseMsgs, systemBlocks := ConvertChatMessagesToConverse(messages)

		assert.Len(t, systemBlocks, 0)
		assert.Len(t, converseMsgs, 1)
	})

	t.Run("handles empty input", func(t *testing.T) {
		converseMsgs, systemBlocks := ConvertChatMessagesToConverse(nil)
		assert.Nil(t, converseMsgs)
		assert.Nil(t, systemBlocks)
	})
}

// --- BuildInferenceConfig ---

func TestBuildInferenceConfig(t *testing.T) {
	t.Run("builds config with all parameters", func(t *testing.T) {
		maxTokens := 1024
		temp := 0.7
		topP := 0.9

		config := BuildInferenceConfig(&maxTokens, &temp, &topP, []string{"STOP"})

		require.NotNil(t, config)
		assert.Equal(t, int32(1024), *config.MaxTokens)
		assert.InDelta(t, float32(0.7), *config.Temperature, 0.001)
		assert.InDelta(t, float32(0.9), *config.TopP, 0.001)
		assert.Equal(t, []string{"STOP"}, config.StopSequences)
	})

	t.Run("returns nil when no parameters set", func(t *testing.T) {
		config := BuildInferenceConfig(nil, nil, nil, nil)
		assert.Nil(t, config)
	})

	t.Run("handles stop as string", func(t *testing.T) {
		config := BuildInferenceConfig(nil, nil, nil, "END")
		require.NotNil(t, config)
		assert.Equal(t, []string{"END"}, config.StopSequences)
	})

	t.Run("handles stop as interface slice", func(t *testing.T) {
		stop := []interface{}{"STOP", "END"}
		config := BuildInferenceConfig(nil, nil, nil, stop)
		require.NotNil(t, config)
		assert.Equal(t, []string{"STOP", "END"}, config.StopSequences)
	})
}

// --- ConvertConverseStopReason ---

func TestConvertConverseStopReason(t *testing.T) {
	tests := []struct {
		input    types.StopReason
		expected string
	}{
		{types.StopReasonEndTurn, "stop"},
		{types.StopReasonToolUse, "tool_calls"},
		{types.StopReasonMaxTokens, "length"},
		{types.StopReasonStopSequence, "stop"},
		{types.StopReasonContentFiltered, "content_filter"},
		{types.StopReasonGuardrailIntervened, "content_filter"},
		{types.StopReason("unknown"), "stop"},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := ConvertConverseStopReason(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- ExtractTextFromContentBlocks ---

func TestExtractTextFromContentBlocks(t *testing.T) {
	t.Run("extracts text from multiple blocks", func(t *testing.T) {
		blocks := []types.ContentBlock{
			&types.ContentBlockMemberText{Value: "Hello, "},
			&types.ContentBlockMemberText{Value: "world!"},
		}
		result := ExtractTextFromContentBlocks(blocks)
		assert.Equal(t, "Hello, world!", result)
	})

	t.Run("skips non-text blocks", func(t *testing.T) {
		blocks := []types.ContentBlock{
			&types.ContentBlockMemberText{Value: "Hello"},
			&types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
				ToolUseId: strPtr("tool-1"),
				Name:      strPtr("get_weather"),
				Input:     document.NewLazyDocument(map[string]interface{}{}),
			}},
		}
		result := ExtractTextFromContentBlocks(blocks)
		assert.Equal(t, "Hello", result)
	})

	t.Run("returns empty for no text blocks", func(t *testing.T) {
		blocks := []types.ContentBlock{}
		result := ExtractTextFromContentBlocks(blocks)
		assert.Equal(t, "", result)
	})
}

// --- ExtractToolCallsFromContentBlocks ---

func TestExtractToolCallsFromContentBlocks(t *testing.T) {
	t.Run("extracts tool calls in OpenAI format", func(t *testing.T) {
		blocks := []types.ContentBlock{
			&types.ContentBlockMemberText{Value: "Let me check the weather."},
			&types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
				ToolUseId: strPtr("call_123"),
				Name:      strPtr("get_weather"),
				Input:     document.NewLazyDocument(map[string]interface{}{"location": "London"}),
			}},
		}

		toolCalls := ExtractToolCallsFromContentBlocks(blocks)
		assert.Len(t, toolCalls, 1)
		assert.Equal(t, "call_123", toolCalls[0]["id"])
		assert.Equal(t, "function", toolCalls[0]["type"])

		fn := toolCalls[0]["function"].(map[string]interface{})
		assert.Equal(t, "get_weather", fn["name"])

		// The bug this pins: Input is a document.Interface whose payload lives
		// in an unexported field, so json.Marshal produced "{}" and every
		// Bedrock tool call reached the client with empty arguments - valid
		// against the schema, and useless.
		var args map[string]any
		require.NoError(t, json.Unmarshal([]byte(fn["arguments"].(string)), &args))
		assert.Equal(t, map[string]any{"location": "London"}, args)
	})

	t.Run("returns nil when no tool calls", func(t *testing.T) {
		blocks := []types.ContentBlock{
			&types.ContentBlockMemberText{Value: "Hello"},
		}
		toolCalls := ExtractToolCallsFromContentBlocks(blocks)
		assert.Nil(t, toolCalls)
	})
}

// --- BuildToolConfig ---

func TestBuildToolConfig(t *testing.T) {
	t.Run("builds tool config from definitions", func(t *testing.T) {
		tools := []ToolDef{
			{
				Type: "function",
				Function: ToolFuncDef{
					Name:        "get_weather",
					Description: "Get current weather",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{"type": "string"},
						},
					},
				},
			},
		}

		config := BuildToolConfig(tools)
		require.NotNil(t, config)
		assert.Len(t, config.Tools, 1)
	})

	t.Run("returns nil for empty tools", func(t *testing.T) {
		config := BuildToolConfig(nil)
		assert.Nil(t, config)
	})

	t.Run("skips non-function tools", func(t *testing.T) {
		tools := []ToolDef{
			{Type: "retrieval"},
		}
		config := BuildToolConfig(tools)
		assert.Nil(t, config)
	})
}

// --- sha256Hex ---

func TestSha256Hex(t *testing.T) {
	// SHA-256 of empty string
	result := sha256Hex([]byte(""))
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", result)

	// SHA-256 of "hello"
	result = sha256Hex([]byte("hello"))
	assert.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", result)
}

// --- convertToConverseRole ---

func TestConvertToConverseRole(t *testing.T) {
	tests := []struct {
		input    string
		expected types.ConversationRole
	}{
		{"user", types.ConversationRoleUser},
		{"human", types.ConversationRoleUser},
		{"assistant", types.ConversationRoleAssistant},
		{"ai", types.ConversationRoleAssistant},
		{"unknown", types.ConversationRoleUser},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := convertToConverseRole(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// helper
func strPtr(s string) *string {
	return &s
}

func TestMarshalToolInput(t *testing.T) {
	t.Run("renders the document payload, not the wrapper", func(t *testing.T) {
		doc := document.NewLazyDocument(map[string]interface{}{"city": "London", "days": 3})

		// json.Marshal is the trap: the wrapper has no exported fields.
		naive, err := json.Marshal(doc)
		require.NoError(t, err)
		assert.Equal(t, "{}", string(naive),
			"if this ever stops being {}, the workaround in MarshalToolInput can go")

		var args map[string]any
		require.NoError(t, json.Unmarshal([]byte(MarshalToolInput(doc)), &args))
		assert.Equal(t, "London", args["city"])
	})

	t.Run("a nil input is an empty object, not an empty string", func(t *testing.T) {
		// "" does not parse; clients json.Unmarshal this field directly.
		assert.Equal(t, "{}", MarshalToolInput(nil))
	})
}

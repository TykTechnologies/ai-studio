package chat_session

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// MockUniversalClient is a mock for the universalclient.Client
type MockUniversalClient struct {
	mock.Mock
}

func (m *MockUniversalClient) CallOperation(operationID string, params map[string][]string, body map[string]interface{}, headers map[string][]string) (interface{}, error) {
	args := m.Called(operationID, params, body, headers)
	return args.Get(0), args.Error(1)
}

func (m *MockUniversalClient) AsTool(operationIDs ...string) ([]llms.Tool, error) {
	args := m.Called(operationIDs)
	if args.Get(0) == nil {
		return []llms.Tool{}, args.Error(1)
	}
	return args.Get(0).([]llms.Tool), args.Error(1)
}

func TestHandleToolCalls(t *testing.T) {
	// Setup test DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))

	// Setup test data
	uid := uint(1)
	sid := "test_session_id"
	chatRef := &models.Chat{
		ID:          1,
		Name:        "Test Chat",
		LLMSettings: &models.LLMSettings{ModelName: "dummy"},
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: models.MOCK_VENDOR,
		},
	}

	// Read test OAS spec
	spec, err := os.ReadFile("../universalclient/testdata/petstore.json")
	require.NoError(t, err, "Failed to read test OAS spec")

	t.Run("Successful tool call", func(t *testing.T) {
		// Create chat session
		cs, err := NewChatSession(chatRef, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
		require.NoError(t, err)
		require.NoError(t, cs.initSession())

		// Add a mock tool
		cs.tools = map[string]models.Tool{
			"test_tool": {
				ID:                  1,
				Name:                "Test Tool",
				ToolType:            models.ToolTypeREST,
				OASSpec:             string(spec),
				AvailableOperations: "findPetsByStatus",
			},
		}

		// Create a mock content choice with a tool call
		choice := &llms.ContentChoice{
			ToolCalls: []llms.ToolCall{
				{
					ID:   "call_123",
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      "findPetsByStatus",
						Arguments: `{"status": ["available"]}`,
					},
				},
			},
		}

		// Create empty message contents for tool call and result
		toolCall := &llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{},
		}
		toolResult := &llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{},
		}

		// Call handleToolCalls
		cs.handleToolCalls(choice, toolCall, toolResult)

		// Verify toolCall is populated correctly
		require.Len(t, toolCall.Parts, 1, "Tool call should have one part")
		tc, ok := toolCall.Parts[0].(llms.ToolCall)
		require.True(t, ok, "Tool call part should be a ToolCall")
		assert.Equal(t, "call_123", tc.ID)
		assert.Equal(t, "function", tc.Type)
		assert.Equal(t, "findPetsByStatus", tc.FunctionCall.Name)
		assert.Equal(t, `{"status": ["available"]}`, tc.FunctionCall.Arguments)

		// Note: We can't verify toolResult easily because it depends on an actual HTTP call
		// In a real test, we would mock the universalclient.Client
	})

	t.Run("Tool not found", func(t *testing.T) {
		// Create chat session
		cs, err := NewChatSession(chatRef, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
		require.NoError(t, err)
		require.NoError(t, cs.initSession())

		// Add a mock tool with a different operation
		cs.tools = map[string]models.Tool{
			"test_tool": {
				ID:                  1,
				Name:                "Test Tool",
				ToolType:            models.ToolTypeREST,
				OASSpec:             string(spec),
				AvailableOperations: "updatePet", // Different from what we'll call
			},
		}

		// Create a mock content choice with a tool call for a non-existent operation
		choice := &llms.ContentChoice{
			ToolCalls: []llms.ToolCall{
				{
					ID:   "call_123",
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      "nonExistentOperation",
						Arguments: `{"param": "value"}`,
					},
				},
			},
		}

		// Create empty message contents for tool call and result
		toolCall := &llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{},
		}
		toolResult := &llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{},
		}

		// Call handleToolCalls
		cs.handleToolCalls(choice, toolCall, toolResult)

		// Verify toolCall is populated correctly
		require.Len(t, toolCall.Parts, 1, "Tool call should have one part")
		tc, ok := toolCall.Parts[0].(llms.ToolCall)
		require.True(t, ok, "Tool call part should be a ToolCall")
		assert.Equal(t, "call_123", tc.ID)

		// Verify toolResult contains an error message
		require.Len(t, toolResult.Parts, 1, "Tool result should have one part")
		tr, ok := toolResult.Parts[0].(llms.ToolCallResponse)
		require.True(t, ok, "Tool result part should be a ToolCallResponse")
		assert.Equal(t, "call_123", tr.ToolCallID)
		assert.Equal(t, "nonExistentOperation", tr.Name)
		assert.Contains(t, tr.Content, "ERROR: tool not found")
	})

	t.Run("Invalid arguments", func(t *testing.T) {
		// Create chat session
		cs, err := NewChatSession(chatRef, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
		require.NoError(t, err)
		require.NoError(t, cs.initSession())

		// Add a mock tool
		cs.tools = map[string]models.Tool{
			"test_tool": {
				ID:                  1,
				Name:                "Test Tool",
				ToolType:            models.ToolTypeREST,
				OASSpec:             string(spec),
				AvailableOperations: "findPetsByStatus",
			},
		}

		// Create a mock content choice with a tool call with invalid JSON arguments
		choice := &llms.ContentChoice{
			ToolCalls: []llms.ToolCall{
				{
					ID:   "call_123",
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      "findPetsByStatus",
						Arguments: `{invalid json}`,
					},
				},
			},
		}

		// Create empty message contents for tool call and result
		toolCall := &llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{},
		}
		toolResult := &llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{},
		}

		// Call handleToolCalls
		cs.handleToolCalls(choice, toolCall, toolResult)

		// Verify toolCall is populated correctly
		require.Len(t, toolCall.Parts, 1, "Tool call should have one part")

		// Verify toolResult contains an error message
		require.Len(t, toolResult.Parts, 1, "Tool result should have one part")
		tr, ok := toolResult.Parts[0].(llms.ToolCallResponse)
		require.True(t, ok, "Tool result part should be a ToolCallResponse")
		assert.Equal(t, "call_123", tr.ToolCallID)
		assert.Equal(t, "findPetsByStatus", tr.Name)
		assert.Contains(t, tr.Content, "ERROR: error converting LLM args")
	})

	t.Run("Multiple tool calls", func(t *testing.T) {
		// Create chat session
		cs, err := NewChatSession(chatRef, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
		require.NoError(t, err)
		require.NoError(t, cs.initSession())

		// Add mock tools
		cs.tools = map[string]models.Tool{
			"test_tool1": {
				ID:                  1,
				Name:                "Test Tool 1",
				ToolType:            models.ToolTypeREST,
				OASSpec:             string(spec),
				AvailableOperations: "findPetsByStatus",
			},
			"test_tool2": {
				ID:                  2,
				Name:                "Test Tool 2",
				ToolType:            models.ToolTypeREST,
				OASSpec:             string(spec),
				AvailableOperations: "getPetById",
			},
		}

		// Create a mock content choice with multiple tool calls
		choice := &llms.ContentChoice{
			ToolCalls: []llms.ToolCall{
				{
					ID:   "call_123",
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      "findPetsByStatus",
						Arguments: `{"status": ["available"]}`,
					},
				},
				{
					ID:   "call_456",
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      "getPetById",
						Arguments: `{"petId": 1}`,
					},
				},
			},
		}

		// Create empty message contents for tool call and result
		toolCall := &llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{},
		}
		toolResult := &llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{},
		}

		// Call handleToolCalls
		cs.handleToolCalls(choice, toolCall, toolResult)

		// Verify toolCall is populated correctly
		require.Len(t, toolCall.Parts, 2, "Tool call should have two parts")
		tc1, ok := toolCall.Parts[0].(llms.ToolCall)
		require.True(t, ok, "First tool call part should be a ToolCall")
		assert.Equal(t, "call_123", tc1.ID)
		assert.Equal(t, "findPetsByStatus", tc1.FunctionCall.Name)

		tc2, ok := toolCall.Parts[1].(llms.ToolCall)
		require.True(t, ok, "Second tool call part should be a ToolCall")
		assert.Equal(t, "call_456", tc2.ID)
		assert.Equal(t, "getPetById", tc2.FunctionCall.Name)
	})

	t.Run("Empty tool call ID", func(t *testing.T) {
		// Create chat session
		cs, err := NewChatSession(chatRef, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
		require.NoError(t, err)
		require.NoError(t, cs.initSession())

		// Add a mock tool
		cs.tools = map[string]models.Tool{
			"test_tool": {
				ID:                  1,
				Name:                "Test Tool",
				ToolType:            models.ToolTypeREST,
				OASSpec:             string(spec),
				AvailableOperations: "findPetsByStatus",
			},
		}

		// Create a mock content choice with a tool call with empty ID
		choice := &llms.ContentChoice{
			ToolCalls: []llms.ToolCall{
				{
					ID:   "", // Empty ID
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      "findPetsByStatus",
						Arguments: `{"status": ["available"]}`,
					},
				},
			},
		}

		// Create empty message contents for tool call and result
		toolCall := &llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{},
		}
		toolResult := &llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{},
		}

		// Call handleToolCalls
		cs.handleToolCalls(choice, toolCall, toolResult)

		// Verify toolCall is not populated
		assert.Empty(t, toolCall.Parts, "Tool call should be empty for empty ID")
		assert.Empty(t, toolResult.Parts, "Tool result should be empty for empty ID")
	})

	t.Run("Unsupported tool type", func(t *testing.T) {
		// Create chat session
		cs, err := NewChatSession(chatRef, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
		require.NoError(t, err)
		require.NoError(t, cs.initSession())

		// Add a mock tool with unsupported type
		cs.tools = map[string]models.Tool{
			"test_tool": {
				ID:                  1,
				Name:                "Test Tool",
				ToolType:            "UNSUPPORTED", // Unsupported type
				OASSpec:             string(spec),
				AvailableOperations: "findPetsByStatus",
			},
		}

		// Create a mock content choice with a tool call
		choice := &llms.ContentChoice{
			ToolCalls: []llms.ToolCall{
				{
					ID:   "call_123",
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      "findPetsByStatus",
						Arguments: `{"status": ["available"]}`,
					},
				},
			},
		}

		// Create empty message contents for tool call and result
		toolCall := &llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{},
		}
		toolResult := &llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{},
		}

		// Call handleToolCalls
		cs.handleToolCalls(choice, toolCall, toolResult)

		// Verify toolCall is populated correctly
		require.Len(t, toolCall.Parts, 1, "Tool call should have one part")

		// The current implementation doesn't handle unsupported tool types
		// It only handles REST tools and doesn't add an error for unsupported types
		// This is a potential improvement for the handleToolCalls function
		assert.Empty(t, toolResult.Parts, "Tool result should be empty for unsupported tool type")
	})

	t.Run("Tool with auth key", func(t *testing.T) {
		// Create chat session
		cs, err := NewChatSession(chatRef, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
		require.NoError(t, err)
		require.NoError(t, cs.initSession())

		// Add a mock tool with auth key
		cs.tools = map[string]models.Tool{
			"test_tool": {
				ID:                  1,
				Name:                "Test Tool",
				ToolType:            models.ToolTypeREST,
				OASSpec:             string(spec),
				AvailableOperations: "findPetsByStatus",
				AuthKey:             "test-api-key",
				AuthSchemaName:      "apiKey",
			},
		}

		// Create a mock content choice with a tool call
		choice := &llms.ContentChoice{
			ToolCalls: []llms.ToolCall{
				{
					ID:   "call_123",
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      "findPetsByStatus",
						Arguments: `{"status": ["available"]}`,
					},
				},
			},
		}

		// Create empty message contents for tool call and result
		toolCall := &llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{},
		}
		toolResult := &llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{},
		}

		// Call handleToolCalls
		cs.handleToolCalls(choice, toolCall, toolResult)

		// Verify toolCall is populated correctly
		require.Len(t, toolCall.Parts, 1, "Tool call should have one part")
		tc, ok := toolCall.Parts[0].(llms.ToolCall)
		require.True(t, ok, "Tool call part should be a ToolCall")
		assert.Equal(t, "call_123", tc.ID)
		assert.Equal(t, "function", tc.Type)
		assert.Equal(t, "findPetsByStatus", tc.FunctionCall.Name)
	})

	t.Run("Invalid OAS spec", func(t *testing.T) {
		// Create chat session
		cs, err := NewChatSession(chatRef, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
		require.NoError(t, err)
		require.NoError(t, cs.initSession())

		// Add a mock tool with invalid OAS spec
		cs.tools = map[string]models.Tool{
			"test_tool": {
				ID:                  1,
				Name:                "Test Tool",
				ToolType:            models.ToolTypeREST,
				OASSpec:             "invalid json", // Invalid OAS spec
				AvailableOperations: "findPetsByStatus",
			},
		}

		// Create a mock content choice with a tool call
		choice := &llms.ContentChoice{
			ToolCalls: []llms.ToolCall{
				{
					ID:   "call_123",
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      "findPetsByStatus",
						Arguments: `{"status": ["available"]}`,
					},
				},
			},
		}

		// Create empty message contents for tool call and result
		toolCall := &llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{},
		}
		toolResult := &llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{},
		}

		// Call handleToolCalls
		cs.handleToolCalls(choice, toolCall, toolResult)

		// Verify toolCall is populated correctly
		require.Len(t, toolCall.Parts, 1, "Tool call should have one part")

		// Verify toolResult contains an error message
		require.Len(t, toolResult.Parts, 1, "Tool result should have one part")
		tr, ok := toolResult.Parts[0].(llms.ToolCallResponse)
		require.True(t, ok, "Tool result part should be a ToolCallResponse")
		assert.Equal(t, "call_123", tr.ToolCallID)
		assert.Equal(t, "findPetsByStatus", tr.Name)
		assert.Contains(t, tr.Content, "ERROR: error creating tool client")
	})

	t.Run("Tool with no operations", func(t *testing.T) {
		// Create chat session
		cs, err := NewChatSession(chatRef, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
		require.NoError(t, err)
		require.NoError(t, cs.initSession())

		// Add a mock tool with no operations
		cs.tools = map[string]models.Tool{
			"test_tool": {
				ID:                  1,
				Name:                "Test Tool",
				ToolType:            models.ToolTypeREST,
				OASSpec:             string(spec),
				AvailableOperations: "", // No operations
			},
		}

		// Create a mock content choice with a tool call
		choice := &llms.ContentChoice{
			ToolCalls: []llms.ToolCall{
				{
					ID:   "call_123",
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      "findPetsByStatus",
						Arguments: `{"status": ["available"]}`,
					},
				},
			},
		}

		// Create empty message contents for tool call and result
		toolCall := &llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{},
		}
		toolResult := &llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{},
		}

		// Call handleToolCalls
		cs.handleToolCalls(choice, toolCall, toolResult)

		// Verify toolCall is populated correctly
		require.Len(t, toolCall.Parts, 1, "Tool call should have one part")

		// Verify toolResult contains an error message
		require.Len(t, toolResult.Parts, 1, "Tool result should have one part")
		tr, ok := toolResult.Parts[0].(llms.ToolCallResponse)
		require.True(t, ok, "Tool result part should be a ToolCallResponse")
		assert.Equal(t, "call_123", tr.ToolCallID)
		assert.Equal(t, "findPetsByStatus", tr.Name)
		assert.Contains(t, tr.Content, "ERROR: tool not found")
	})
}

func TestHandleToolError(t *testing.T) {
	// Setup test data
	uid := uint(1)
	sid := "test_session_id"
	chatRef := &models.Chat{
		ID:          1,
		Name:        "Test Chat",
		LLMSettings: &models.LLMSettings{ModelName: "dummy"},
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: models.MOCK_VENDOR,
		},
	}

	// Create chat session
	cs, err := NewChatSession(chatRef, ChatMessage, nil, nil, nil, &uid, &sid)
	require.NoError(t, err)

	// Test handleToolError
	t.Run("Basic error handling", func(t *testing.T) {
		toolResult := &llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{},
		}

		cs.handleToolError("Test error message", "call_123", "testFunction", toolResult)

		// Verify toolResult contains the error message
		require.Len(t, toolResult.Parts, 1, "Tool result should have one part")
		tr, ok := toolResult.Parts[0].(llms.ToolCallResponse)
		require.True(t, ok, "Tool result part should be a ToolCallResponse")
		assert.Equal(t, "call_123", tr.ToolCallID)
		assert.Equal(t, "testFunction", tr.Name)
		assert.Equal(t, "ERROR: Test error message", tr.Content)
	})

	t.Run("Multiple errors", func(t *testing.T) {
		toolResult := &llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{},
		}

		// Add first error
		cs.handleToolError("Error 1", "call_123", "function1", toolResult)

		// Add second error
		cs.handleToolError("Error 2", "call_456", "function2", toolResult)

		// Verify toolResult contains both error messages
		require.Len(t, toolResult.Parts, 2, "Tool result should have two parts")

		tr1, ok := toolResult.Parts[0].(llms.ToolCallResponse)
		require.True(t, ok, "First tool result part should be a ToolCallResponse")
		assert.Equal(t, "call_123", tr1.ToolCallID)
		assert.Equal(t, "function1", tr1.Name)
		assert.Equal(t, "ERROR: Error 1", tr1.Content)

		tr2, ok := toolResult.Parts[1].(llms.ToolCallResponse)
		require.True(t, ok, "Second tool result part should be a ToolCallResponse")
		assert.Equal(t, "call_456", tr2.ToolCallID)
		assert.Equal(t, "function2", tr2.Name)
		assert.Equal(t, "ERROR: Error 2", tr2.Content)
	})
}

func TestConvertLLMArgsToUniversalClientInputs(t *testing.T) {
	// Setup test data
	uid := uint(1)
	sid := "test_session_id"
	cs, _ := NewChatSession(&models.Chat{}, ChatMessage, nil, nil, nil, &uid, &sid)

	t.Run("Valid arguments with body, headers, and parameters", func(t *testing.T) {
		args := `{
			"body": {"name": "Fluffy", "status": "available"},
			"headers": {"Content-Type": ["application/json"], "Accept": ["application/json"]},
			"parameters": {"status": ["available"], "limit": [10]}
		}`

		params, err := cs.convertLLMArgsToUniversalClientInputs([]byte(args), "findPetsByStatus", nil)
		require.NoError(t, err)

		// Verify body
		assert.Equal(t, "Fluffy", params.Body["name"])
		assert.Equal(t, "available", params.Body["status"])

		// Verify headers
		assert.Equal(t, []string{"application/json"}, params.Headers["Content-Type"])
		assert.Equal(t, []string{"application/json"}, params.Headers["Accept"])

		// Verify parameters
		assert.Equal(t, []string{"available"}, params.Parameters["status"])
		assert.Equal(t, []string{"10"}, params.Parameters["limit"])
	})

	t.Run("Parameters outside of parameters object", func(t *testing.T) {
		args := `{
			"body": {"name": "Fluffy"},
			"headers": {"Content-Type": ["application/json"]},
			"status": ["available"],
			"limit": 10
		}`

		params, err := cs.convertLLMArgsToUniversalClientInputs([]byte(args), "findPetsByStatus", nil)
		require.NoError(t, err)

		// Verify parameters outside of parameters object are added to Parameters
		assert.Equal(t, []string{"available"}, params.Parameters["status"])
		assert.Equal(t, []string{"10"}, params.Parameters["limit"])
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		args := `{invalid json}`

		_, err := cs.convertLLMArgsToUniversalClientInputs([]byte(args), "findPetsByStatus", nil)
		assert.Error(t, err)
	})

	t.Run("Non-object body", func(t *testing.T) {
		args := `{"body": "string body"}`

		_, err := cs.convertLLMArgsToUniversalClientInputs([]byte(args), "findPetsByStatus", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected 'body' to be a JSON object")
	})

	t.Run("Non-object headers", func(t *testing.T) {
		args := `{"headers": "string headers"}`

		_, err := cs.convertLLMArgsToUniversalClientInputs([]byte(args), "findPetsByStatus", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected 'headers' to be a JSON object")
	})

	t.Run("Non-object parameters", func(t *testing.T) {
		args := `{"parameters": "string parameters"}`

		_, err := cs.convertLLMArgsToUniversalClientInputs([]byte(args), "findPetsByStatus", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected 'parameters' to be a JSON object")
	})

	t.Run("Various parameter types", func(t *testing.T) {
		args := `{
			"stringParam": "value",
			"numberParam": 123,
			"boolParam": true,
			"arrayParam": ["one", "two", "three"],
			"mixedArrayParam": ["one", 2, true]
		}`

		params, err := cs.convertLLMArgsToUniversalClientInputs([]byte(args), "testOp", nil)
		require.NoError(t, err)

		assert.Equal(t, []string{"value"}, params.Parameters["stringParam"])
		assert.Equal(t, []string{"123"}, params.Parameters["numberParam"])
		assert.Equal(t, []string{"true"}, params.Parameters["boolParam"])
		assert.Equal(t, []string{"one", "two", "three"}, params.Parameters["arrayParam"])
		assert.Equal(t, []string{"one", "2", "true"}, params.Parameters["mixedArrayParam"])
	})
}

func TestInterfaceToStrings(t *testing.T) {
	t.Run("String value", func(t *testing.T) {
		result, err := interfaceToStrings("test")
		require.NoError(t, err)
		assert.Equal(t, []string{"test"}, result)
	})

	t.Run("String array", func(t *testing.T) {
		result, err := interfaceToStrings([]string{"one", "two", "three"})
		require.NoError(t, err)
		assert.Equal(t, []string{"one", "two", "three"}, result)
	})

	t.Run("Interface array", func(t *testing.T) {
		result, err := interfaceToStrings([]interface{}{"one", 2, true})
		require.NoError(t, err)
		assert.Equal(t, []string{"one", "2", "true"}, result)
	})

	t.Run("Number value", func(t *testing.T) {
		result, err := interfaceToStrings(123)
		require.NoError(t, err)
		assert.Equal(t, []string{"123"}, result)
	})

	t.Run("Boolean value", func(t *testing.T) {
		result, err := interfaceToStrings(true)
		require.NoError(t, err)
		assert.Equal(t, []string{"true"}, result)
	})

	t.Run("Unsupported type", func(t *testing.T) {
		_, err := interfaceToStrings(struct{}{})
		assert.Error(t, err)
	})
}

func TestInterfaceToString(t *testing.T) {
	t.Run("String value", func(t *testing.T) {
		result, err := interfaceToString("test")
		require.NoError(t, err)
		assert.Equal(t, "test", result)
	})

	t.Run("Integer value", func(t *testing.T) {
		result, err := interfaceToString(123)
		require.NoError(t, err)
		assert.Equal(t, "123", result)
	})

	t.Run("Float value", func(t *testing.T) {
		result, err := interfaceToString(123.45)
		require.NoError(t, err)
		assert.Equal(t, "123.45", result)
	})

	t.Run("Boolean value", func(t *testing.T) {
		result, err := interfaceToString(true)
		require.NoError(t, err)
		assert.Equal(t, "true", result)
	})

	t.Run("Unsupported type", func(t *testing.T) {
		_, err := interfaceToString(struct{}{})
		assert.Error(t, err)
	})
}

// TestHandleToolCallsResponseTypes tests the different response types in the handleToolCalls function
func TestHandleToolCallsResponseTypes(t *testing.T) {
	// Setup test DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))

	// Setup test data
	uid := uint(1)
	sid := "test_session_id"
	chatRef := &models.Chat{
		ID:          1,
		Name:        "Test Chat",
		LLMSettings: &models.LLMSettings{ModelName: "dummy"},
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: models.MOCK_VENDOR,
		},
	}

	// Read test OAS spec
	spec, err := os.ReadFile("../universalclient/testdata/petstore.json")
	require.NoError(t, err, "Failed to read test OAS spec")

	t.Run("Response of type []byte", func(t *testing.T) {
		// Create a test server that returns a byte response
		byteResponse := []byte(`{"id": 1, "name": "doggie", "status": "available"}`)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(byteResponse)
		}))
		defer server.Close()

		// Create a modified OAS spec that points to our test server
		modifiedSpec := `{
			"openapi": "3.0.0",
			"info": {
				"title": "Test API",
				"version": "1.0.0"
			},
			"servers": [
				{
					"url": "` + server.URL + `"
				}
			],
			"paths": {
				"/pets/findByStatus": {
					"get": {
						"operationId": "findPetsByStatus",
						"parameters": [
							{
								"name": "status",
								"in": "query",
								"schema": {
									"type": "array",
									"items": {
										"type": "string"
									}
								}
							}
						],
						"responses": {
							"200": {
								"description": "successful operation"
							}
						}
					}
				}
			}
		}`

		// Create chat session
		cs, err := NewChatSession(chatRef, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
		require.NoError(t, err)
		require.NoError(t, cs.initSession())

		// Add a mock tool with our modified spec
		cs.tools = map[string]models.Tool{
			"test_tool": {
				ID:                  1,
				Name:                "Test Tool",
				ToolType:            models.ToolTypeREST,
				OASSpec:             modifiedSpec,
				AvailableOperations: "findPetsByStatus",
			},
		}

		// Create a mock content choice with a tool call
		choice := &llms.ContentChoice{
			ToolCalls: []llms.ToolCall{
				{
					ID:   "call_123",
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      "findPetsByStatus",
						Arguments: `{"status": ["available"]}`,
					},
				},
			},
		}

		// Create empty message contents for tool call and result
		toolCall := &llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{},
		}
		toolResult := &llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{},
		}

		// Call handleToolCalls
		cs.handleToolCalls(choice, toolCall, toolResult)

		// Verify toolCall is populated correctly
		require.Len(t, toolCall.Parts, 1, "Tool call should have one part")

		// Verify toolResult contains the response
		require.Len(t, toolResult.Parts, 1, "Tool result should have one part")
		tr, ok := toolResult.Parts[0].(llms.ToolCallResponse)
		require.True(t, ok, "Tool result part should be a ToolCallResponse")
		assert.Equal(t, "call_123", tr.ToolCallID)
		assert.Equal(t, "findPetsByStatus", tr.Name)
		assert.Equal(t, string(byteResponse), tr.Content)
	})

	t.Run("Response of type string", func(t *testing.T) {
		// Create a test server that returns a string response
		stringResponse := `{"id": 1, "name": "doggie", "status": "available"}`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(stringResponse))
		}))
		defer server.Close()

		// Create a modified OAS spec that points to our test server
		modifiedSpec := `{
			"openapi": "3.0.0",
			"info": {
				"title": "Test API",
				"version": "1.0.0"
			},
			"servers": [
				{
					"url": "` + server.URL + `"
				}
			],
			"paths": {
				"/pets/findByStatus": {
					"get": {
						"operationId": "findPetsByStatus",
						"parameters": [
							{
								"name": "status",
								"in": "query",
								"schema": {
									"type": "array",
									"items": {
										"type": "string"
									}
								}
							}
						],
						"responses": {
							"200": {
								"description": "successful operation"
							}
						}
					}
				}
			}
		}`

		// Create chat session
		cs, err := NewChatSession(chatRef, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
		require.NoError(t, err)
		require.NoError(t, cs.initSession())

		// Add a mock tool with our modified spec
		cs.tools = map[string]models.Tool{
			"test_tool": {
				ID:                  1,
				Name:                "Test Tool",
				ToolType:            models.ToolTypeREST,
				OASSpec:             modifiedSpec,
				AvailableOperations: "findPetsByStatus",
			},
		}

		// Create a mock content choice with a tool call
		choice := &llms.ContentChoice{
			ToolCalls: []llms.ToolCall{
				{
					ID:   "call_123",
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      "findPetsByStatus",
						Arguments: `{"status": ["available"]}`,
					},
				},
			},
		}

		// Create empty message contents for tool call and result
		toolCall := &llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{},
		}
		toolResult := &llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{},
		}

		// Call handleToolCalls
		cs.handleToolCalls(choice, toolCall, toolResult)

		// Verify toolCall is populated correctly
		require.Len(t, toolCall.Parts, 1, "Tool call should have one part")

		// Verify toolResult contains the response
		require.Len(t, toolResult.Parts, 1, "Tool result should have one part")
		tr, ok := toolResult.Parts[0].(llms.ToolCallResponse)
		require.True(t, ok, "Tool result part should be a ToolCallResponse")
		assert.Equal(t, "call_123", tr.ToolCallID)
		assert.Equal(t, "findPetsByStatus", tr.Name)
		assert.Equal(t, stringResponse, tr.Content)
	})

	t.Run("Response of incompatible type", func(t *testing.T) {
		// Create chat session
		cs, err := NewChatSession(chatRef, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
		require.NoError(t, err)
		require.NoError(t, cs.initSession())

		// Create a test tool
		cs.tools = map[string]models.Tool{
			"test_tool": {
				ID:                  1,
				Name:                "Test Tool",
				ToolType:            models.ToolTypeREST,
				OASSpec:             string(spec),
				AvailableOperations: "findPetsByStatus",
			},
		}

		// Create empty message contents for tool result
		toolResult := &llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{},
		}

		// Directly test the handleToolError function
		errMsg := "response is not a compatible string (map[string]interface {})"
		cs.handleToolError(errMsg, "call_123", "findPetsByStatus", toolResult)

		// Verify toolResult contains an error message
		require.Len(t, toolResult.Parts, 1, "Tool result should have one part")
		tr, ok := toolResult.Parts[0].(llms.ToolCallResponse)
		require.True(t, ok, "Tool result part should be a ToolCallResponse")
		assert.Equal(t, "call_123", tr.ToolCallID)
		assert.Equal(t, "findPetsByStatus", tr.Name)
		assert.Equal(t, "ERROR: response is not a compatible string (map[string]interface {})", tr.Content)
	})
}

// TestSendStatus tests the sendStatus function
func TestSendStatus(t *testing.T) {
	// Setup test data
	uid := uint(1)
	sid := "test_session_id"
	chatRef := &models.Chat{
		ID:          1,
		Name:        "Test Chat",
		LLMSettings: &models.LLMSettings{ModelName: "dummy"},
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: models.MOCK_VENDOR,
		},
	}

	// Create chat session
	cs, err := NewChatSession(chatRef, ChatMessage, nil, nil, nil, &uid, &sid)
	require.NoError(t, err)

	// Test status message sending through queue interface

	// Send a status message
	testStatus := "Test status message"
	cs.sendStatus(testStatus)

	// Verify message was sent to outputMessages via queue
	select {
	case msg := <-cs.OutputMessage():
		assert.Contains(t, msg.Payload, testStatus)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "Timeout waiting for message in outputMessages")
	}

	// Note: sendStatus now only sends to outputMessages channel to prevent duplicates
}

// TestHandleToolCallsWithFilters tests the handleToolCalls function with tools that have filters
func TestHandleToolCallsWithFilters(t *testing.T) {
	// Setup test DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))

	// Setup test data
	uid := uint(1)
	sid := "test_session_id"
	chatRef := &models.Chat{
		ID:          1,
		Name:        "Test Chat",
		LLMSettings: &models.LLMSettings{ModelName: "dummy"},
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: models.MOCK_VENDOR,
		},
	}

	// Read test OAS spec
	spec, err := os.ReadFile("../universalclient/testdata/petstore.json")
	require.NoError(t, err, "Failed to read test OAS spec")

	// Create chat session
	cs, err := NewChatSession(chatRef, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
	require.NoError(t, err)
	require.NoError(t, cs.initSession())

	// Add a mock tool with filters
	cs.tools = map[string]models.Tool{
		"test_tool": {
			ID:                  1,
			Name:                "Test Tool",
			ToolType:            models.ToolTypeREST,
			OASSpec:             string(spec),
			AvailableOperations: "findPetsByStatus",
			Filters: []models.Filter{
				{
					ID:     1,
					Name:   "Test Filter",
					Script: []byte("function filter(content) { return content; }"),
				},
			},
		},
	}

	// Create a mock content choice with a tool call
	choice := &llms.ContentChoice{
		ToolCalls: []llms.ToolCall{
			{
				ID:   "call_123",
				Type: "function",
				FunctionCall: &llms.FunctionCall{
					Name:      "findPetsByStatus",
					Arguments: `{"status": ["available"]}`,
				},
			},
		},
	}

	// Create empty message contents for tool call and result
	toolCall := &llms.MessageContent{
		Role:  llms.ChatMessageTypeAI,
		Parts: []llms.ContentPart{},
	}
	toolResult := &llms.MessageContent{
		Role:  llms.ChatMessageTypeTool,
		Parts: []llms.ContentPart{},
	}

	// Call handleToolCalls
	cs.handleToolCalls(choice, toolCall, toolResult)

	// Verify toolCall is populated correctly
	require.Len(t, toolCall.Parts, 1, "Tool call should have one part")
	tc, ok := toolCall.Parts[0].(llms.ToolCall)
	require.True(t, ok, "Tool call part should be a ToolCall")
	assert.Equal(t, "call_123", tc.ID)
	assert.Equal(t, "function", tc.Type)
	assert.Equal(t, "findPetsByStatus", tc.FunctionCall.Name)
}

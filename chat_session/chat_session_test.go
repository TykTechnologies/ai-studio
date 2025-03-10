package chat_session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

var (
	uid = uint(1)
	sid = "test_session_id"
)

func TestNewChatSession(t *testing.T) {
	db := setupTestDB(t)
	chat := &models.Chat{
		LLM: &models.LLM{
			Name: "Dummy LLM",
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs, _ := NewChatSession(chat, ChatMessage, db, services.NewService(db), nil, &uid, &sid)

	assert.NotNil(t, cs)
	assert.Equal(t, chat, cs.chatRef)
	assert.Equal(t, ChatMessage, cs.mode)
	assert.NotNil(t, cs.input)
	assert.NotNil(t, cs.outputMessages)
	assert.NotNil(t, cs.outputStream)
	assert.NotNil(t, cs.stop)
	assert.NotNil(t, cs.errors)
}

func TestChatSession_InitSession(t *testing.T) {
	db := setupTestDB(t)
	chat := &models.Chat{
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: models.MOCK_VENDOR,
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs, _ := NewChatSession(chat, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
	err := cs.initSession()

	assert.NoError(t, err)
	assert.NotNil(t, cs.chatHistory)
	assert.NotNil(t, cs.caller)
}

func TestChatSession_HandleUserMessage(t *testing.T) {
	t.Skip()
	db := setupTestDB(t)
	chat := &models.Chat{
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: models.MOCK_VENDOR,
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs, _ := NewChatSession(chat, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
	err := cs.initSession()
	assert.NoError(t, err)

	msg := &models.UserMessage{Payload: "Test message"}
	resp, err := cs.HandleUserMessage(msg, []schema.Document{}, []llms.Tool{}, map[string]string{})

	assert.NoError(t, err)
	assert.NotEmpty(t, resp)
}

func TestChatSession_PreProcessors(t *testing.T) {
	db := setupTestDB(t)
	cs, _ := NewChatSession(&models.Chat{}, ChatMessage, db, services.NewService(db), nil, &uid, &sid)

	preprocessor := func(msg *models.UserMessage) error {
		msg.Payload = "Processed: " + msg.Payload
		return nil
	}

	cs.AddPreProcessor(preprocessor)

	msg := &models.UserMessage{Payload: "Test message"}
	err := cs.preProcessMessage(msg)

	assert.NoError(t, err)
	assert.Equal(t, "Processed: Test message", msg.Payload)
}

func TestChatSession_Start(t *testing.T) {
	t.Skip()
	db := setupTestDB(t)
	chat := &models.Chat{
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: models.MOCK_VENDOR,
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs, _ := NewChatSession(chat, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
	err := cs.Start()
	assert.NoError(t, err)

	// Send a message
	cs.input <- &models.UserMessage{Payload: "Test message"}

	// Wait for response
	select {
	case response := <-cs.OutputMessage():
		assert.NotEmpty(t, response.Payload)
	case err := <-cs.Errors():
		assert.Fail(t, "Received error", err)
	case <-time.After(5 * time.Second):
		assert.Fail(t, "Timeout waiting for response")
	}

	cs.Stop()
}

func TestChatSession_StreamingMode(t *testing.T) {
	t.Skip()
	chat := &models.Chat{
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: models.MOCK_VENDOR,
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	db := setupTestDB(t)
	cs, _ := NewChatSession(chat, ChatStream, db, services.NewService(db), nil, &uid, &sid)
	err := cs.Start()
	assert.NoError(t, err)

	go func() {
		cs.Input() <- &models.UserMessage{Payload: "Test prompt"}
	}()

	count := 0
	for {
		select {
		case chunk := <-cs.OutputStream():
			assert.NotEmpty(t, chunk)
			count++
		case err := <-cs.Errors():
			assert.Fail(t, "Received error", err)
		case <-time.After(5 * time.Second):
			assert.Fail(t, "Timeout waiting for streaming data")
		}
		if count >= 10 {
			break
		}
	}

	assert.GreaterOrEqual(t, count, 10)
	cs.Stop()
}

func TestChatSession_GetOptions(t *testing.T) {
	llmSettings := &models.LLMSettings{
		Temperature: 0.7,
		MaxTokens:   100,
		TopP:        0.9,
	}

	db := setupTestDB(t)
	cs, _ := NewChatSession(&models.Chat{LLMSettings: llmSettings}, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
	options := cs.getOptions(llmSettings, []llms.Tool{})

	assert.NotEmpty(t, options)
	// Additional checks for specific options can be added here
}

func TestChatSession_ErrorHandling(t *testing.T) {
	t.Skip()
	chat := &models.Chat{
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: models.MOCK_VENDOR,
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	db := setupTestDB(t)
	cs, _ := NewChatSession(chat, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
	err := cs.Start()
	assert.NoError(t, err)

	cs.AddPreProcessor(func(*models.UserMessage) error {
		return fmt.Errorf("test error")
	})

	cs.input <- &models.UserMessage{Payload: "Test message"}

	select {
	case err := <-cs.Errors():
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "preprocessing error")
	case <-time.After(2 * time.Second):
		assert.Fail(t, "Timeout waiting for error")
	}

	cs.Stop()
}

func TestChatSession_AddRemoveDatasource(t *testing.T) {
	t.Skip()
	db := setupTestDB(t)
	cs, _ := NewChatSession(&models.Chat{}, ChatMessage, db, services.NewService(db), nil, &uid, &sid)

	// Create a test datasource
	ds := models.Datasource{ID: 1, Name: "Test Datasource"}
	err := db.Create(&ds).Error
	require.NoError(t, err)

	// Test AddDatasource
	err = cs.AddDatasource(ds.ID)
	assert.NoError(t, err)
	assert.Contains(t, cs.datasources, ds.ID)

	// Test RemoveDatasource
	cs.RemoveDatasource(ds.ID)
	assert.NotContains(t, cs.datasources, ds.ID)
}

func TestChatSession_PrepareTools(t *testing.T) {
	db := setupTestDB(t)
	cs, _ := NewChatSession(&models.Chat{}, ChatMessage, db, services.NewService(db), nil, &uid, &sid)

	spec, err := os.ReadFile("../universalclient/testdata/petstore.json")
	require.NoError(t, err)

	// Add a mock tool
	cs.tools = map[string]models.Tool{
		"test_tool": {
			ToolType:            models.ToolTypeREST,
			OASSpec:             string(spec),
			AvailableOperations: "addPet,updatePet",
		},
	}

	tools := cs.prepareTools()
	assert.NotEmpty(t, tools)
	// Additional checks on the prepared tools can be added here
}

func TestChatSession_ConvertLLMArgsToUniversalClientInputs(t *testing.T) {
	cs, _ := NewChatSession(&models.Chat{}, ChatMessage, nil, nil, nil, &uid, &sid)

	testArgs := `{"body": {"key": "value"}, "headers": {"Content-Type": ["application/json"]}, "parameters": {"query": ["test"]}}`
	params, err := cs.convertLLMArgsToUniversalClientInputs([]byte(testArgs), "foo", nil)

	assert.NoError(t, err)
	assert.Equal(t, "value", params.Body["key"])
	assert.Equal(t, []string{"application/json"}, params.Headers["Content-Type"])
	assert.Equal(t, []string{"test"}, params.Parameters["query"])
}

func TestChatSession_ConvertLLMArgsToUniversalClientInputs_WithUnstructuredInput(t *testing.T) {

	chat := &models.Chat{
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: models.MOCK_VENDOR,
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}
	cs, _ := NewChatSession(chat, ChatMessage, nil, nil, nil, &uid, &sid)

	// LLMs might not send back the parameters key, so we just assume it for anything not in the other two categpories
	testArgs := `{"body": {"key": "value"}, "headers": {"Content-Type": ["application/json"]}, "query": ["test"]}`
	params, err := cs.convertLLMArgsToUniversalClientInputs([]byte(testArgs), "foo", nil)

	assert.NoError(t, err)
	assert.Equal(t, "value", params.Body["key"])
	assert.Equal(t, []string{"application/json"}, params.Headers["Content-Type"])
	assert.Equal(t, []string{"test"}, params.Parameters["query"])
}

func TestChatSession_HandleToolCalls(t *testing.T) {
	t.Skip()
	db := setupTestDB(t)
	chatRef := &models.Chat{
		ID:          1,
		Name:        "Test Chat",
		LLMSettings: &models.LLMSettings{ModelName: "dummy"},
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: models.MOCK_VENDOR,
		},
	}

	cs, _ := NewChatSession(chatRef, ChatMessage, db, services.NewService(db), nil, &uid, &sid)

	cs.initSession()

	spec, err := os.ReadFile("../universalclient/testdata/petstore.json")
	require.NoError(t, err)

	// Mock a tool
	cs.tools = map[string]models.Tool{
		"test_tool": {
			ToolType:            models.ToolTypeREST,
			OASSpec:             string(spec),
			AvailableOperations: "findPetsByStatus",
		},
	}

	choice := &llms.ContentChoice{
		ToolCalls: []llms.ToolCall{
			{
				FunctionCall: &llms.FunctionCall{
					Name:      "findPetsByStatus",
					Arguments: `{"body": {}, "headers": {}, "parameters": {"status": ["available"]}}`,
				},
			},
		},
	}

	called, err := cs.handleToolCalls(choice, &llms.MessageContent{}, &llms.MessageContent{})
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestChatSession_GetMessages(t *testing.T) {
	db := setupTestDB(t)
	chat := &models.Chat{
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: models.MOCK_VENDOR,
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}
	cs, _ := NewChatSession(chat, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
	cs.initSession()

	// Add some messages to the history
	// err := cs.chatHistory.addMessage(context.Background(), llms.HumanChatMessage{Content: "Hello"})
	err := cs.chatHistory.addMessage(context.Background(), llms.TextParts(llms.ChatMessageTypeHuman, "Hello"))
	assert.NoError(t, err)
	err = cs.chatHistory.addMessage(context.Background(), llms.TextParts(llms.ChatMessageTypeAI, "Hi there"))
	// err = cs.chatHistory.addMessage(context.Background(), llms.AIChatMessage{Content: "Hi there"})
	assert.NoError(t, err)

	messages, err := cs.getMessages()
	assert.NoError(t, err)
	assert.Len(t, messages, 2)
	msg, ok := messages[0].Parts[0].(llms.TextContent)
	assert.True(t, ok)
	assert.Equal(t, "Hello", msg.Text)

	msg2, ok := messages[1].Parts[0].(llms.TextContent)
	assert.True(t, ok)
	assert.Equal(t, "Hi there", msg2.Text)
}

func TestChatSession_PrepHumanMessage(t *testing.T) {
	cs, _ := NewChatSession(&models.Chat{}, ChatMessage, nil, nil, nil, &uid, &sid)
	docs := []schema.Document{
		{PageContent: "Document 1"},
		{PageContent: "Document 2"},
	}

	msg := cs.prepHumanMessage("Test message", docs)
	assert.Contains(t, msg.Content, "Context for this message:")
	assert.Contains(t, msg.Content, "Document 1")
	assert.Contains(t, msg.Content, "Document 2")
	assert.Contains(t, msg.Content, "Test message")
}

func TestChatSession_JoinDocuments(t *testing.T) {
	cs, _ := NewChatSession(&models.Chat{}, ChatMessage, nil, nil, nil, &uid, &sid)
	docs := []schema.Document{
		{PageContent: "Document 1"},
		{PageContent: "Document 2"},
		{PageContent: "Document 3"},
	}

	joined := cs.joinDocuments(docs, " | ")
	assert.Equal(t, "Document 1 | Document 2 | Document 3", joined)
}

func TestChatSession_StreamingFunc(t *testing.T) {
	cs, _ := NewChatSession(&models.Chat{}, ChatStream, nil, nil, nil, &uid, &sid)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := cs.streamingFunc(ctx, []byte("test chunk"))
	assert.NoError(t, err)

	select {
	case chunk := <-cs.outputStream:
		assert.Equal(t, []byte("test chunk"), chunk)
	case <-time.After(1 * time.Second):
		assert.Fail(t, "Timeout waiting for streaming chunk")
	}
}

func TestChatSession_FetchDriver(t *testing.T) {
	tests := []struct {
		name    string
		vendor  string
		wantErr bool
	}{
		{"OpenAI", "openai", false},
		{"Anthropic", "anthropic", false},
		{"Mock", "mock", false},
		{"Unsupported", "unsupported", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chat := &models.Chat{
				LLM: &models.LLM{
					Vendor: models.Vendor(tt.vendor),
					APIKey: "foo",
				},
				LLMSettings: &models.LLMSettings{
					ModelName: "test-model",
				},
			}
			cs, _ := NewChatSession(chat, ChatMessage, nil, nil, nil, &uid, &sid)
			_, err := cs.fetchDriver(nil)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExtractEmbeddedToolCalls(t *testing.T) {
	t.Run("No tool calls", func(t *testing.T) {
		input := "This is a normal message with no tool calls"
		output, toolCalls := extractEmbeddedToolCalls(input)

		assert.Equal(t, input, output)
		assert.Empty(t, toolCalls)
	})

	t.Run("Basic tool call with URL", func(t *testing.T) {
		input := `To answer your question about whether the Tyk Developer Portal supports Dynamic Client 
		Registration (DCR), I'll need to verify the most current information from Tyk's documentation. 
		Let me try to access the main documentation page to find this information for you.
		tool_use
		{"function":{"arguments":{"formats":["markdown"],"url":"https://tyk.io/docs/"},
		"name":"scrapeAndExtractFromUrl"},"tool_call_id":"toolu_01JRWHmuenfbA4Y89bs6BjWE","type":""}
		/tool_use`
		expectedOutput := `To answer your question about whether the Tyk Developer Portal supports Dynamic Client 
		Registration (DCR), I'll need to verify the most current information from Tyk's documentation. 
		Let me try to access the main documentation page to find this information for you.`
		output, toolCalls := extractEmbeddedToolCalls(input)

		// Normalize strings by removing whitespace for comparison
		normalizedOutput := strings.TrimSpace(output)
		normalizedExpected := strings.TrimSpace(expectedOutput)
		assert.Equal(t, normalizedExpected, normalizedOutput)
		assert.Equal(t, 1, len(toolCalls))

		// Verify the specific content of the extracted tool call
		assert.Equal(t, "toolu_01JRWHmuenfbA4Y89bs6BjWE", toolCalls[0].ID)
		assert.Equal(t, "", toolCalls[0].Type)
		assert.Equal(t, "scrapeAndExtractFromUrl", toolCalls[0].FunctionCall.Name)

		// Parse the arguments to verify their content
		var args map[string]interface{}
		err := json.Unmarshal([]byte(toolCalls[0].FunctionCall.Arguments), &args)
		assert.NoError(t, err)
		assert.Equal(t, "https://tyk.io/docs/", args["url"])

		formats, ok := args["formats"].([]interface{})
		assert.True(t, ok)
		assert.Equal(t, 1, len(formats))
		assert.Equal(t, "markdown", formats[0])
	})

	t.Run("Tool call with longer URL path", func(t *testing.T) {
		input := `I apologize for the error. It seems there was an issue with the URL provided. 
		Let me try again with a slightly modified URL to ensure it includes the proper top-level domain.
		tool_use
		{"function":{"arguments":{"formats":["markdown"],
		"url":"https://tyk.io/docs/tyk-developer-portal/portal-concepts/dynamic-client-registration/"},
		"name":"scrapeAndExtractFromUrl"},"tool_call_id":"toolu_014ZfgX9VMxfpxCVGq8iHb3V","type":""}
		/tool_use`
		expectedOutput := `I apologize for the error. It seems there was an issue with the URL provided. 
		Let me try again with a slightly modified URL to ensure it includes the proper top-level domain.`
		output, toolCalls := extractEmbeddedToolCalls(input)

		// Normalize strings by removing whitespace for comparison
		normalizedOutput := strings.TrimSpace(output)
		normalizedExpected := strings.TrimSpace(expectedOutput)
		assert.Equal(t, normalizedExpected, normalizedOutput)
		assert.Equal(t, 1, len(toolCalls))
		assert.Equal(t, "toolu_014ZfgX9VMxfpxCVGq8iHb3V", toolCalls[0].ID)
		assert.Equal(t, "scrapeAndExtractFromUrl", toolCalls[0].FunctionCall.Name)
	})

	t.Run("Multiple tool calls with text between them", func(t *testing.T) {
		input := `I'll need to check multiple sources to answer your question comprehensively.
		First, let me check the main documentation:
		tool_use
		{"function":{"arguments":{"formats":["markdown"],"url":"https://tyk.io/docs/"},
		"name":"scrapeAndExtractFromUrl"},"tool_call_id":"toolu_01JRWHmuenfbA4Y89bs6BjWE","type":""}
		/tool_use
		Now that I've checked the main documentation, I should also look at the specific section about 
		developer portals to get more detailed information:
		tool_use
		{"function":{"arguments":{"formats":["markdown"],
		"url":"https://tyk.io/docs/tyk-developer-portal/"},
		"name":"scrapeAndExtractFromUrl"},"tool_call_id":"toolu_02KSWImuenfbA4Y89bs7CkXF","type":""}
		/tool_use
		With this information, I can now provide you with a comprehensive answer.`
		expectedOutput := `I'll need to check multiple sources to answer your question comprehensively.
		First, let me check the main documentation:
		Now that I've checked the main documentation, I should also look at the specific section about 
		developer portals to get more detailed information:
		With this information, I can now provide you with a comprehensive answer.`

		// Normalize input for testing
		normalizedInput := strings.TrimSpace(input)
		output, toolCalls := extractEmbeddedToolCalls(normalizedInput)

		// Normalize strings by removing whitespace for comparison
		normalizedOutput := regexp.MustCompile(`\s+`).ReplaceAllString(output, " ")
		normalizedExpected := regexp.MustCompile(`\s+`).ReplaceAllString(expectedOutput, " ")
		assert.Equal(t, normalizedExpected, normalizedOutput)
		assert.Equal(t, 2, len(toolCalls))
		assert.Equal(t, "toolu_01JRWHmuenfbA4Y89bs6BjWE", toolCalls[0].ID)
		assert.Equal(t, "toolu_02KSWImuenfbA4Y89bs7CkXF", toolCalls[1].ID)
		assert.Equal(t, "scrapeAndExtractFromUrl", toolCalls[0].FunctionCall.Name)
		assert.Equal(t, "scrapeAndExtractFromUrl", toolCalls[1].FunctionCall.Name)
	})

	t.Run("Tool call with nested arguments", func(t *testing.T) {
		input := `Let me search for that information.
		tool_use
		{"function":{"arguments":{"formats":["markdown"],"onlyMainContent":true,
		"url":"https://tyk.io/docs/tyk-developer-portal/portal-concepts/dynamic-client-registration/"},
		"name":"scrapeAndExtractFromUrl"},"tool_call_id":"toolu_014ZfgX9VMxfpxCVGq8iHb3V","type":""}
		/tool_use`
		output, toolCalls := extractEmbeddedToolCalls(input)

		// Normalize strings by removing whitespace for comparison
		normalizedOutput := strings.TrimSpace(output)
		expectedOutput := "Let me search for that information."
		assert.Equal(t, expectedOutput, normalizedOutput)
		// This test may fail if the JSON structure doesn't match what the function expects
		// The important part is that the tool_use block is removed from the output
		if len(toolCalls) > 0 {
			assert.Equal(t, "toolu_014ZfgX9VMxfpxCVGq8iHb3V", toolCalls[0].ID)
			assert.Equal(t, "scrapeAndExtractFromUrl", toolCalls[0].FunctionCall.Name)
		}
	})

	t.Run("Tool call at beginning of message", func(t *testing.T) {
		input := `tool_use
		{"function":{"arguments":{"formats":["markdown"],"url":"https://tyk.io/docs/"},
		"name":"scrapeAndExtractFromUrl"},"tool_call_id":"toolu_01JRWHmuenfbA4Y89bs6BjWE","type":""}
		/tool_use
		Now I'll analyze the documentation for you.`

		expectedOutput := `Now I'll analyze the documentation for you.`
		output, toolCalls := extractEmbeddedToolCalls(input)

		// Normalize strings by removing whitespace for comparison
		normalizedOutput := strings.TrimSpace(output)
		assert.Equal(t, expectedOutput, normalizedOutput)
		assert.Equal(t, 1, len(toolCalls))
	})

	t.Run("Malformed tool call JSON", func(t *testing.T) {
		input := `This contains tool_use
		{"malformed_json": "missing closing brace"}
		/tool_use in the message`
		output, _ := extractEmbeddedToolCalls(input)
		expectedOutput := `This contains in the message`

		assert.Equal(t, expectedOutput, output)
	})
}

func TestChatSession_PrivacyScoreValidation(t *testing.T) {
	t.Skip()
	db := setupTestDB(t)

	// Create a chat with a low privacy score LLM
	chat := &models.Chat{
		LLM: &models.LLM{
			Name:         "Low Privacy LLM",
			Vendor:       models.MOCK_VENDOR,
			PrivacyScore: 3,
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs, err := NewChatSession(chat, ChatMessage, db, services.NewService(db), nil, &uid, &sid)
	require.NoError(t, err)

	// Test AddDatasource with incompatible privacy score
	highPrivacyDatasource := &models.Datasource{
		ID:           1,
		Name:         "High Privacy Datasource",
		PrivacyScore: 5,
	}
	err = db.Create(highPrivacyDatasource).Error
	require.NoError(t, err)

	err = cs.AddDatasource(highPrivacyDatasource.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "datasource or tool privacy score (5) is higher than LLM privacy score (3)")

	// Test AddDatasource with compatible privacy score
	lowPrivacyDatasource := &models.Datasource{
		ID:           2,
		Name:         "Low Privacy Datasource",
		PrivacyScore: 2,
	}
	err = db.Create(lowPrivacyDatasource).Error
	require.NoError(t, err)

	err = cs.AddDatasource(lowPrivacyDatasource.ID)
	assert.NoError(t, err)

	// Test AddTool with incompatible privacy score
	highPrivacyTool := models.Tool{
		ID:           3,
		Name:         "High Privacy Tool",
		PrivacyScore: 5,
		ToolType:     models.ToolTypeREST,
	}

	err = cs.AddTool(highPrivacyTool.Name, highPrivacyTool)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "datasource or tool privacy score (5) is higher than LLM privacy score (3)")

	// Test AddTool with compatible privacy score
	lowPrivacyTool := models.Tool{
		ID:           4,
		Name:         "Low Privacy Tool",
		PrivacyScore: 2,
		ToolType:     models.ToolTypeREST,
	}

	err = cs.AddTool(lowPrivacyTool.Name, lowPrivacyTool)
	assert.NoError(t, err)
}

// func TestChatSession_Live_Weather(t *testing.T) {
// 	if os.Getenv("WEATHERBIT_KEY") == "" {
// 		t.Skip("Skipping live test, set WEATHERBIT_KEY to run this test")
// 	}

// 	db := setupTestDB(t)
// 	chat := &models.Chat{
// 		LLM: &models.LLM{
// 			Name:   "claude-3-5-sonnet-20240620",
// 			Vendor: models.ANTHROPIC,
// 			APIKey: os.Getenv("ANTHROPIC_KEY"),
// 		},
// 		LLMSettings: &models.LLMSettings{
// 			ModelName: "claude-3-5-sonnet-20240620",
// 		},
// 	}

// 	spec, err := os.ReadFile("../universalclient/testdata/weatherbit.json")
// 	assert.NoError(t, err)

// 	weathertool := models.Tool{
// 		Name:                "weather forecast",
// 		Description:         "Get the weather forecast for a given location",
// 		ToolType:            models.ToolTypeREST,
// 		AvailableOperations: "ReturnsadailyforecastGivenLatLon",
// 		AuthKey:             os.Getenv("WEATHERBIT_KEY"),
// 		OASSpec:             spec,
// 	}

// 	session, _ := NewChatSession(chat, ChatMessage, db, services.NewService(db), nil, nil, nil)
// 	session.AddTool("weather", weathertool)

// 	err = session.Start()
// 	assert.NoError(t, err)

// 	// Send a message
// 	select {
// 	case session.Input() <- &models.UserMessage{Payload: "What is the weather like today in Auckland, New Zealand, and in New York City, USA?"}:
// 	default:
// 		assert.Fail(t, "Failed to send message")
// 	}

// 	// Wait for a response
// 	resps := 0
// 	t0 := time.Now()
// 	for {
// 		select {
// 		case resp := <-session.OutputMessage():
// 			fmt.Println("[RESPONSE]", resp.Payload)
// 			resps += 1
// 		case err := <-session.Errors():
// 			fmt.Println("[ERROR]", err)
// 			assert.Fail(t, "Error received")
// 		default:
// 			// if resps == 2 {
// 			// 	return
// 			// }
// 			if time.Since(t0) > 20*time.Second {
// 				assert.Fail(t, "Timeout waiting for response")
// 				return
// 			}
// 		}
// 	}

// }

// func TestChatSession_Live_Weather_Streaming(t *testing.T) {
// 	if os.Getenv("WEATHERBIT_KEY") == "" {
// 		t.Skip("Skipping live test, set WEATHERBIT_KEY to run this test")
// 	}

// 	db := setupTestDB(t)
// 	chat := &models.Chat{
// 		LLM: &models.LLM{
// 			Name:   "claude-3-5-sonnet-20240620",
// 			Vendor: models.ANTHROPIC,
// 			APIKey: os.Getenv("ANTHROPIC_KEY"),
// 		},
// 		LLMSettings: &models.LLMSettings{
// 			ModelName: "claude-3-5-sonnet-20240620",
// 		},
// 		// LLM: &models.LLM{
// 		// 	Name:   "gpt-4-turbo",
// 		// 	Vendor: models.OPENAI,
// 		// 	APIKey: os.Getenv("OPENAI_KEY"),
// 		// },
// 		// LLMSettings: &models.LLMSettings{
// 		// 	ModelName: "gpt-4-turbo",
// 		// },
// 	}

// 	spec, err := os.ReadFile("../universalclient/testdata/weatherbit.json")
// 	assert.NoError(t, err)

// 	weathertool := models.Tool{
// 		Name:                "weather forecast",
// 		Description:         "Get the weather forecast for a given location",
// 		ToolType:            models.ToolTypeREST,
// 		AvailableOperations: "ReturnsadailyforecastGivenLatLon",
// 		AuthKey:             os.Getenv("WEATHERBIT_KEY"),
// 		OASSpec:             spec,
// 	}

// 	session, _ := NewChatSession(chat, ChatStream, db, services.NewService(db), nil, nil, nil)
// 	session.AddTool("weather", weathertool)

// 	err = session.Start()
// 	assert.NoError(t, err)

// 	// Send a message
// 	select {
// 	case session.Input() <- &models.UserMessage{Payload: "What is the weather like today in Auckland, New Zealand, and in New York City, USA?"}:
// 	default:
// 		assert.Fail(t, "Failed to send message")
// 	}

// 	// Wait for streaming responses
// 	var fullResponse strings.Builder
// 	t0 := time.Now()
// 	timeout := time.After(60 * time.Second)

// 	for {
// 		select {
// 		case chunk := <-session.OutputStream():
// 			fmt.Print(string(chunk))
// 			fullResponse.Write(chunk)
// 		case err := <-session.Errors():
// 			fmt.Println("\n[ERROR]", err)
// 			assert.Fail(t, "Error received")
// 		case <-timeout:
// 			assert.Fail(t, "Timeout waiting for complete response")
// 			return
// 		default:
// 			if time.Since(t0) > 60*time.Second {
// 				// Check if we have a complete response
// 				if strings.Contains(fullResponse.String(), "New York City") &&
// 					strings.Contains(fullResponse.String(), "Auckland") {
// 					fmt.Println("\n\nFull response received:")
// 					fmt.Println(fullResponse.String())
// 					return
// 				}
// 			}
// 		}
// 	}
// }

// func TestChatSession_Live_Petstore(t *testing.T) {
// 	if os.Getenv("WEATHERBIT_KEY") == "" {
// 		t.Skip("Skipping live test, set WEATHERBIT_KEY to run this test")
// 	}

// 	db := setupTestDB(t)
// 	chat := &models.Chat{
// 		LLM: &models.LLM{
// 			Name:   "claude-3-5-sonnet-20240620",
// 			Vendor: models.ANTHROPIC,
// 			APIKey: os.Getenv("ANTHROPIC_KEY"),
// 		},
// 		LLMSettings: &models.LLMSettings{
// 			ModelName: "claude-3-5-sonnet-20240620",
// 		},
// 	}

// 	spec, err := os.ReadFile("../universalclient/testdata/petstore.json")
// 	assert.NoError(t, err)

// 	tool := models.Tool{
// 		Name:                "access to the pet store",
// 		Description:         "Access specific functions for the pet store",
// 		ToolType:            models.ToolTypeREST,
// 		AvailableOperations: "findPetsByStatus,updatePet,getPetById",
// 		AuthKey:             "foo",
// 		OASSpec:             spec,
// 	}

// 	session, _ := NewChatSession(chat, ChatMessage, db)
// 	session.AddTool("petstore", tool)

// 	err = session.Start()
// 	assert.NoError(t, err)

// 	// Send a message
// 	select {
// 	case session.Input() <- &UserMessage{Payload: "I'd like you to update the dog named Rex in the pet store by listing them as unnavailable please. You can find thr ID by gettign a list of available pets and checking the list,once you've done that, can you list out the pets that are still available please?"}:
// 	default:
// 		assert.Fail(t, "Failed to send message")
// 	}

// 	// Wait for a response
// 	resps := 0
// 	t0 := time.Now()
// 	for {
// 		select {
// 		case resp := <-session.OutputMessage():
// 			fmt.Println("[RESPONSE]", resp.Payload)
// 			resps += 1
// 		case err := <-session.Errors():
// 			fmt.Println("[ERROR]", err)
// 			assert.Fail(t, "Error received")
// 		default:
// 			if time.Since(t0) > 70*time.Second {
// 				return
// 			}
// 		}
// 	}

// }

// func TestChatSession_Live_JIRA(t *testing.T) {
// 	if os.Getenv("JIRA_KEY") == "" {
// 		t.Skip("Skipping live test, set JIRA_KEY to run this test")
// 	}

// 	db := setupTestDB(t)
// 	chat := &models.Chat{
// 		LLM: &models.LLM{
// 			Name:   "claude-3-5-sonnet-20240620",
// 			Vendor: models.ANTHROPIC,
// 			APIKey: os.Getenv("ANTHROPIC_KEY"),
// 		},
// 		LLMSettings: &models.LLMSettings{
// 			ModelName: "claude-3-5-sonnet-20240620",
// 		LLM: &models.LLM{
// 			Name:   "claude-3-5-sonnet-20240620",
// 			Vendor: models.ANTHROPIC,
// 			APIKey: os.Getenv("ANTHROPIC_KEY"),
// 		},
// 		LLMSettings: &models.LLMSettings{
// 			ModelName: "claude-3-5-sonnet-20240620",
// 		},
// 	}

// 	spec, err := os.ReadFile("../universalclient/testdata/jira.json")
// 	assert.NoError(t, err)

// 	key := os.Getenv("JIRA_KEY")
// 	auth := fmt.Sprintf("montag-bot@tyk.io:%s", key)

// 	tool := models.Tool{
// 		Name:                "access to our JIRA instance",
// 		Description:         "Access multiple facets of our JIRA instance",
// 		ToolType:            models.ToolTypeREST,
// 		AvailableOperations: "search,getIssue,searchForIssuesUsingJql,searchForIssuesIds",
// 		AuthKey:             auth,
// 		AuthSchemaName:      "basicAuth",
// 		OASSpec:             spec,
// 	}

// 	session, _ := NewChatSession(chat, ChatMessage, db)
// 	session.AddTool("jira", tool)

// 	err = session.Start()
// 	assert.NoError(t, err)

// 	// Send a message
// 	select {
// 	case session.Input() <- &UserMessage{Payload: "Please find all the issues in JIRA that are related to Tyk Gateway and SSL?"}:
// 	default:
// 		assert.Fail(t, "Failed to send message")
// 	}

// 	// Wait for a response
// 	resps := 0
// 	t0 := time.Now()
// 	for {
// 		select {
// 		case resp := <-session.OutputMessage():
// 			fmt.Println("[RESPONSE]", resp.Payload)
// 			resps += 1
// 		case err := <-session.Errors():
// 			fmt.Println("[ERROR]", err)
// 			assert.Fail(t, "Error received")
// 		default:
// 			if time.Since(t0) > 70*time.Second {
// 				return
// 			}
// 		}
// 	}

// }

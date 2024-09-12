package chat_session

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
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

	cs := NewChatSession(chat, ChatMessage, db)

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
			Vendor: "mock",
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs := NewChatSession(chat, ChatMessage, db)
	err := cs.initSession()

	assert.NoError(t, err)
	assert.NotNil(t, cs.chatHistory)
	assert.NotNil(t, cs.caller)
}

func TestChatSession_HandleUserMessage(t *testing.T) {
	db := setupTestDB(t)
	chat := &models.Chat{
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: "mock",
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs := NewChatSession(chat, ChatMessage, db)
	err := cs.initSession()
	assert.NoError(t, err)

	msg := &UserMessage{Payload: "Test message"}
	resp, err := cs.HandleUserMessage(msg, []schema.Document{}, []llms.Tool{})

	assert.NoError(t, err)
	assert.NotEmpty(t, resp)
}

func TestChatSession_PreProcessors(t *testing.T) {
	db := setupTestDB(t)
	cs := NewChatSession(&models.Chat{}, ChatMessage, db)

	preprocessor := func(msg *UserMessage) error {
		msg.Payload = "Processed: " + msg.Payload
		return nil
	}

	cs.AddPreProcessor(preprocessor)

	msg := &UserMessage{Payload: "Test message"}
	err := cs.preProcessMessage(msg)

	assert.NoError(t, err)
	assert.Equal(t, "Processed: Test message", msg.Payload)
}

func TestChatSession_Start(t *testing.T) {
	db := setupTestDB(t)
	chat := &models.Chat{
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: "mock",
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs := NewChatSession(chat, ChatMessage, db)
	err := cs.Start()
	assert.NoError(t, err)

	// Send a message
	cs.input <- &UserMessage{Payload: "Test message"}

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
	chat := &models.Chat{
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: "mock",
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	db := setupTestDB(t)
	cs := NewChatSession(chat, ChatStream, db)
	err := cs.Start()
	assert.NoError(t, err)

	go func() {
		cs.Input() <- &UserMessage{Payload: "Test prompt"}
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
	cs := NewChatSession(&models.Chat{LLMSettings: llmSettings}, ChatMessage, db)
	options := cs.getOptions(llmSettings, []llms.Tool{})

	assert.NotEmpty(t, options)
	// Additional checks for specific options can be added here
}

func TestChatSession_ErrorHandling(t *testing.T) {
	chat := &models.Chat{
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: "mock",
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	db := setupTestDB(t)
	cs := NewChatSession(chat, ChatMessage, db)
	err := cs.Start()
	assert.NoError(t, err)

	cs.AddPreProcessor(func(*UserMessage) error {
		return fmt.Errorf("test error")
	})

	cs.input <- &UserMessage{Payload: "Test message"}

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
	db := setupTestDB(t)
	cs := NewChatSession(&models.Chat{}, ChatMessage, db)

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
	cs := NewChatSession(&models.Chat{}, ChatMessage, db)

	spec, err := os.ReadFile("../universalclient/testdata/petstore.json")
	require.NoError(t, err)

	// Add a mock tool
	cs.tools = map[string]models.Tool{
		"test_tool": {
			ToolType:            models.ToolTypeREST,
			OASSpec:             spec,
			AvailableOperations: "addPet,updatePet",
		},
	}

	tools := cs.prepareTools()
	assert.NotEmpty(t, tools)
	// Additional checks on the prepared tools can be added here
}

func TestChatSession_ConvertLLMArgsToUniversalClientInputs(t *testing.T) {
	cs := NewChatSession(&models.Chat{}, ChatMessage, nil)

	testArgs := `{"body": {"key": "value"}, "headers": {"Content-Type": ["application/json"]}, "parameters": {"query": ["test"]}}`
	params, err := cs.convertLLMArgsToUniversalClientInputs([]byte(testArgs), "foo", nil)

	assert.NoError(t, err)
	assert.Equal(t, "value", params.Body["key"])
	assert.Equal(t, []string{"application/json"}, params.Headers["Content-Type"])
	assert.Equal(t, []string{"test"}, params.Parameters["query"])
}

func TestChatSession_ConvertLLMArgsToUniversalClientInputs_WithUnstructuredInput(t *testing.T) {
	cs := NewChatSession(&models.Chat{}, ChatMessage, nil)

	// LLMs might not send back the parameters key, so we just assume it for anything not in the other two categpories
	testArgs := `{"body": {"key": "value"}, "headers": {"Content-Type": ["application/json"]}, "query": ["test"]}`
	params, err := cs.convertLLMArgsToUniversalClientInputs([]byte(testArgs), "foo", nil)

	assert.NoError(t, err)
	assert.Equal(t, "value", params.Body["key"])
	assert.Equal(t, []string{"application/json"}, params.Headers["Content-Type"])
	assert.Equal(t, []string{"test"}, params.Parameters["query"])
}

func TestChatSession_HandleToolCalls(t *testing.T) {
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

	cs := NewChatSession(chatRef, ChatMessage, db)

	cs.initSession()

	spec, err := os.ReadFile("../universalclient/testdata/petstore.json")
	require.NoError(t, err)

	// Mock a tool
	cs.tools = map[string]models.Tool{
		"test_tool": {
			ToolType:            models.ToolTypeREST,
			OASSpec:             spec,
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
			Vendor: "mock",
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}
	cs := NewChatSession(chat, ChatMessage, db)
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
	cs := NewChatSession(&models.Chat{}, ChatMessage, nil)
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
	cs := NewChatSession(&models.Chat{}, ChatMessage, nil)
	docs := []schema.Document{
		{PageContent: "Document 1"},
		{PageContent: "Document 2"},
		{PageContent: "Document 3"},
	}

	joined := cs.joinDocuments(docs, " | ")
	assert.Equal(t, "Document 1 | Document 2 | Document 3", joined)
}

func TestChatSession_StreamingFunc(t *testing.T) {
	cs := NewChatSession(&models.Chat{}, ChatStream, nil)

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
			cs := NewChatSession(chat, ChatMessage, nil)
			_, err := cs.fetchDriver(nil)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestChatSession_Live(t *testing.T) {
	if os.Getenv("WEATHERBIT_KEY") == "" {
		t.Skip("Skipping live test, set WEATHERBIT_KEY to run this test")
	}

	db := setupTestDB(t)
	chat := &models.Chat{
		LLM: &models.LLM{
			Name:   "claude-3-5-sonnet-20240620",
			Vendor: models.ANTHROPIC,
			APIKey: os.Getenv("ANTHROPIC_KEY"),
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "claude-3-5-sonnet-20240620",
		},
	}

	spec, err := os.ReadFile("../universalclient/testdata/weatherbit.json")
	assert.NoError(t, err)

	weathertool := models.Tool{
		Name:                "weather forecast",
		Description:         "Get the weather forecast for a given location",
		ToolType:            models.ToolTypeREST,
		AvailableOperations: "ReturnsadailyforecastGivenLatLon",
		AuthKey:             os.Getenv("WEATHERBIT_KEY"),
		OASSpec:             spec,
	}

	session := NewChatSession(chat, ChatMessage, db)
	session.AddTool("weather", weathertool)

	err = session.Start()
	assert.NoError(t, err)

	// Send a message
	select {
	case session.Input() <- &UserMessage{Payload: "What is the weather like today in Auckland, New Zealand, and in New York City, USA?"}:
	default:
		assert.Fail(t, "Failed to send message")
	}

	// Wait for a response
	resps := 0
	t0 := time.Now()
	for {
		select {
		case resp := <-session.OutputMessage():
			fmt.Println("[RESPONSE]", resp.Payload)
			resps += 1
		case err := <-session.Errors():
			fmt.Println("[ERROR]", err)
			assert.Fail(t, "Error received")
		default:
			// if resps == 2 {
			// 	return
			// }
			if time.Since(t0) > 20*time.Second {
				assert.Fail(t, "Timeout waiting for response")
				return
			}
		}
	}

}

// func TestNewChatSession(t *testing.T) {
// 	db := setupTestDB(t)
// 	chat := &models.Chat{
// 		LLM: &models.LLM{
// 			Name: "Dummy LLM",
// 		},
// 		LLMSettings: &models.LLMSettings{
// 			ModelName: "dummy",
// 		},
// 	}

// 	cs := NewChatSession(chat, ChatMessage, db)

// 	assert.NotNil(t, cs)
// 	assert.Equal(t, chat, cs.chatRef)
// 	assert.Equal(t, ChatMessage, cs.mode)
// 	assert.NotNil(t, cs.input)
// 	assert.NotNil(t, cs.outputMessages)
// 	assert.NotNil(t, cs.outputStream)
// 	assert.NotNil(t, cs.stop)
// 	assert.NotNil(t, cs.errors)
// }

// func TestChatSession_InitSession(t *testing.T) {
// 	db := setupTestDB(t)
// 	chat := &models.Chat{
// 		LLM: &models.LLM{
// 			Name:   "Dummy LLM",
// 			Vendor: models.MOCK_VENDOR,
// 		},
// 		LLMSettings: &models.LLMSettings{
// 			ModelName: "dummy",
// 		},
// 	}

// 	cs := NewChatSession(chat, ChatMessage, db)
// 	err := cs.initSession()

// 	assert.NoError(t, err)
// }

// func TestChatSession_HandleUserMessage(t *testing.T) {
// 	db := setupTestDB(t)
// 	chat := &models.Chat{
// 		LLM: &models.LLM{
// 			Name:   "Dummy LLM",
// 			Vendor: models.MOCK_VENDOR,
// 		},
// 		LLMSettings: &models.LLMSettings{
// 			ModelName: "dummy",
// 		},
// 	}

// 	cs := NewChatSession(chat, ChatMessage, db)
// 	err := cs.initSession()
// 	assert.NoError(t, err)

// 	msg := &UserMessage{Payload: "Test message"}
// 	resp, err := cs.HandleUserMessage(msg, []schema.Document{}, []llms.Tool{})

// 	assert.NoError(t, err)
// 	assert.NotEmpty(t, resp)
// }

// func TestChatSession_PreProcessors(t *testing.T) {
// 	db := setupTestDB(t)
// 	cs := NewChatSession(&models.Chat{}, ChatMessage, db)

// 	preprocessor := func(msg *UserMessage) error {
// 		msg.Payload = "Processed: " + msg.Payload
// 		return nil
// 	}

// 	cs.AddPreProcessor(preprocessor)

// 	msg := &UserMessage{Payload: "Test message"}
// 	err := cs.preProcessMessage(msg)

// 	assert.NoError(t, err)
// 	assert.Equal(t, "Processed: Test message", msg.Payload)
// }

// func TestChatSession_Start(t *testing.T) {
// 	db := setupTestDB(t)
// 	chat := &models.Chat{
// 		LLM: &models.LLM{
// 			Name:   "Dummy LLM",
// 			Vendor: models.MOCK_VENDOR,
// 		},
// 		LLMSettings: &models.LLMSettings{
// 			ModelName: "dummy",
// 		},
// 	}

// 	cs := NewChatSession(chat, ChatMessage, db)
// 	err := cs.initSession()
// 	assert.NoError(t, err)

// 	cs.Start()

// 	// Send a message
// 	cs.input <- &UserMessage{Payload: "Test message"}

// 	// Wait for response
// 	select {
// 	case response := <-cs.OutputMessage():
// 		assert.NotEmpty(t, response.Payload)
// 	case err := <-cs.Errors():
// 		assert.Fail(t, "Received error", err)
// 	case <-time.After(10 * time.Second):
// 		assert.Fail(t, "Timeout waiting for response")
// 	}

// 	cs.Stop()
// }

// func TestChatSession_StreamingMode(t *testing.T) {
// 	chat := &models.Chat{
// 		LLM: &models.LLM{
// 			Name:   "Dummy LLM",
// 			Vendor: models.MOCK_VENDOR,
// 		},
// 		LLMSettings: &models.LLMSettings{
// 			ModelName: "dummy",
// 		},
// 	}

// 	db := setupTestDB(t)
// 	cs := NewChatSession(chat, ChatStream, db)
// 	err := cs.Start()
// 	assert.NoError(t, err)

// 	// Call with streaming option
// 	go func() {
// 		select {
// 		case cs.Input() <- &UserMessage{Payload: "Test prompt"}:
// 		default:
// 			fmt.Println("failed to send prompt")
// 		}

// 	}()

// 	// Check if streaming data is received
// 	count := 0
// 	for {
// 		select {
// 		case chunk := <-cs.OutputStream():
// 			fmt.Printf("%s ", string(chunk))
// 			count += 1
// 			assert.NotEmpty(t, chunk)
// 		case intErr := <-cs.Errors():
// 			assert.Fail(t, "Received error", intErr)
// 		case <-time.After(5 * time.Second):
// 			assert.Fail(t, "Timeout waiting for streaming data")
// 		}
// 		if count >= 10 {
// 			break
// 		}
// 	}

// 	assert.Greater(t, count, 9)
// 	cs.Stop()
// }

// func TestChatSession_GetOptions(t *testing.T) {
// 	llmSettings := &models.LLMSettings{
// 		Temperature: 0.7,
// 		MaxTokens:   100,
// 		TopP:        0.9,
// 	}

// 	db := setupTestDB(t)
// 	cs := NewChatSession(&models.Chat{LLMSettings: llmSettings}, ChatMessage, db)
// 	options := cs.getOptions(llmSettings, []llms.Tool{})

// 	assert.NotEmpty(t, options)
// 	// You might want to add more specific checks for each option
// 	// This would require exposing the option values or using reflection
// }

// func TestChatSession_ErrorHandling(t *testing.T) {
// 	chat := &models.Chat{
// 		LLM: &models.LLM{
// 			Name:   "Dummy LLM",
// 			Vendor: models.MOCK_VENDOR,
// 		},
// 		LLMSettings: &models.LLMSettings{
// 			ModelName: "dummy",
// 		},
// 	}

// 	db := setupTestDB(t)
// 	cs := NewChatSession(chat, ChatMessage, db)
// 	err := cs.initSession()
// 	assert.NoError(t, err)

// 	cs.Start()

// 	// Add a preprocessor that always fails
// 	cs.AddPreProcessor(func(*UserMessage) error {
// 		return assert.AnError
// 	})

// 	// Send a message
// 	cs.input <- &UserMessage{Payload: "Test message"}

// 	// Wait for error
// 	select {
// 	case err := <-cs.Errors():
// 		assert.Error(t, err)
// 		assert.Contains(t, err.Error(), "preprocessing error")
// 	case <-time.After(2 * time.Second):
// 		assert.Fail(t, "Timeout waiting for error")
// 	}

// 	cs.Stop()
// }

// func TestChatSession_Anthropic(t *testing.T) {
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

// 	cs := NewChatSession(chat, ChatMessage, db)
// 	cs.Start()

// 	// Send a message
// 	cs.input <- &UserMessage{Payload: "What is the capital of New Zealand"}

// 	// Wait for response
// 	select {
// 	case response := <-cs.OutputMessage():
// 		assert.NotEmpty(t, response.Payload)
// 		fmt.Println("[LLM Response]: ", response.Payload)
// 	case err := <-cs.Errors():
// 		assert.Fail(t, "Received error", err)
// 	case <-time.After(10 * time.Second):
// 		assert.Fail(t, "Timeout waiting for response")
// 	}

// 	cs.Stop()
// }

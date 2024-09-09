package chat_session

import (
	"fmt"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/tmc/langchaingo/chains"
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
			Vendor: models.MOCK_VENDOR,
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs := NewChatSession(chat, ChatMessage, db)
	err := cs.initSession()

	assert.NoError(t, err)
	assert.IsType(t, chains.LLMChain{}, cs.caller)
}

func TestChatSession_HandleUserMessage(t *testing.T) {
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

	cs := NewChatSession(chat, ChatMessage, db)
	err := cs.initSession()
	assert.NoError(t, err)

	msg := &UserMessage{Payload: "Test message"}
	resp, err := cs.HandleUserMessage(msg)

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
			Vendor: models.MOCK_VENDOR,
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs := NewChatSession(chat, ChatMessage, db)
	err := cs.initSession()
	assert.NoError(t, err)

	cs.Start()

	// Send a message
	cs.input <- &UserMessage{Payload: "Test message"}

	// Wait for response
	select {
	case response := <-cs.OutputMessage():
		assert.NotEmpty(t, response.Payload)
	case err := <-cs.Errors():
		assert.Fail(t, "Received error", err)
	case <-time.After(10 * time.Second):
		assert.Fail(t, "Timeout waiting for response")
	}

	cs.Stop()
}

func TestChatSession_StreamingMode(t *testing.T) {
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
	cs := NewChatSession(chat, ChatStream, db)
	err := cs.Start()
	assert.NoError(t, err)

	// Call with streaming option
	go func() {
		select {
		case cs.Input() <- &UserMessage{Payload: "Test prompt"}:
		default:
			fmt.Println("failed to send prompt")
		}

	}()

	// Check if streaming data is received
	count := 0
	for {
		select {
		case chunk := <-cs.OutputStream():
			fmt.Printf("%s ", string(chunk))
			count += 1
			assert.NotEmpty(t, chunk)
		case intErr := <-cs.Errors():
			assert.Fail(t, "Received error", intErr)
		case <-time.After(5 * time.Second):
			assert.Fail(t, "Timeout waiting for streaming data")
		}
		if count >= 10 {
			break
		}
	}

	assert.Greater(t, count, 9)
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
	options := cs.getOptions(llmSettings)

	assert.NotEmpty(t, options)
	// You might want to add more specific checks for each option
	// This would require exposing the option values or using reflection
}

func TestChatSession_ErrorHandling(t *testing.T) {
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
	cs := NewChatSession(chat, ChatMessage, db)
	err := cs.initSession()
	assert.NoError(t, err)

	cs.Start()

	// Add a preprocessor that always fails
	cs.AddPreProcessor(func(*UserMessage) error {
		return assert.AnError
	})

	// Send a message
	cs.input <- &UserMessage{Payload: "Test message"}

	// Wait for error
	select {
	case err := <-cs.Errors():
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "preprocessing error")
	case <-time.After(2 * time.Second):
		assert.Fail(t, "Timeout waiting for error")
	}

	cs.Stop()
}

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

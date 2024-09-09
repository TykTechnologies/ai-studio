package chat_session

import (
	"fmt"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

func TestNewChatSession(t *testing.T) {
	chat := &models.Chat{
		LLM: &models.LLM{
			Name: "Dummy LLM",
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs := NewChatSession(chat, ChatMessage)

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
	chat := &models.Chat{
		LLM: &models.LLM{
			Name: "Dummy LLM",
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs := NewChatSession(chat, ChatMessage)
	err := cs.initSession()

	assert.NoError(t, err)
	assert.IsType(t, &DummyDriver{}, cs.caller)
}

func TestChatSession_HandleUserMessage(t *testing.T) {
	chat := &models.Chat{
		LLM: &models.LLM{
			Name: "Dummy LLM",
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs := NewChatSession(chat, ChatMessage)
	err := cs.initSession()
	assert.NoError(t, err)

	msg := &UserMessage{Payload: "Test message"}
	resp, err := cs.HandleUserMessage(msg)

	assert.NoError(t, err)
	assert.NotEmpty(t, resp)
}

func TestChatSession_PreProcessors(t *testing.T) {
	cs := NewChatSession(&models.Chat{}, ChatMessage)

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
	chat := &models.Chat{
		LLM: &models.LLM{
			Name: "Dummy LLM",
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs := NewChatSession(chat, ChatMessage)
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
	case <-time.After(2 * time.Second):
		assert.Fail(t, "Timeout waiting for response")
	}

	cs.Stop()
}

func TestChatSession_StreamingMode(t *testing.T) {
	chat := &models.Chat{
		LLM: &models.LLM{
			Name: "Dummy LLM",
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs := NewChatSession(chat, ChatStream)
	err := cs.Start()
	assert.NoError(t, err)

	// Call with streaming option
	cs.Input() <- &UserMessage{Payload: "Test prompt"}

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
		case <-time.After(1 * time.Second):
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
		Temperature:      0.7,
		MaxTokens:        100,
		FrequencyPenalty: 0.5,
		PresencePenalty:  0.5,
		TopP:             0.9,
	}

	cs := NewChatSession(&models.Chat{LLMSettings: llmSettings}, ChatMessage)
	options := cs.getOptions(llmSettings)

	assert.NotEmpty(t, options)
	// You might want to add more specific checks for each option
	// This would require exposing the option values or using reflection
}

func TestChatSession_ErrorHandling(t *testing.T) {
	chat := &models.Chat{
		LLM: &models.LLM{
			Name: "Dummy LLM",
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
	}

	cs := NewChatSession(chat, ChatMessage)
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

package chat_session

import (
	"context"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

type ChatMode string

const (
	ChatStream  ChatMode = "stream"
	ChatMessage ChatMode = "message"
)

type LLMDriver interface {
	Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error)
}

type ChatSession struct {
	chatRef        *models.Chat
	input          chan *UserMessage
	outputMessages chan *ChatResponse
	outputStream   chan []byte
	stop           chan struct{}
	errors         chan error
	preProcessors  []func(*UserMessage) error
	postProcessors []func(*UserMessage) error
	caller         LLMDriver
	mode           ChatMode
}

type UserMessage struct {
	Payload string
}

type ChatResponse struct {
	Payload string
}

func NewChatSession(chat *models.Chat, mode ChatMode) *ChatSession {
	return &ChatSession{
		chatRef:        chat,
		input:          make(chan *UserMessage, 100),
		outputMessages: make(chan *ChatResponse, 100),
		outputStream:   make(chan []byte, 100),
		stop:           make(chan struct{}),
		errors:         make(chan error, 100),
		preProcessors:  []func(*UserMessage) error{},
		postProcessors: []func(*UserMessage) error{},
		mode:           mode,
	}
}

func (cs *ChatSession) Errors() chan error {
	return cs.errors
}

func (cs *ChatSession) OutputMessage() chan *ChatResponse {
	return cs.outputMessages
}

func (cs *ChatSession) OutputStream() chan []byte {
	return cs.outputStream
}

func (cs *ChatSession) Input() chan *UserMessage {
	return cs.input
}

func (cs *ChatSession) AddPreProcessor(fn func(*UserMessage) error) {
	cs.preProcessors = append(cs.preProcessors, fn)
}

func (cs *ChatSession) AddPostProcessor(fn func(*UserMessage) error) {
	cs.postProcessors = append(cs.postProcessors, fn)
}

func (cs *ChatSession) Stop() {
	cs.stop <- struct{}{}
	close(cs.input)
	close(cs.outputMessages)
	close(cs.outputStream)
}

func (cs *ChatSession) Start() error {
	fmt.Println("starting chat session")
	if err := cs.initSession(); err != nil {
		return fmt.Errorf("error initializing chat session: %v", err)
	}

	go func() {
		for {
			select {
			case <-cs.stop:
				return
			case msg := <-cs.input:
				fmt.Println("input received")
				err := cs.preProcessMessage(msg)
				if err != nil {
					cs.errors <- fmt.Errorf("preprocessing error: %v", err)
					continue
				}

				resp, err := cs.HandleUserMessage(msg)
				if err != nil {
					cs.errors <- fmt.Errorf("chat session handler error: %v", err)
					continue
				}

				select {
				case cs.outputMessages <- &ChatResponse{Payload: resp}:
				default:
					cs.errors <- fmt.Errorf("output channel is full")
				}
			}
		}
	}()

	return nil
}

func (cs *ChatSession) initSession() error {
	// create the LLM client
	var llm LLMDriver
	var err error
	switch cs.chatRef.LLMSettings.ModelName {
	case "gpt-4o":
		llm, err = setupOpenAIDriver(cs.chatRef.LLM, cs.chatRef.LLMSettings)
	case "gpt-4":
		llm, err = setupOpenAIDriver(cs.chatRef.LLM, cs.chatRef.LLMSettings)
	case "gpt-4-turbo":
		llm, err = setupOpenAIDriver(cs.chatRef.LLM, cs.chatRef.LLMSettings)
	case "gpt-3.5-turbo":
		llm, err = setupOpenAIDriver(cs.chatRef.LLM, cs.chatRef.LLMSettings)
	case "gpt-3.5":
		llm, err = setupOpenAIDriver(cs.chatRef.LLM, cs.chatRef.LLMSettings)
	case "gpt-3":
		llm, err = setupOpenAIDriver(cs.chatRef.LLM, cs.chatRef.LLMSettings)
	case "dummy":
		llm = &DummyDriver{}
	default:
		return fmt.Errorf("unsupported LLM model: %s", cs.chatRef.LLMSettings.ModelName)
	}

	if err != nil {
		return fmt.Errorf("failed to create LLM driver for model %s: %v", cs.chatRef.LLMSettings.ModelName, err)
	}

	cs.caller = llm

	return nil
}

func (cs *ChatSession) preProcessMessage(msg *UserMessage) error {
	for _, fn := range cs.preProcessors {
		if err := fn(msg); err != nil {
			return err
		}
	}
	return nil
}

func (cs *ChatSession) HandleUserMessage(msg *UserMessage) (string, error) {
	fmt.Println("Handling user message")
	opts := cs.getOptions(cs.chatRef.LLMSettings)
	if cs.caller == nil {
		return "", fmt.Errorf("LLM driver is not initialized")
	}
	resp, err := cs.caller.Call(context.Background(), msg.Payload, opts...)
	if err != nil {
		return "", err
	}

	return resp, nil
}

func (cs *ChatSession) streamingFunc(ctx context.Context, chunk []byte) error {
	select {
	case cs.outputStream <- chunk:
	case <-ctx.Done():
		return nil
	default:
		return fmt.Errorf("streaming channel is full")
	}

	return nil
}

func (cs *ChatSession) getOptions(llmSettings *models.LLMSettings) []llms.CallOption {
	var callOptions = make([]llms.CallOption, 0)
	if llmSettings.CandidateCount > 0 {
		callOptions = append(callOptions, llms.WithCandidateCount(llmSettings.CandidateCount))
	}
	if llmSettings.FrequencyPenalty > 0 {
		callOptions = append(callOptions, llms.WithFrequencyPenalty(llmSettings.FrequencyPenalty))
	}
	if llmSettings.JSONMode {
		callOptions = append(callOptions, llms.WithJSONMode())
	}
	if llmSettings.MaxLength > 0 {
		callOptions = append(callOptions, llms.WithMaxLength(llmSettings.MaxLength))
	}
	if llmSettings.MaxTokens > 0 {
		callOptions = append(callOptions, llms.WithMaxTokens(llmSettings.MaxTokens))
	}
	if llmSettings.MinLength > 0 {
		callOptions = append(callOptions, llms.WithMinLength(llmSettings.MinLength))
	}
	if llmSettings.N > 0 {
		callOptions = append(callOptions, llms.WithN(llmSettings.N))
	}
	if llmSettings.PresencePenalty > 0 {
		callOptions = append(callOptions, llms.WithPresencePenalty(llmSettings.PresencePenalty))
	}
	if llmSettings.RepetitionPenalty > 0 {
		callOptions = append(callOptions, llms.WithRepetitionPenalty(llmSettings.RepetitionPenalty))
	}
	if llmSettings.Seed > 0 {
		callOptions = append(callOptions, llms.WithSeed(llmSettings.Seed))
	}
	if len(llmSettings.StopWords) > 0 {
		callOptions = append(callOptions, llms.WithStopWords(llmSettings.StopWords))
	}
	if llmSettings.Temperature > 0 {
		callOptions = append(callOptions, llms.WithTemperature(llmSettings.Temperature))
	}
	if llmSettings.TopK > 0 {
		callOptions = append(callOptions, llms.WithTopK(llmSettings.TopK))
	}
	if llmSettings.TopP > 0 {
		callOptions = append(callOptions, llms.WithTopP(llmSettings.TopP))
	}

	if cs.mode == ChatStream {
		fmt.Println("STRAMING MODE")
		callOptions = append(callOptions, llms.WithStreamingFunc(cs.streamingFunc))
	}

	return callOptions
}

func setupOpenAIDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (LLMDriver, error) {
	var opts = make([]openai.Option, 0)
	if connDef.APIEndpoint != "" {
		opts = append(opts, openai.WithBaseURL(connDef.APIEndpoint))
	}

	if connDef.APIKey != "" {
		opts = append(opts, openai.WithToken(connDef.APIKey))
	}

	opts = append(opts, openai.WithModel(llmSettings.ModelName))

	llm, err := openai.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI driver: %v", err)
	}

	return llm, nil
}

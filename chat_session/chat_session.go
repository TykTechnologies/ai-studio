package chat_session

import (
	"context"
	"fmt"
	"time"

	dataSession "github.com/TykTechnologies/midsommar/v2/data_session"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gofrs/uuid"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/prompts"
	"github.com/tmc/langchaingo/schema"
	"gorm.io/gorm"
)

type ChatMode string

const (
	ChatStream  ChatMode = "stream"
	ChatMessage ChatMode = "message"
)

const informedTemplate = `The following is a friendly conversation between a human and an AI. The AI is talkative and provides lots of specific details from its context. If the AI does not know the answer to a question, it truthfully says it does not know.
Some Background:
{{.context}}
Current conversation:
{{.history}}
Human: {{.input}}
AI:`

type LLMDriver interface {
	Call(ctx context.Context, inputs map[string]any, options ...chains.ChainCallOption) (map[string]any, error)
}

type ChatSession struct {
	id             string
	chatRef        *models.Chat
	input          chan *UserMessage
	outputMessages chan *ChatResponse
	outputStream   chan []byte
	stop           chan struct{}
	errors         chan error
	preProcessors  []func(*UserMessage) error
	caller         chains.Chain
	mode           ChatMode
	db             *gorm.DB
	datasources    map[uint]*models.Datasource
}

type UserMessage struct {
	Payload string
}

type ChatResponse struct {
	Payload string
}

func NewChatSession(chat *models.Chat, mode ChatMode, db *gorm.DB) *ChatSession {
	id, _ := uuid.NewV4()
	return &ChatSession{
		id:             id.String(),
		chatRef:        chat,
		input:          make(chan *UserMessage, 100),
		outputMessages: make(chan *ChatResponse, 100),
		outputStream:   make(chan []byte, 100),
		stop:           make(chan struct{}),
		errors:         make(chan error, 100),
		preProcessors:  []func(*UserMessage) error{},
		mode:           mode,
		db:             db,
		datasources:    map[uint]*models.Datasource{},
	}
}

func (cs *ChatSession) ID() string {
	return cs.id
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

func (cs *ChatSession) AddDatasource(id uint) error {
	ds := models.Datasource{}
	err := ds.Get(cs.db, id)
	if err != nil {
		return fmt.Errorf("error getting datasource: %v", err)
	}

	cs.datasources[id] = &ds
	return nil
}

func (cs *ChatSession) RemoveDatasource(id uint) {
	delete(cs.datasources, id)
}

func (cs *ChatSession) AddPreProcessor(fn func(*UserMessage) error) {
	cs.preProcessors = append(cs.preProcessors, fn)
}

func (cs *ChatSession) Stop() {
	cs.stop <- struct{}{}
	close(cs.input)
	close(cs.outputMessages)
	close(cs.outputStream)
}

func (cs *ChatSession) Start() error {
	if err := cs.initSession(); err != nil {
		return fmt.Errorf("error initializing chat session: %v", err)
	}

	go func() {
		for {
			select {
			case <-cs.stop:
				return
			case msg := <-cs.input:
				err := cs.preProcessMessage(msg)
				if err != nil {
					cs.errors <- fmt.Errorf("preprocessing error: %v", err)
					continue
				}

				ds := dataSession.NewDataSession(cs.datasources)
				docs, err := ds.Search(msg.Payload, 5)
				if err != nil {
					cs.errors <- fmt.Errorf("error searching datasources: %v", err)
					continue
				}

				resp, err := cs.HandleUserMessage(msg, docs)
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
	// History for the chain
	chatHistory := NewGormChatMessageHistory(cs.db, cs.id)
	conversationBuffer := memory.NewConversationBuffer(
		memory.WithChatHistory(chatHistory),
		memory.WithInputKey("input"))

	// create the LLM client
	llm, err := cs.fetchDriver(conversationBuffer)
	if err != nil {
		return err
	}

	convo := chains.NewConversation(llm, conversationBuffer)
	convo.Prompt = prompts.NewPromptTemplate(
		informedTemplate,
		[]string{
			"history",
			"input",
			"context",
		})

	docChain := chains.NewStuffDocuments(&convo)
	cs.caller = docChain

	return nil
}

func (cs *ChatSession) fetchDriver(mem schema.Memory) (llms.Model, error) {
	var llm llms.Model
	var err error
	switch cs.chatRef.LLM.Vendor {
	case models.OPENAI:
		llm, err = setupOpenAIDriver(cs.chatRef.LLM, cs.chatRef.LLMSettings)
	case models.ANTHROPIC:
		llm, err = setupAnthropicDriver(cs.chatRef.LLM, cs.chatRef.LLMSettings)
	case models.MOCK_VENDOR:
		llm = &DummyDriver{
			StreamingFunc: cs.streamingFunc,
			Memory:        mem,
		}
	default:
		return nil, fmt.Errorf("unsupported LLM model: %s", cs.chatRef.LLMSettings.ModelName)
	}

	return llm, err
}

func (cs *ChatSession) preProcessMessage(msg *UserMessage) error {
	for _, fn := range cs.preProcessors {
		if err := fn(msg); err != nil {
			return err
		}
	}
	return nil
}

func (cs *ChatSession) fetchDocuments(payload string) ([]schema.Document, error) {
	// var docs []schema.Document
	panic("not implemented")
}

func (cs *ChatSession) HandleUserMessage(msg *UserMessage, docs []schema.Document) (string, error) {
	opts := cs.getOptions(cs.chatRef.LLMSettings)
	if cs.caller == nil {
		return "", fmt.Errorf("LLM driver is not initialized")
	}

	ctx, done := context.WithTimeout(context.Background(), 120*time.Second)
	defer done()
	// resp, err := chains.Run(ctx, cs.caller, msg.Payload, opts...)
	inputs := map[string]any{
		"input":           msg.Payload,
		"input_documents": docs,
	}

	resp, err := chains.Call(ctx, cs.caller, inputs, opts...)
	if err != nil {
		return "", err
	}

	txt, ok := resp["text"]
	if !ok {
		return "", fmt.Errorf("no text field returned by caller")
	}

	txtStr, ok := txt.(string)
	if !ok {
		return "", fmt.Errorf("text field is not a string")
	}

	return txtStr, nil
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

func (cs *ChatSession) getOptions(llmSettings *models.LLMSettings) []chains.ChainCallOption {
	var callOptions = make([]chains.ChainCallOption, 0)

	if llmSettings.MaxLength > 0 {
		callOptions = append(callOptions, chains.WithMaxLength(llmSettings.MaxLength))
	}
	if llmSettings.MaxTokens > 0 {
		callOptions = append(callOptions, chains.WithMaxTokens(llmSettings.MaxTokens))
	}
	if llmSettings.MinLength > 0 {
		callOptions = append(callOptions, chains.WithMinLength(llmSettings.MinLength))
	}

	if llmSettings.RepetitionPenalty > 0 {
		callOptions = append(callOptions, chains.WithRepetitionPenalty(llmSettings.RepetitionPenalty))
	}
	if llmSettings.Seed > 0 {
		callOptions = append(callOptions, chains.WithSeed(llmSettings.Seed))
	}
	if len(llmSettings.StopWords) > 0 {
		callOptions = append(callOptions, chains.WithStopWords(llmSettings.StopWords))
	}
	if llmSettings.Temperature > 0 {
		callOptions = append(callOptions, chains.WithTemperature(llmSettings.Temperature))
	}
	if llmSettings.TopK > 0 {
		callOptions = append(callOptions, chains.WithTopK(llmSettings.TopK))
	}
	if llmSettings.TopP > 0 {
		callOptions = append(callOptions, chains.WithTopP(llmSettings.TopP))
	}

	if cs.mode == ChatStream {
		callOptions = append(callOptions, chains.WithStreamingFunc(cs.streamingFunc))
	}

	return callOptions
}

func setupOpenAIDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
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

func setupAnthropicDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	var opts = make([]anthropic.Option, 0)
	if connDef.APIEndpoint != "" {
		opts = append(opts, anthropic.WithBaseURL(connDef.APIEndpoint))
	}

	if connDef.APIKey != "" {
		opts = append(opts, anthropic.WithToken(connDef.APIKey))
	}

	opts = append(opts, anthropic.WithModel(llmSettings.ModelName))

	llm, err := anthropic.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI driver: %v", err)
	}

	return llm, nil
}

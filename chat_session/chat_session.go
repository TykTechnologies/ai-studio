package chat_session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dataSession "github.com/TykTechnologies/midsommar/v2/data_session"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/universalclient"
	"github.com/gofrs/uuid"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/schema"
	"gorm.io/gorm"
)

type ChatMode string

const (
	ChatStream  ChatMode = "stream"
	ChatMessage ChatMode = "message"
)

type LLMDriver interface {
	Call(ctx context.Context, inputs map[string]any, options ...chains.ChainCallOption) (map[string]any, error)
}

type ChatSession struct {
	id             string
	chatRef        *models.Chat
	chatHistory    *GormChatMessageHistory
	input          chan *UserMessage
	outputMessages chan *ChatResponse
	outputStream   chan []byte
	stop           chan struct{}
	errors         chan error
	preProcessors  []func(*UserMessage) error
	caller         llms.Model
	mode           ChatMode
	db             *gorm.DB
	datasources    map[uint]*models.Datasource
	tools          map[string]models.Tool
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

				// handle RAG
				ds := dataSession.NewDataSession(cs.datasources)
				docs, err := ds.Search(msg.Payload, 5)
				if err != nil {
					cs.errors <- fmt.Errorf("error searching datasources: %v", err)
					continue
				}

				// prep tools
				tools := cs.prepareTools()

				resp, err := cs.HandleUserMessage(msg, docs, tools)
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

func (cs *ChatSession) prepareTools() []llms.Tool {
	tools := make([]llms.Tool, 0)
	for _, t := range cs.tools {
		switch t.ToolType {
		case models.ToolTypeREST:
			opts := []universalclient.ClientOption{}
			if t.AuthKey != "" {
				// API Key only at the moment
				opts = append(opts, universalclient.WithAuth("apiKey", t.AuthKey))
			}

			uc, err := universalclient.NewClient(t.OASSpec, "", opts...)
			if err != nil {
				cs.errors <- fmt.Errorf("error creating universal client: %v", err)
				continue
			}

			if len(t.GetOperations()) > 0 {
				asToolDef, err := uc.AsTool(t.GetOperations()...)
				if err != nil {
					cs.errors <- fmt.Errorf("error creating tool definition: %v", err)
					continue
				}

				tools = append(tools, asToolDef...)
			}

		default:
			cs.errors <- fmt.Errorf("unknown tool type: %s", t.ToolType)
		}

	}

	return tools
}

func (cs *ChatSession) initSession() error {
	// History for the chat session
	cs.chatHistory = NewGormChatMessageHistory(cs.db, cs.id)

	// create the LLM client
	llm, err := cs.fetchDriver(nil)
	if err != nil {
		return err
	}

	cs.caller = llm

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

func (cs *ChatSession) joinDocuments(docs []schema.Document, separator string) string {
	var text string
	docLen := len(docs)
	for k, doc := range docs {
		text += doc.PageContent
		if k != docLen-1 {
			text += separator
		}
	}
	return text
}

func (cs *ChatSession) prepHumanMessage(payload string, docs []schema.Document) llms.HumanChatMessage {
	pl := fmt.Sprintf("Context for this message: \n\n%s\n\nMessage: \n\n%s", cs.joinDocuments(docs, "\n\n"), payload)
	return llms.HumanChatMessage{
		Content: pl,
	}
}

func (cs *ChatSession) getMessages() ([]llms.MessageContent, error) {
	history, err := cs.chatHistory.Messages(context.Background())
	if err != nil {
		return nil, fmt.Errorf("error getting chat history: %v", err)
	}

	// manual history management (!)
	messages := make([]llms.MessageContent, 0)
	for i, _ := range history {
		messages = append(messages, llms.TextParts(history[i].GetType(), history[i].GetContent()))
	}

	return messages, nil
}

func (cs *ChatSession) HandleUserMessage(msg *UserMessage, docs []schema.Document, tools []llms.Tool) (string, error) {
	opts := cs.getOptions(cs.chatRef.LLMSettings, tools)
	if cs.caller == nil {
		return "", fmt.Errorf("LLM driver is not initialized")
	}

	ctx, done := context.WithTimeout(context.Background(), 300*time.Second)
	defer done()

	err := cs.chatHistory.AddMessage(context.Background(), cs.prepHumanMessage(msg.Payload, docs))
	if err != nil {
		return "", fmt.Errorf("error adding message to history: %v", err)
	}

	messages, err := cs.getMessages()
	if err != nil {
		return "", fmt.Errorf("error getting chat history: %v", err)
	}

	resp, err := cs.caller.GenerateContent(ctx, messages, opts...)
	if err != nil {
		return "", fmt.Errorf("error generating content: %v", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices returned by model")
	}

	content := ""
	// lets just use the first reply for now
	reply := resp.Choices[0]

	if reply.FuncCall != nil {
		tNames := []string{}
		for i, _ := range reply.ToolCalls {
			tNames = append(tNames, reply.ToolCalls[i].FunctionCall.Name)
		}

		content = fmt.Sprintf("Using tools: %s\n", strings.Join(tNames, ", "))
		called, err := cs.handleToolCalls(reply, tools)
		if err != nil {
			return "", fmt.Errorf("error handling tool calls: %v", err)
		}

		if called {
			history, err := cs.getMessages()
			if err != nil {
				return "", fmt.Errorf("error getting chat history after tool call: %v", err)
			}
			toolCallResp, err := cs.caller.GenerateContent(ctx, history, opts...)
			if err != nil {
				return "", fmt.Errorf("error generating content: %v", err)
			}

			// Not sure if this is the right option here, it may want to call more tools?
			return toolCallResp.Choices[0].Content, nil
		}
	} else {
		// Add the response to the chat history
		content = reply.Content
		err = cs.chatHistory.AddMessage(context.Background(), llms.HumanChatMessage{
			Content: reply.Content,
		})
		if err != nil {
			return "", fmt.Errorf("error adding message to history: %v", err)
		}
	}

	return content, nil
}

type CallParams struct {
	Body    map[string]interface{} `json:"body"`
	Headers map[string][]string    `json:"headers"`
	Params  map[string][]string    `json:"params"`
}

func (cs *ChatSession) convertLLMArgsToUniversalClientInputs(params []byte) (*CallParams, error) {
	callParams := &CallParams{}
	err := json.Unmarshal(params, callParams)
	if err != nil {
		return nil, err
	}

	return callParams, nil
}

func (cs *ChatSession) handleToolCalls(choice *llms.ContentChoice, tools []llms.Tool) (bool, error) {
	err := cs.chatHistory.AddMessage(context.Background(), llms.AIChatMessage{
		Content:   choice.Content,
		ToolCalls: choice.ToolCalls,
	})

	if err != nil {
		return false, fmt.Errorf("error adding message to history: %v", err)
	}

	called := false
	for i, _ := range choice.ToolCalls {
		t := choice.ToolCalls[i]
		toolDef, ok := cs.tools[t.FunctionCall.Name]
		if !ok {
			return false, fmt.Errorf("tool not found: %s", t.FunctionCall.Name)
		}

		// Call the tool
		if toolDef.ToolType == models.ToolTypeREST {
			opts := make([]universalclient.ClientOption, 0)
			if toolDef.AuthKey != "" {
				opts = append(opts, universalclient.WithAuth("apiKey", toolDef.AuthKey))
			}

			opts = append(opts, universalclient.WithResponseFormat(universalclient.ResponseFormatJSON))

			uc, err := universalclient.NewClient(toolDef.OASSpec, "", opts...)
			if err != nil {
				return false, fmt.Errorf("error creating tool client: %v", err)
			}

			args, err := cs.convertLLMArgsToUniversalClientInputs([]byte(t.FunctionCall.Arguments))
			if err != nil {
				return false, fmt.Errorf("error converting LLM args to universal client inputs: %v", err)
			}

			resp, err := uc.CallOperation(t.FunctionCall.Name, args.Params, args.Body, args.Headers)
			if err != nil {
				return false, fmt.Errorf("error calling tool operation: %v", err)
			}

			asStr, ok := resp.([]byte)
			if !ok {
				return false, fmt.Errorf("response is not a byte array")
			}

			err = cs.chatHistory.AddMessage(context.Background(), llms.ToolChatMessage{
				ID:      t.ID,
				Content: string(asStr),
			})

			if err != nil {
				return false, fmt.Errorf("error adding message to history: %v", err)
			}

			called = true
		}
	}

	return called, nil
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

func (cs *ChatSession) getOptions(llmSettings *models.LLMSettings, tools []llms.Tool) []llms.CallOption {
	var callOptions = make([]llms.CallOption, 0)

	if llmSettings.MaxLength > 0 {
		callOptions = append(callOptions, llms.WithMaxLength(llmSettings.MaxLength))
	}
	if llmSettings.MaxTokens > 0 {
		callOptions = append(callOptions, llms.WithMaxTokens(llmSettings.MaxTokens))
	}
	if llmSettings.MinLength > 0 {
		callOptions = append(callOptions, llms.WithMinLength(llmSettings.MinLength))
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
		callOptions = append(callOptions, llms.WithStreamingFunc(cs.streamingFunc))
	}

	if len(tools) > 0 {
		callOptions = append(callOptions, llms.WithTools(tools))
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

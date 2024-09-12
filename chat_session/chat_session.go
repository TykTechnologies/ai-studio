package chat_session

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
		tools:          map[string]models.Tool{},
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

func (cs *ChatSession) AddTool(id string, t models.Tool) {
	cs.tools[id] = t
}

func (cs *ChatSession) RemoveTool(id string) {
	delete(cs.tools, id)
}

func (cs *ChatSession) CurrentTools() map[string]models.Tool {
	return cs.tools
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

				cs.HandleUserMessage(msg, docs, tools)
				// if err != nil {
				// 	cs.errors <- fmt.Errorf("chat session handler error: %v", err)
				// 	continue
				// }
			}
		}
	}()

	return nil
}

func (cs *ChatSession) sendOutput(resp string) {
	select {
	case cs.outputMessages <- &ChatResponse{Payload: resp}:
	}
}

func (cs *ChatSession) sendError(err error) {
	select {
	case cs.errors <- err:
	default:
		fmt.Println("error sending error to channel")
	}
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

	return history, nil
}

func (cs *ChatSession) HandleUserMessage(msg *UserMessage, docs []schema.Document, tools []llms.Tool) (string, error) {
	opts := cs.getOptions(cs.chatRef.LLMSettings, tools)
	if cs.caller == nil {
		return "", fmt.Errorf("LLM driver is not initialized")
	}

	ctx, done := context.WithTimeout(context.Background(), 300*time.Second)
	defer done()

	err := cs.chatHistory.AddUserMessage(context.Background(), cs.prepHumanMessage(msg.Payload, docs).Content)
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

	toolCall := false

	mc := llms.MessageContent{
		Role:  llms.ChatMessageTypeAI,
		Parts: []llms.ContentPart{},
	}
	for _, reply := range resp.Choices {
		if reply.Content != "" {
			// this is to make sure the function responds with a message,
			// regular usage should use the
			// Messager() channel
			content = reply.Content
			cs.sendOutput(reply.Content)
		}

		if len(reply.ToolCalls) > 0 {
			_, err := cs.handleToolCalls(reply, &mc)
			if err != nil {
				cs.sendError(fmt.Errorf("error handling tool calls: %v", err))
				continue
			}

			toolCall = true
		}
	}

	if err != nil {
		return "", err
	}

	if toolCall {
		history, err := cs.getMessages()
		if err != nil {
			cs.sendError(fmt.Errorf("error getting chat history after tool call: %v", err))
			return "", err
		}

		toolCallResp, err := cs.caller.GenerateContent(ctx, history, opts...)
		if err != nil {
			cs.sendError(fmt.Errorf("error generating content after tool call: %v", err))
			return "", err
		}

		err = cs.chatHistory.AddAIMessage(ctx, toolCallResp.Choices[0].Content)
		if err != nil {
			cs.sendError(fmt.Errorf("error adding AI message to history: %v", err))
			return "", err
		}

		// Not sure if this is the right option here, it may want to call more tools?
		cs.sendOutput(toolCallResp.Choices[0].Content)
	}

	return content, nil
}

type CallParams struct {
	Body       map[string]interface{} `json:"body"`
	Headers    map[string][]string    `json:"headers"`
	Parameters map[string][]string    `json:"parameters"`
}

func (cs *ChatSession) convertLLMArgsToUniversalClientInputs(params []byte, opName string, uc *universalclient.Client) (*CallParams, error) {
	callParams := map[string]interface{}{}
	err := json.Unmarshal(params, &callParams)
	if err != nil {
		return nil, err
	}

	actualParams := &CallParams{
		Headers:    map[string][]string{},
		Parameters: map[string][]string{},
		Body:       map[string]interface{}{},
	}

	for k, v := range callParams {
		if k == "body" {
			actualParams.Body = v.(map[string]interface{})
			continue
		}

		if k == "headers" {
			for hk, hv := range v.(map[string]interface{}) {
				hSlice, ok := hv.([]interface{})
				if ok {
					for _, h := range hSlice {
						asStr, ok := h.(string)
						if ok {
							actualParams.Headers[hk] = append(actualParams.Headers[hk], asStr)
						}
					}
					continue
				}
				actualParams.Headers[hk] = hv.([]string)
			}
			continue
		}

		if k == "parameters" {
			for pk, pv := range v.(map[string]interface{}) {
				pSlice, ok := pv.([]interface{})
				if ok {
					for _, p := range pSlice {
						asStr, ok := p.(string)
						if ok {
							actualParams.Parameters[pk] = append(actualParams.Parameters[pk], asStr)
						}
					}
					continue
				}
				actualParams.Parameters[pk] = pv.([]string)
			}
			continue
		}

		paramName := k
		paramValue := callParams[k]

		switch paramValue.(type) {
		case string:
			actualParams.Parameters[paramName] = []string{paramValue.(string)}
		case []interface{}:
			for _, v := range paramValue.([]interface{}) {
				switch v.(type) {
				case string:
					actualParams.Parameters[paramName] = append(actualParams.Parameters[paramName], v.(string))
				}
			}
		case []string:
			actualParams.Parameters[paramName] = paramValue.([]string)
		case float64:
			actualParams.Parameters[paramName] = []string{strconv.FormatFloat(paramValue.(float64), 'f', -1, 64)}
		case int:
			actualParams.Parameters[paramName] = []string{strconv.Itoa(paramValue.(int))}
		case float32:
			actualParams.Parameters[paramName] = []string{strconv.FormatFloat(float64(paramValue.(float32)), 'f', -1, 32)}
		case bool:
			actualParams.Parameters[paramName] = []string{strconv.FormatBool(paramValue.(bool))}
		default:
			return nil, fmt.Errorf("unsupported type for parameter %s: %T", k, v)
		}

	}

	return actualParams, nil
}

func (cs *ChatSession) handleToolCalls(choice *llms.ContentChoice, mc *llms.MessageContent) (bool, error) {
	called := false
	for i, _ := range choice.ToolCalls {
		t := choice.ToolCalls[i]

		mc.Parts = append(mc.Parts, llms.ToolCall{
			ID:   t.ID,
			Type: t.Type,
			FunctionCall: &llms.FunctionCall{
				Name:      t.FunctionCall.Name,
				Arguments: t.FunctionCall.Arguments,
			},
		})

		// save the tool call to the chat history
		err := cs.chatHistory.AddMessage(context.Background(), *mc)

		// err := cs.chatHistory.AddAIToolCall(context.Background(), llms.ToolCall{
		// 	ID:   t.ID,
		// 	Type: t.Type,
		// 	FunctionCall: &llms.FunctionCall{
		// 		Name:      t.FunctionCall.Name,
		// 		Arguments: t.FunctionCall.Arguments,
		// 	},
		// })

		if err != nil {
			return false, fmt.Errorf("error adding tool call to history: %v", err)
		}

		// tools are sent to the LLM  as a list of operation names
		// This means that the tool name from the LLM will be the opp,
		// not the tool name
		toolDefIndex := ""
		for i, tool := range cs.tools {
			asList := strings.Split(tool.AvailableOperations, ",")
			for _, op := range asList {
				if op == t.FunctionCall.Name {
					toolDefIndex = i
					break
				}
				if toolDefIndex != "" {
					break
				}
			}
		}

		toolDef, ok := cs.tools[toolDefIndex]
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

			args, err := cs.convertLLMArgsToUniversalClientInputs([]byte(t.FunctionCall.Arguments), t.FunctionCall.Name, uc)
			if err != nil {
				return false, fmt.Errorf("error converting LLM args to universal client inputs: %v", err)
			}

			resp, err := uc.CallOperation(t.FunctionCall.Name, args.Parameters, args.Body, args.Headers)
			if err != nil {
				return false, fmt.Errorf("error calling tool operation: %v", err)
			}

			var asStr string
			switch resp.(type) {
			case []byte:
				asStr = string(resp.([]byte))
			case string:
				asStr = resp.(string)
			default:
				return false, fmt.Errorf("response is not a compatible string (%T)", resp)
			}

			toolResp := llms.ToolCallResponse{
				ToolCallID: t.ID,
				Name:       t.FunctionCall.Name,
				Content:    asStr,
			}

			err = cs.chatHistory.AddToolMessage(context.Background(), toolResp)
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

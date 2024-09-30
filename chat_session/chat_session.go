package chat_session

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	dataSession "github.com/TykTechnologies/midsommar/v2/data_session"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/TykTechnologies/midsommar/v2/universalclient"
	"github.com/gofrs/uuid"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
	"gorm.io/gorm"
)

type ChatMode string

const (
	ChatStream  ChatMode = "stream"
	ChatMessage ChatMode = "message"
)

type LLMResponseWrapper struct {
	Response *llms.ContentResponse
	Opts     []llms.CallOption
}

type LLMDriver interface {
	Call(ctx context.Context, inputs map[string]any, options ...chains.ChainCallOption) (map[string]any, error)
}

type ChatSession struct {
	id             string
	chatRef        *models.Chat
	chatHistory    *GormChatMessageHistory
	input          chan *models.UserMessage
	llmResponses   chan *LLMResponseWrapper
	outputMessages chan *ChatResponse
	outputStream   chan []byte
	stop           chan struct{}
	errors         chan error
	preProcessors  []func(*models.UserMessage) error
	scriptRunners  []*scripting.ScriptRunner
	caller         llms.Model
	mode           ChatMode
	datasources    map[uint]*models.Datasource
	tools          map[string]models.Tool
	db             *gorm.DB
	service        *services.Service
	userID         uint
	files          map[string]string
}

type ChatResponse struct {
	Payload string
}

func NewChatSession(chat *models.Chat, mode ChatMode, db *gorm.DB, svc *services.Service, withFilters []*models.Filter, userID *uint, sessionID *string) (*ChatSession, error) {
	uid, _ := uuid.NewV4()
	id := uid.String()

	// override ID if set so we can retain the chat history
	if sessionID != nil {
		id = *sessionID
	}

	cs := &ChatSession{
		id:             id,
		chatRef:        chat,
		input:          make(chan *models.UserMessage, 100),
		outputMessages: make(chan *ChatResponse, 100),
		outputStream:   make(chan []byte, 100),
		stop:           make(chan struct{}),
		errors:         make(chan error, 100),
		preProcessors:  []func(*models.UserMessage) error{},
		mode:           mode,
		db:             db,
		datasources:    map[uint]*models.Datasource{},
		tools:          map[string]models.Tool{},
		llmResponses:   make(chan *LLMResponseWrapper, 100),
		service:        svc,
		files:          map[string]string{},
		userID:         *userID,
	}

	// Perform initial privacy check
	if len(cs.datasources) > 0 || len(cs.tools) > 0 {
		if err := cs.validatePrivacyScores(); err != nil {
			return nil, fmt.Errorf("initial privacy score validation failed: %v", err)
		}
	}

	// filter setup
	preProcessors := []func(*models.UserMessage) error{}
	for i, _ := range withFilters {
		sr := scripting.NewScriptRunner(withFilters[i].Script)
		asFunc := func(m *models.UserMessage) error {
			return sr.RunFilter(m.Payload)
		}

		preProcessors = append(preProcessors, asFunc)
	}

	return cs, nil
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

func (cs *ChatSession) Input() chan *models.UserMessage {
	return cs.input
}

func (cs *ChatSession) AddDatasource(id uint) error {
	ds := models.Datasource{}
	err := ds.Get(cs.db, id)
	if err != nil {
		return fmt.Errorf("error getting datasource: %v", err)
	}

	cs.datasources[id] = &ds

	// Validate privacy scores
	if err := cs.validatePrivacyScores(); err != nil {
		// If validation fails, remove the datasource and return the error
		delete(cs.datasources, id)
		return err
	}

	return nil
}

func (cs *ChatSession) RemoveDatasource(id uint) {
	delete(cs.datasources, id)
}

func (cs *ChatSession) AddTool(id string, t models.Tool) error {
	cs.tools[id] = t

	// Validate privacy scores
	if err := cs.validatePrivacyScores(); err != nil {
		// If validation fails, remove the tool and return the error
		delete(cs.tools, id)
		return err
	}

	return nil
}

func (cs *ChatSession) AddFileReference(filename, contents string) {
	cs.files[filename] = contents
}

func (cs *ChatSession) GetFileReference(filename string) (string, bool) {
	contents, ok := cs.files[filename]
	return contents, ok
}

func (cs *ChatSession) RemoveTool(id string) {
	delete(cs.tools, id)
}

func (cs *ChatSession) CurrentTools() map[string]models.Tool {
	return cs.tools
}

func (cs *ChatSession) AddPreProcessor(fn func(*models.UserMessage) error) {
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
				docs, err := ds.Search(msg.Payload, 5) //TODO this should be configurable in the future
				if err != nil {
					cs.errors <- fmt.Errorf("error searching datasources: %v", err)
					continue
				}

				// prep tools
				tools := cs.prepareTools()

				// Check file references
				files := make(map[string]string)
				if len(msg.FileRef) > 0 {
					for i, _ := range msg.FileRef {
						fileContents, ok := cs.GetFileReference(msg.FileRef[i])
						if !ok {
							cs.errors <- fmt.Errorf("file reference not found: %s", msg.FileRef[i])
							continue
						}
						files[msg.FileRef[i]] = fileContents
					}
				}

				// Handle the message from the user
				_, err = cs.HandleUserMessage(msg, docs, tools, files)
				if err != nil {
					cs.errors <- fmt.Errorf("error handling user message: %v", err)
					continue
				}

			case resp := <-cs.llmResponses:
				// handle any response from the LLM
				err := cs.HandleLLMResponse(resp)
				if err != nil {
					cs.errors <- fmt.Errorf("error handling LLM response: %v", err)
					continue
				}
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
				schemaName := t.AuthSchemaName
				if schemaName == "" {
					schemaName = "apiKey"
				}
				opts = append(opts, universalclient.WithAuth(schemaName, t.AuthKey))
			}

			uc, err := universalclient.NewClient([]byte(t.OASSpec), "", opts...)
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
	if cs.db == nil {
		return fmt.Errorf("no database connection")
	}

	if cs.chatRef == nil {
		return fmt.Errorf("no chat reference")
	}

	if cs.chatRef.LLMSettings == nil {
		return fmt.Errorf("no LLM settings")
	}

	cs.chatHistory = NewGormChatMessageHistory(cs.db, cs.id, cs.chatRef.ID, cs.userID, cs.chatRef.LLMSettings.SystemPrompt)

	// create the LLM client
	llm, err := cs.fetchDriver(nil)
	if err != nil {
		return err
	}

	cs.caller = llm

	// Validate privacy scores
	if err := cs.validatePrivacyScores(); err != nil {
		return fmt.Errorf("privacy score validation failed: %v", err)
	}

	return nil
}

func (cs *ChatSession) fetchDriver(mem schema.Memory) (llms.Model, error) {
	llm, err := switches.FetchDriver(cs.chatRef.LLM, cs.chatRef.LLMSettings, mem, cs.streamingFunc)
	return llm, err
}

func (cs *ChatSession) preProcessMessage(msg *models.UserMessage) error {
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
	pl := fmt.Sprintf("Context for this message: \n%s\nMessage: \n%s", cs.joinDocuments(docs, "\n\n"), payload)
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

func (cs *ChatSession) HandleLLMResponse(w *LLMResponseWrapper) error {
	resp := w.Response
	if len(resp.Choices) == 0 {
		cs.sendError(fmt.Errorf("no choices in response"))
		return nil
	}

	toolCall := false
	toolCallRequest := llms.MessageContent{
		Role:  llms.ChatMessageTypeAI,
		Parts: []llms.ContentPart{},
	}

	toolCallResult := llms.MessageContent{
		Role:  llms.ChatMessageTypeTool,
		Parts: []llms.ContentPart{},
	}

	content := ""
	for _, reply := range resp.Choices {
		if reply.Content != "" {
			content = reply.Content
			//cs.sendOutput(reply.Content)
		}

		if len(reply.ToolCalls) > 0 {
			_, err := cs.handleToolCalls(reply, &toolCallRequest, &toolCallResult)
			if err != nil {
				cs.sendError(fmt.Errorf("error handling tool calls: %v", err))
				continue
			}

			toolCall = true
		}
	}

	if toolCall {
		// add the whole tool call to history
		ctx := context.Background()
		err := cs.chatHistory.AddMessage(ctx, toolCallRequest)
		if err != nil {
			cs.sendError(fmt.Errorf("error adding tool call to history: %v", err))
			return err
		}

		// add the tool results to the history
		err = cs.chatHistory.AddMessage(ctx, toolCallResult)
		if err != nil {
			cs.sendError(fmt.Errorf("error adding tool call to history: %v", err))
			return err
		}

		history, err := cs.getMessages()
		if err != nil {
			cs.sendError(fmt.Errorf("error getting chat history after tool call: %v", err))
			return err
		}

		toolCallResp, err := cs.caller.GenerateContent(ctx, history, w.Opts...)
		if err != nil {
			cs.sendError(fmt.Errorf("[toolcall] error generating content after tool call: %v", err))
			return err
		}

		cs.llmResponses <- &LLMResponseWrapper{Response: toolCallResp, Opts: w.Opts}

	}

	if content != "" {
		// Acknowlkedge the AI message
		cs.sendOutput(content)
		if !toolCall {
			// only store non-tool call MESSAGES from the AI as AI messages
			err := cs.chatHistory.AddAIMessage(context.Background(), content)
			if err != nil {
				cs.sendError(fmt.Errorf("error adding AI message to history: %v", err))
				return err
			}
		}
	}

	return nil
}

func (cs *ChatSession) HandleUserMessage(msg *models.UserMessage, docs []schema.Document, tools []llms.Tool, files map[string]string) (*llms.ContentResponse, error) {
	opts := cs.getOptions(cs.chatRef.LLMSettings, tools)
	if cs.caller == nil {
		return nil, fmt.Errorf("LLM driver is not initialized")
	}

	ctx, done := context.WithTimeout(context.Background(), 300*time.Second)
	defer done()

	if len(files) > 0 {
		if docs == nil {
			docs = []schema.Document{}
		}

		for fName, _ := range files {
			newDoc := schema.Document{
				PageContent: fmt.Sprintf("File: %s \n %s", fName, files[fName]),
			}
			docs = append(docs, newDoc)
		}
	}

	pl := cs.prepHumanMessage(msg.Payload, docs).Content
	err := cs.chatHistory.AddUserMessage(context.Background(), pl)
	if err != nil {
		return nil, fmt.Errorf("error adding message to history: %v", err)
	}

	messages, err := cs.getMessages()
	if err != nil {
		return nil, fmt.Errorf("error getting chat history: %v", err)
	}

	resp, err := cs.caller.GenerateContent(ctx, messages, opts...)
	if err != nil {
		return nil, fmt.Errorf("[userMessage handler] error generating content: %v", err)
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled")
	case cs.llmResponses <- &LLMResponseWrapper{Response: resp, Opts: opts}:
	default:
		return nil, fmt.Errorf("could not send response to llm responses channel")
	}

	mc := llms.TextParts(llms.ChatMessageTypeHuman, pl)
	analytics.RecordContentMessage(
		&mc,
		resp,
		cs.chatRef.LLM.Vendor,
		cs.chatRef.LLMSettings.ModelName,
		cs.ID(),
		0,
		0,
		0,
		time.Now(),
		cs.service,
	)

	return resp, nil
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
		switch k {
		case "body":
			bodyMap, ok := v.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("expected 'body' to be a JSON object")
			}
			actualParams.Body = bodyMap

		case "headers":
			headersMap, ok := v.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("expected 'headers' to be a JSON object")
			}
			for hk, hv := range headersMap {
				headerValues, err := interfaceToStrings(hv)
				if err != nil {
					return nil, fmt.Errorf("error processing header %s: %v", hk, err)
				}
				actualParams.Headers[hk] = headerValues
			}

		case "parameters":
			paramsMap, ok := v.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("expected 'parameters' to be a JSON object")
			}
			for pk, pv := range paramsMap {
				paramValues, err := interfaceToStrings(pv)
				if err != nil {
					return nil, fmt.Errorf("error processing parameter %s: %v", pk, err)
				}
				actualParams.Parameters[pk] = paramValues
			}

		default:
			paramValues, err := interfaceToStrings(v)
			if err != nil {
				return nil, fmt.Errorf("error converting parameter %s: %v", k, err)
			}
			actualParams.Parameters[k] = paramValues
		}
	}

	return actualParams, nil
}

func interfaceToStrings(value interface{}) ([]string, error) {
	switch v := value.(type) {
	case string:
		return []string{v}, nil
	case []interface{}:
		var strs []string
		for _, item := range v {
			s, err := interfaceToString(item)
			if err != nil {
				return nil, err
			}
			strs = append(strs, s)
		}
		return strs, nil
	case []string:
		return v, nil
	default:
		s, err := interfaceToString(v)
		if err != nil {
			return nil, err
		}
		return []string{s}, nil
	}
}

func interfaceToString(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case bool:
		return strconv.FormatBool(v), nil
	default:
		return "", fmt.Errorf("cannot convert type %T to string", value)
	}
}

func (cs *ChatSession) handleToolCalls(choice *llms.ContentChoice, toolCall, toolResult *llms.MessageContent) (bool, error) {
	called := false

	for i, _ := range choice.ToolCalls {
		t := choice.ToolCalls[i]

		toolCall.Parts = append(toolCall.Parts, llms.ToolCall{
			ID:   t.ID,
			Type: t.Type,
			FunctionCall: &llms.FunctionCall{
				Name:      t.FunctionCall.Name,
				Arguments: t.FunctionCall.Arguments,
			},
		})

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
				schemaName := toolDef.AuthSchemaName
				if toolDef.AuthSchemaName == "" {
					schemaName = "apiKey"
				}

				opts = append(opts, universalclient.WithAuth(schemaName, toolDef.AuthKey))
			}

			opts = append(opts, universalclient.WithResponseFormat(universalclient.ResponseFormatJSON))

			uc, err := universalclient.NewClient([]byte(toolDef.OASSpec), "", opts...)
			if err != nil {
				return false, fmt.Errorf("error creating tool client: %v", err)
			}

			t0 := time.Now()
			args, err := cs.convertLLMArgsToUniversalClientInputs([]byte(t.FunctionCall.Arguments), t.FunctionCall.Name, uc)
			if err != nil {
				return false, fmt.Errorf("error converting LLM args to universal client inputs: %v", err)
			}

			cs.sendOutput(fmt.Sprintf("[TOOL] calling [%s]", t.FunctionCall.Name))
			resp, err := uc.CallOperation(t.FunctionCall.Name, args.Parameters, args.Body, args.Headers)
			if err != nil {
				return false, fmt.Errorf("error calling tool operation [%s]: %v", t.FunctionCall.Name, err)
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

			t1 := time.Now()

			toolResp := llms.ToolCallResponse{
				ToolCallID: t.ID,
				Name:       t.FunctionCall.Name,
				Content:    asStr,
			}

			toolResult.Parts = append(toolResult.Parts, toolResp)
			called = true

			analytics.RecordToolCall(
				t.FunctionCall.Name,
				time.Now(),
				int(t1.Sub(t0).Milliseconds()), toolDef.ID)
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

func (cs *ChatSession) validatePrivacyScores() error {
	var maxLLMScore int
	var maxDataSourceScore int = 0 // Initialize with a value higher than the maximum possible score

	// Get LLM privacy score
	if cs.chatRef.LLM != nil {
		maxLLMScore = cs.chatRef.LLM.PrivacyScore
	}

	// Check datasources
	for _, ds := range cs.datasources {
		if ds.PrivacyScore > maxDataSourceScore {
			maxDataSourceScore = ds.PrivacyScore
		}
	}

	// Check tools (assuming tools have a PrivacyScore field)
	for _, tool := range cs.tools {
		if tool.PrivacyScore > maxDataSourceScore {
			maxDataSourceScore = tool.PrivacyScore
		}
	}

	if maxDataSourceScore > maxLLMScore {
		return fmt.Errorf("datasource or tool privacy score (%d) is higher than LLM privacy score (%d)", maxDataSourceScore, maxLLMScore)
	}

	return nil
}

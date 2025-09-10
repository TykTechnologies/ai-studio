package chat_session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/config"
	dataSession "github.com/TykTechnologies/midsommar/v2/data_session"
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/TykTechnologies/midsommar/v2/universalclient"
	"github.com/gofrs/uuid"
	"github.com/pkoukk/tiktoken-go"
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
	id            string
	chatRef       *models.Chat
	chatHistory   *GormChatMessageHistory
	input         chan *models.UserMessage
	queue         MessageQueue // NEW: Replaces llmResponses, outputMessages, outputStream, errors
	stop          chan struct{}
	preProcessors []func(*models.UserMessage) error
	caller        llms.Model
	mode          ChatMode
	datasources   map[uint]*models.Datasource
	tools         map[string]models.Tool
	db            *gorm.DB
	service       *services.Service
	userID        uint
	files         map[string]string
	filters       []*models.Filter
}

type ChatResponse struct {
	Payload string
}

func NewChatSession(chat *models.Chat, mode ChatMode, db *gorm.DB, svc *services.Service, withFilters []*models.Filter, userID *uint, sessionID *string, queueFactory ...QueueFactory) (*ChatSession, error) {
	uid, _ := uuid.NewV4()
	id := uid.String()

	// override ID if set so we can retain the chat history
	if sessionID != nil {
		id = *sessionID
	}

	// Create queue using factory or default
	var queue MessageQueue
	var err error

	if len(queueFactory) > 0 && queueFactory[0] != nil {
		// Use provided factory
		queue, err = queueFactory[0].CreateQueue(id, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create queue: %w", err)
		}
	} else {
		// Use default factory with shared database connection to prevent connection exhaustion
		factory := CreateDefaultQueueFactoryWithSharedDB(db)
		queue, err = factory.CreateQueue(id, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create queue with shared database: %w", err)
		}
	}

	cs := &ChatSession{
		id:            id,
		chatRef:       chat,
		input:         make(chan *models.UserMessage, 100),
		queue:         queue, // Use MessageQueue interface
		stop:          make(chan struct{}),
		preProcessors: []func(*models.UserMessage) error{},
		mode:          mode,
		db:            db,
		datasources:   map[uint]*models.Datasource{},
		tools:         map[string]models.Tool{},
		service:       svc,
		files:         map[string]string{},
		userID:        *userID,
		filters:       withFilters,
	}

	// filter setup
	preProcessors := []func(*models.UserMessage) error{}
	for i, _ := range withFilters {
		sr := scripting.NewScriptRunner(withFilters[i].Script)
		asFunc := func(m *models.UserMessage) error {
			return sr.RunFilter(m.Payload, cs.service)
		}

		preProcessors = append(preProcessors, asFunc)
	}

	cs.preProcessors = preProcessors

	return cs, nil
}

func (cs *ChatSession) ID() string {
	return cs.id
}

func (cs *ChatSession) Errors() <-chan error {
	return cs.queue.ConsumeErrors(context.Background())
}

func (cs *ChatSession) OutputMessage() <-chan *ChatResponse {
	return cs.queue.ConsumeMessages(context.Background())
}

func (cs *ChatSession) OutputStream() <-chan []byte {
	return cs.queue.ConsumeStream(context.Background())
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

	entitlements, err := cs.service.GetUserEntitlements(cs.userID)
	if err != nil {
		return fmt.Errorf("error getting user entitlements: %v", err)
	}

	if !entitlements.HasDataSourceAccess(ds.ID) {
		return fmt.Errorf("user does not have access to datasource %s", ds.Name)
	}

	cs.datasources[id] = &ds

	// Validate privacy scores
	if err := cs.validatePrivacyScores(); err != nil {
		// If validation fails, remove the datasource and return the error
		delete(cs.datasources, id)
		return err
	}

	cs.sendStatus(fmt.Sprintf("Datasource '%s' added to room", ds.Name))

	return nil
}

func (cs *ChatSession) RemoveDatasource(id uint) {
	ds, ok := cs.datasources[id]
	if !ok {
		return
	}

	delete(cs.datasources, id)
	cs.sendStatus(fmt.Sprintf("Datasource '%s' removed from room", ds.Name))
}

func (cs *ChatSession) AddTool(id string, t models.Tool) error {
	entitlements, err := cs.service.GetUserEntitlements(cs.userID)
	if err != nil {
		return fmt.Errorf("error getting user entitlements: %v", err)
	}

	if !entitlements.HasToolAccess(t.ID) {
		return fmt.Errorf("user does not have access to tool %s", t.Name)
	}

	// Validate privacy scores
	if err := cs.validatePrivacyScores(); err != nil {
		// If validation fails, remove the tool and return the error
		delete(cs.tools, id)
		return err
	}

	slog.Info("tool added to chat", "tool", t.Name)
	cs.sendStatus(fmt.Sprintf("Tool '%s' added to room", t.Name))

	for i, _ := range t.FileStores {
		// base64 decode the file contents first
		content, err := base64.StdEncoding.DecodeString(t.FileStores[i].Content)
		if err != nil {
			return fmt.Errorf("error decoding file contents: %v", err)
		}

		pl := fmt.Sprintf("[CONTEXT]\nThe following additional documentation file '%s' has been provided for the tool '%s' to help you use it:\n%s\n[/CONTEXT]",
			t.FileStores[i].FileName,
			t.Name,
			content)

		err = cs.chatHistory.AddUserMessage(context.Background(), pl)
		if err != nil {
			return fmt.Errorf("error adding message to history: %v", err)
		}

	}

	if len(t.Dependencies) > 0 {
		slog.Info("tool has dependencies", "count", len(t.Dependencies))
		for i, _ := range t.Dependencies {
			dep, err := cs.service.GetToolByID(t.Dependencies[i].ID)
			if err != nil {
				return fmt.Errorf("error getting tool dependency: %v", err)
			}

			dep.OASSpec, err = helpers.DecodeToUTF8(dep.OASSpec)
			err = cs.AddTool(
				dep.Name,
				*dep)
			if err != nil {
				return fmt.Errorf("error adding tool dependency: %v", err)
			}
		}
	}

	cs.tools[id] = t

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
	t, ok := cs.tools[id]
	if !ok {
		return
	}

	delete(cs.tools, id)
	cs.sendStatus(fmt.Sprintf("Tool '%s' removed from room", t.Name))
}

func (cs *ChatSession) CurrentTools() map[string]models.Tool {
	return cs.tools
}

// GetCurrentDatasources returns a slice of current datasources
func (cs *ChatSession) GetCurrentDatasources() []*models.Datasource {
	datasources := make([]*models.Datasource, 0, len(cs.datasources))
	for _, ds := range cs.datasources {
		datasources = append(datasources, ds)
	}
	return datasources
}

// NotifyStatus sends a status message through the chat session
func (cs *ChatSession) NotifyStatus(status string) {
	cs.sendStatus(status)
}

func (cs *ChatSession) AddPreProcessor(fn func(*models.UserMessage) error) {
	cs.preProcessors = append(cs.preProcessors, fn)
}

func (cs *ChatSession) Stop() {
	cs.stop <- struct{}{}
	close(cs.input)
	cs.queue.Close() // Close the queue instead of individual channels
}

func (cs *ChatSession) Start() error {
	if err := cs.initSession(); err != nil {
		slog.Error("Failed to initialize chat session", "error", err)
		return fmt.Errorf("error initializing chat session: %v", err)
	}

	if cs.chatRef.LLMSettings == nil {
		slog.Error("LLM settings is nil")
		return fmt.Errorf("LLM settings not configured")
	}

	if cs.chatRef.LLM == nil {
		slog.Error("LLM is nil")
		return fmt.Errorf("LLM not configured")
	}

	slog.Info("Chat session configuration",
		"llm_settings_id", cs.chatRef.LLMSettingsID,
		"llm_id", cs.chatRef.LLMID,
		"llm_settings", cs.chatRef.LLMSettings != nil,
		"llm", cs.chatRef.LLM != nil)

	err := cs.handleDefaults()
	if err != nil {
		return fmt.Errorf("error handling defaults: %v", err)
	}

	go func() {
		for {
			select {
			case <-cs.stop:
				return
			case msg := <-cs.input:
				err := cs.preProcessMessage(msg)
				if err != nil {
					cs.sendStatus(fmt.Sprintf("Content guideline violation detected. This request cannot be processed."))
					continue
				}

				// handle RAG
				n := cs.chatRef.RagResultsPerSource
				if n == 0 {
					n = 10
				}
				ds := dataSession.NewDataSession(cs.datasources)
				docs, err := ds.Search(msg.Payload, 10) //TODO this should be configurable in the future
				if err != nil {
					cs.sendError(fmt.Errorf("error searching datasources: %v", err))
					continue
				}

				// prep tools
				tools := cs.prepareTools()

				// secure file references
				scanFailureResponse, ok := cs.scanFiles(msg.FileRef)
				if !ok {
					cs.sendError(fmt.Errorf(scanFailureResponse))
					continue
				}

				// Add file references
				files := make(map[string]string)
				if len(msg.FileRef) > 0 {
					for i, _ := range msg.FileRef {
						fileContents, ok := cs.GetFileReference(msg.FileRef[i])
						if !ok {
							cs.sendError(fmt.Errorf("file reference not found: %s", msg.FileRef[i]))
							continue
						}
						files[msg.FileRef[i]] = fileContents
					}
				}

				// Handle the message from the user
				_, err = cs.HandleUserMessage(msg, docs, tools, files)
				if err != nil {
					cs.sendError(fmt.Errorf("error handling user message: %v", err))
					continue
				}

			case resp := <-cs.queue.ConsumeLLMResponses(context.Background()):
				// handle any response from the LLM
				err := cs.HandleLLMResponse(resp)
				if err != nil {
					cs.sendError(fmt.Errorf("error handling LLM response: %v", err))
					continue
				}
			}
		}
	}()

	return nil
}

func (cs *ChatSession) sendOutput(resp string) {
	// Disabled to prevent duplicate messages
}

func (cs *ChatSession) sendStatus(resp string) {
	msg := fmt.Sprintf(":::system %s:::", resp)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Send only to message channel to avoid duplicate messages
	// The SSE handler processes this via OutputMessage() which creates system-type messages
	cs.queue.PublishMessage(ctx, &ChatResponse{Payload: msg})
}

func (cs *ChatSession) sendError(err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if queueErr := cs.queue.PublishError(ctx, err); queueErr != nil {
		slog.Error("error sending error to queue", "queue_error", queueErr, "original_error", err)
	}
}

func (cs *ChatSession) prepareTools() []llms.Tool {
	tools := make([]llms.Tool, 0)
	ids := make(map[string]struct{})
	for _, t := range cs.tools {
		// make sure only unique tool names go into the final array
		if _, ok := ids[t.Name]; ok {
			continue
		}

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
				cs.sendError(fmt.Errorf("error creating universal client: %v", err))
				continue
			}

			if len(t.GetOperations()) > 0 {
				asToolDef, err := uc.AsTool(t.GetOperations()...)
				if err != nil {
					cs.sendError(fmt.Errorf("error creating tool definition: %v", err))
					continue
				}

				tools = append(tools, asToolDef...)
				ids[t.Name] = struct{}{}
			}

		default:
			cs.sendError(fmt.Errorf("unknown tool type: %s", t.ToolType))
		}

	}

	return tools
}

func (cs *ChatSession) getSystemPrompt() string {
	// allow override of system prompt in chat room config
	prompt := cs.chatRef.LLMSettings.SystemPrompt
	if cs.chatRef.SystemPrompt != "" {
		prompt = cs.chatRef.SystemPrompt
	}

	if len(cs.chatRef.ExtraContext) > 0 {
		contextStr := "I have provided additional context for this chat session. Please review the following information and bear it in mind for every interaction:"
		for i := range cs.chatRef.ExtraContext {
			contextStr = fmt.Sprintf("%s\n\n## File Name: %s \n\n## File Content:\n\n %s",
				contextStr,
				cs.chatRef.ExtraContext[i].FileName,
				cs.chatRef.ExtraContext[i].Content)
		}

		prompt = fmt.Sprintf("%s\n\n%s", contextStr, prompt)
	}

	return prompt
}

func (cs *ChatSession) initSession() error {
	// History for the chat session
	if cs.db == nil {
		return fmt.Errorf("no database connection")
	}

	if cs.chatRef == nil {
		slog.Error("Chat reference is nil")
		return fmt.Errorf("no chat reference")
	}

	slog.Info("Chat reference loaded",
		"llm_settings_id", cs.chatRef.LLMSettingsID,
		"llm_id", cs.chatRef.LLMID)

	if cs.chatRef.LLMSettings == nil {
		slog.Error("LLM settings is nil")
		return fmt.Errorf("no LLM settings")
	}

	if cs.chatRef.LLM == nil {
		slog.Error("LLM is nil")
		return fmt.Errorf("no LLM configuration")
	}

	slog.Info("LLM configuration",
		"vendor", cs.chatRef.LLM.Vendor,
		"model", cs.chatRef.LLMSettings.ModelName)

	cs.chatHistory = NewGormChatMessageHistory(cs.db, cs.id, cs.chatRef.ID, cs.userID, cs.getSystemPrompt())

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

func (cs *ChatSession) handleDefaults() error {
	// auto-load the default
	if cs.chatRef.DefaultDataSource != nil {
		err := cs.AddDatasource(cs.chatRef.DefaultDataSource.ID)
		if err != nil {
			return fmt.Errorf("error adding default datasource to chat session: %v", err)
		}
	}

	if cs.chatRef.DefaultTools != nil {
		for i, _ := range cs.chatRef.DefaultTools {
			toolDef, err := cs.service.GetToolByID(cs.chatRef.DefaultTools[i].ID)
			if err != nil {
				return fmt.Errorf("error getting default tool definition: %v", err)
			}

			toolDef.OASSpec, err = helpers.DecodeToUTF8(toolDef.OASSpec)
			err = cs.AddTool(
				toolDef.Name,
				*toolDef)
			if err != nil {
				return fmt.Errorf("error adding default tool to chat session: %v", err)
			}
		}
	}

	// Perform initial privacy check
	if len(cs.datasources) > 0 || len(cs.tools) > 0 {
		if err := cs.validatePrivacyScores(); err != nil {
			return fmt.Errorf("privacy score validation failed: %v", err)
		}
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

func (cs *ChatSession) scanFiles(refs []string) (string, bool) {
	for i, _ := range refs {
		content, ok := cs.GetFileReference(refs[i])
		if ok {
			for i2, _ := range cs.filters {
				sr := scripting.NewScriptRunner(cs.filters[i2].Script)
				if sr == nil {
					cs.sendError(fmt.Errorf("error creating script runner"))
					continue
				}

				if err := sr.RunFilter(content, cs.service); err != nil {
					return fmt.Sprintf("filter denied content in %s", refs[i]), false
				}
			}

		}
	}

	return "", true
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

func isToolCaller(name string) bool {
	lowerName := strings.ToLower(name)
	toolCallers := []string{"gpt", "claude", "gemini"}
	for _, tc := range toolCallers {
		if strings.Contains(lowerName, tc) {
			return true
		}
	}

	return false
}

func (cs *ChatSession) prepHumanMessage(payload string, docs []schema.Document) llms.HumanChatMessage {
	pl := payload
	if len(docs) > 0 {
		pl = fmt.Sprintf("[CONTEXT]\nContext for this message: \n%s\n[/CONTEXT]\n%s", cs.joinDocuments(docs, "\n\n"), payload)
	}

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

func handleEcho(prefix string, dat interface{}) {
	prettyJSON, err := json.MarshalIndent(dat, "", "    ")
	if err != nil {
		slog.Error("error echoing response to stdout", "error", err)
	} else {
		slog.Info("[CONV ECHO]", prefix, "")
		fmt.Println("--------------------------------------------------")
		fmt.Printf("%s\n", string(prettyJSON))
	}
}

func extractEmbeddedToolCalls(content string) (string, []llms.ToolCall) {
	regex := regexp.MustCompile(`(?s)\s*tool_use\s*\n?(.*?)\s*/tool_use\s*`)

	matches := regex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return content, nil
	}

	var toolCalls []llms.ToolCall

	for i, match := range matches {
		if len(match) <= 1 {
			continue
		}

		toolCallJSON := strings.TrimSpace(match[1])
		slog.Info("Found embedded tool_use block",
			"index", i,
			"match_length", len(match[0]),
			"tool_call_json", toolCallJSON)

		var toolCallData struct {
			Function struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"function"`
			ToolCallID string `json:"tool_call_id"`
			Type       string `json:"type"`
		}

		if err := json.Unmarshal([]byte(toolCallJSON), &toolCallData); err != nil {
			slog.Error("Error unmarshaling embedded tool call", "index", i, "error", err)
			continue
		}

		toolCall := llms.ToolCall{
			ID:   toolCallData.ToolCallID,
			Type: toolCallData.Type,
			FunctionCall: &llms.FunctionCall{
				Name:      toolCallData.Function.Name,
				Arguments: string(toolCallData.Function.Arguments),
			},
		}

		toolCalls = append(toolCalls, toolCall)
	}

	result := regex.ReplaceAllString(content, " ")

	return result, toolCalls
}

func (cs *ChatSession) HandleLLMResponse(w *LLMResponseWrapper) error {
	if config.Get().EchoConversation {
		handleEcho("LLM", w.Response)
	}
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
			cs.handleToolCalls(reply, &toolCallRequest, &toolCallResult)
			toolCall = true
		}
	}

	ctx := context.Background()

	if content != "" {
		// For regular messages without tool calls
		err := cs.chatHistory.AddAIMessage(ctx, content)
		if err != nil {
			cs.sendError(fmt.Errorf("error adding AI message to history: %v", err))
			return err
		}
		// Only send to output stream if this is not a tool call
		if !toolCall {
			// Send message via queue to both message and stream channels
			msgCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
			defer cancel()

			// Send as ChatResponse message only
			// Note: We don't send to PublishStream here because streaming is complete
			// and sending to both channels causes duplicate messages in the SSE handler
			if err := cs.queue.PublishMessage(msgCtx, &ChatResponse{Payload: content}); err != nil {
				slog.Warn("failed to publish message to queue", "session_id", cs.id, "error", err)
			}
		}
	}

	if toolCall {
		// Get final response from LLM with tool results
		history, err := cs.getMessages()
		if err != nil {
			cs.sendError(fmt.Errorf("error getting chat history after tool call: %v", err))
			return err
		}

		err = cs.chatHistory.AddMessage(ctx, toolCallRequest)
		if err != nil {
			cs.sendError(fmt.Errorf("error adding tool call to history: %v", err))
			return err
		}

		err = cs.chatHistory.AddMessage(ctx, toolCallResult)
		if err != nil {
			cs.sendError(fmt.Errorf("error adding tool call to history: %v", err))
			return err
		}

		// Get updated history with tool call and result
		history, err = cs.getMessages()
		if err != nil {
			cs.sendError(fmt.Errorf("error getting updated history: %v", err))
			return err
		}

		// Regenerate options from current session state instead of using w.Opts
		// This ensures we have the correct tools and settings after NATS deserialization
		tools := cs.prepareTools()
		currentOpts := cs.getOptions(cs.chatRef.LLMSettings, tools)

		// Check token length and get LLM response based on updated history
		history = cs.PreflightTokenLengthCheck(history)
		resp, err := cs.caller.GenerateContent(ctx, history, currentOpts...)
		if err != nil {
			cs.sendError(fmt.Errorf("error getting LLM response after tool call: %v", err))
			return err
		}

		// Send the new LLM response to continue the conversation
		// Note: We still use nil for Opts since they'll be regenerated when needed
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		cs.queue.PublishLLMResponse(ctx, &LLMResponseWrapper{Response: resp, Opts: nil})
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

	if config.Get().EchoConversation {
		type ComboObj struct {
			Message *models.UserMessage
			Docs    []schema.Document
		}

		handleEcho("USER", ComboObj{Message: msg, Docs: docs})
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

	// need to make sure we stay in content window
	messages = cs.PreflightTokenLengthCheck(messages)

	resp, err := cs.caller.GenerateContent(ctx, messages, opts...)
	if err != nil {
		return nil, fmt.Errorf("[userMessage handler] error generating content: %v", err)
	}

	// Send LLM response to queue with timeout
	publishCtx, publishCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer publishCancel()

	if err := cs.queue.PublishLLMResponse(publishCtx, &LLMResponseWrapper{Response: resp, Opts: opts}); err != nil {
		return nil, fmt.Errorf("could not send response to llm responses queue: %v", err)
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled")
	default:
		// Continue normally
	}

	// Try to generate a title for this chat if appropriate
	cs.maybeGenerateTitle(msg.Payload)

	mc := llms.TextParts(llms.ChatMessageTypeHuman, pl)
	analytics.RecordContentMessage(
		&mc,
		resp,
		cs.chatRef.LLM.Vendor,
		cs.chatRef.LLMSettings.ModelName,
		strconv.Itoa(int(cs.chatRef.ID)),
		0,
		cs.userID,
		0,
		cs.chatRef.LLMID,
		time.Now(),
		cs.service,
	)

	return resp, nil
}

// generateChatTitle uses the LLM to generate a concise title based on the user's message
func (cs *ChatSession) generateChatTitle(userMessage string) (string, error) {
	if cs.caller == nil {
		return "", fmt.Errorf("LLM driver is not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a simple prompt for title generation
	titlePrompt := fmt.Sprintf(`Based on the following user message, generate a short, descriptive title (maximum 8 words) that captures the main topic or intent. Only return the title, nothing else.

User message: %s`, userMessage)

	// Create messages for the title generation request
	messages := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, titlePrompt),
	}

	// Use minimal options for title generation (no tools, etc.)
	opts := []llms.CallOption{
		llms.WithMaxTokens(50), // Keep response short
		llms.WithTemperature(0.7),
	}

	resp, err := cs.caller.GenerateContent(ctx, messages, opts...)
	if err != nil {
		return "", fmt.Errorf("error generating chat title: %v", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in title generation response")
	}

	title := strings.TrimSpace(resp.Choices[0].Content)
	
	// Clean up the title - remove quotes and limit length
	title = strings.Trim(title, `"'`)
	if len(title) > 60 {
		title = title[:57] + "..."
	}

	return title, nil
}

// maybeGenerateTitle checks if we should generate a title and does so if needed
func (cs *ChatSession) maybeGenerateTitle(userMessage string) {
	// Get the chat history record
	exists, historyRecord, err := cs.chatHistory.CheckIfSessionExists(context.Background())
	if err != nil {
		slog.Error("failed to check session for title generation", "error", err, "session_id", cs.id)
		return
	}

	if !exists || historyRecord == nil {
		return // No record found
	}

	// Check if we should generate a title
	if !historyRecord.ShouldGenerateTitle(userMessage) {
		return
	}

	// Generate the title asynchronously to avoid blocking the main chat flow
	go func() {
		title, err := cs.generateChatTitle(userMessage)
		if err != nil {
			slog.Error("failed to generate chat title", "error", err, "session_id", cs.id)
			return
		}

		// Update the chat history record with the new title
		if err := historyRecord.UpdateName(cs.db, title); err != nil {
			slog.Error("failed to update chat title", "error", err, "session_id", cs.id, "title", title)
			return
		}

		// Mark as title generated
		if err := historyRecord.MarkTitleGenerated(cs.db); err != nil {
			slog.Error("failed to mark title as generated", "error", err, "session_id", cs.id)
			return
		}

		slog.Info("successfully generated chat title", "session_id", cs.id, "title", title)
	}()
}

type CallParams struct {
	Body       map[string]interface{} `json:"body"`
	Headers    map[string][]string    `json:"headers"`
	Parameters map[string][]string    `json:"parameters"`
}

func (cs *ChatSession) PreflightTokenLengthCheck(msgs []llms.MessageContent) []llms.MessageContent {
	maxInputTokens := cs.chatRef.LLMSettings.MaxLength
	removed := 0
	for {
		tokenLength := cs.estimateTokenLength(msgs)
		slog.Info("preflight token count", "estiamte", tokenLength)
		if tokenLength <= maxInputTokens {
			break
		}

		// Keep the first message (system prompt) and remove the second message if we have enough messages
		if len(msgs) >= 3 {
			msgs = append(msgs[:1], msgs[2:]...)
			removed++
		} else {
			// Not enough messages to remove while keeping system prompt
			break
		}
	}

	fmt.Println("REMOVED", removed, "messages to stay within token limit")

	return msgs
}

func (cs *ChatSession) estimateTokenLength(msgs []llms.MessageContent) int {
	encoding := "cl100k_base"
	tke, err := tiktoken.GetEncoding(encoding)
	if err != nil {
		slog.Error("error getting encoding", "error", err)
		return -1
	}

	// encode
	total := 0
	for _, m := range msgs {
		for _, p := range m.Parts {
			switch p.(type) {
			case llms.TextContent:
				text := p.(llms.TextContent).Text
				token := tke.Encode(text, nil, nil)
				total += len(token)
			case llms.ToolCallResponse:
				text := p.(llms.ToolCallResponse).Content
				token := tke.Encode(text, nil, nil)
				total += len(token)
			}
		}
	}

	return total
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

func (cs *ChatSession) handleToolError(errMsg string, toolCallID string, functionName string, toolResult *llms.MessageContent) {
	cs.sendStatus(errMsg)

	toolResp := llms.ToolCallResponse{
		ToolCallID: toolCallID,
		Name:       functionName,
		Content:    "ERROR: " + errMsg,
	}

	toolResult.Parts = append(toolResult.Parts, toolResp)
}

func (cs *ChatSession) handleToolCalls(choice *llms.ContentChoice, toolCall, toolResult *llms.MessageContent) {
	for i, _ := range choice.ToolCalls {
		t := choice.ToolCalls[i]

		if t.ID == "" {
			continue
		}

		toolCall.Parts = append(toolCall.Parts, llms.ToolCall{
			ID:   t.ID,
			Type: t.Type,
			FunctionCall: &llms.FunctionCall{
				Name:      t.FunctionCall.Name,
				Arguments: t.FunctionCall.Arguments,
			},
		})

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
			errMsg := fmt.Sprintf("tool not found: %s", t.FunctionCall.Name)
			cs.handleToolError(errMsg, t.ID, t.FunctionCall.Name, toolResult)
			continue
		}

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
				errMsg := fmt.Sprintf("error creating tool client: %v", err)
				cs.handleToolError(errMsg, t.ID, t.FunctionCall.Name, toolResult)
				continue
			}

			t0 := time.Now()
			args, err := cs.convertLLMArgsToUniversalClientInputs([]byte(t.FunctionCall.Arguments), t.FunctionCall.Name, uc)
			if err != nil {
				errMsg := fmt.Sprintf("error converting LLM args to universal client inputs: %v", err)
				cs.handleToolError(errMsg, t.ID, t.FunctionCall.Name, toolResult)
				continue
			}

			cs.sendStatus(fmt.Sprintf("Using function: `%s()`", t.FunctionCall.Name))
			cs.sendStatus(fmt.Sprintf("Parameters: `%s`", t.FunctionCall.Arguments))
			if config.Get().EchoConversation {
				slog.Info("[TOOL-CALL]", "[FUNCTION]", t.FunctionCall.Name)
				slog.Info("[TOOL-CALL]", "[PARAMS]", t.FunctionCall.Arguments)
			}

			resp, err := uc.CallOperation(t.FunctionCall.Name, args.Parameters, args.Body, args.Headers)
			if err != nil {
				if config.Get().EchoConversation {
					slog.Info("[TOOL-CALL]", "[ERROR]", err)
				}

				errMsg := fmt.Sprintf("error calling tool operation [%s]: %v", t.FunctionCall.Name, err)
				cs.handleToolError(errMsg, t.ID, t.FunctionCall.Name, toolResult)
				continue
			}

			var asStr string
			switch resp.(type) {
			case []byte:
				asStr = string(resp.([]byte))
			case string:
				asStr = resp.(string)
			default:
				errMsg := fmt.Sprintf("response is not a compatible string (%T)", resp)
				cs.handleToolError(errMsg, t.ID, t.FunctionCall.Name, toolResult)
				continue
			}

			t1 := time.Now()

			if config.Get().EchoConversation {
				fmt.Println("===============================================")
				slog.Info("[TOOL CALL]", "[FUNCTION]", t.FunctionCall.Name)
				fmt.Println(asStr)
				fmt.Println("===============================================")
			}

			for i, _ := range toolDef.Filters {
				filter := toolDef.Filters[i]
				sr := scripting.NewScriptRunner(filter.Script)
				cs.sendStatus(fmt.Sprintf("Running governance filter: `%s`", filter.Name))
				filtered, err := sr.RunMiddleware(asStr, cs.service)
				if err != nil {
					errMsg := fmt.Sprintf("error running governance filter: %v", err)
					cs.handleToolError(errMsg, t.ID, t.FunctionCall.Name, toolResult)
					continue
				}

				asStr = filtered
			}

			toolResp := llms.ToolCallResponse{
				ToolCallID: t.ID,
				Name:       t.FunctionCall.Name,
				Content:    asStr,
			}

			cs.sendStatus(fmt.Sprintf("Function `%s()` returned: `%d` bytes", t.FunctionCall.Name, len(asStr)))
			if config.Get().EchoConversation && len(toolDef.Filters) > 0 {
				slog.Info("[TOOL-CALL]", "[FILTERED]", t.FunctionCall.Name)
				fmt.Println("===============================================")
				fmt.Println(asStr)
				fmt.Println("===============================================")
			}

			toolResult.Parts = append(toolResult.Parts, toolResp)

			analytics.RecordToolCall(
				t.FunctionCall.Name,
				time.Now(),
				int(t1.Sub(t0).Milliseconds()), toolDef.ID)
		}
	}
}

func (cs *ChatSession) streamingFunc(ctx context.Context, chunk []byte) error {
	// Try to parse as JSON to check if it's a final message
	var msg llms.MessageContent
	if err := json.Unmarshal(chunk, &msg); err != nil {
		// Not JSON, send as stream chunk via queue
		streamCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		if queueErr := cs.queue.PublishStream(streamCtx, chunk); queueErr != nil {
			return fmt.Errorf("streaming queue error: %v", queueErr)
		}
	}
	return nil
}

func (cs *ChatSession) getOptions(llmSettings *models.LLMSettings, tools []llms.Tool) []llms.CallOption {
	return llmSettings.GenerateOptionsFromSettings(tools, string(cs.mode), cs.streamingFunc)
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

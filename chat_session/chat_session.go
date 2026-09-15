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
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/config"
	dataSession "github.com/TykTechnologies/midsommar/v2/data_session"
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/secrets"
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
	id                  string
	chatRef             *models.Chat
	chatHistory         *GormChatMessageHistory
	input               chan *models.UserMessage
	queue               MessageQueue // NEW: Replaces llmResponses, outputMessages, outputStream, errors
	stop                chan struct{}
	ctx                 context.Context    // Session-scoped context, cancelled on Stop()
	ctxCancel           context.CancelFunc // Cancels ctx
	preProcessors       []func(*models.UserMessage) error
	caller              llms.Model
	mode                ChatMode
	datasources         map[uint]*models.Datasource
	tools               map[string]models.Tool
	db                  *gorm.DB
	service             *services.Service
	userID              uint
	files               map[string]string
	filters             []*models.Filter
	streamBuffer        string // Accumulated response text for streaming response filters
	streamChunkIndex    int    // Current chunk index for streaming response filters
	streamFilterBlocked bool   // Indicates if streaming was blocked by a filter

	// Lifecycle and v2 event support (see events.go).
	outputMode  OutputMode
	stopOnce    sync.Once
	runMu       sync.Mutex                  // held by a v2 run handler for the whole turn
	stateMu     sync.Mutex                  // guards activeRun, tools, datasources, files
	activeRun   *runState                   // turn being processed by the session goroutine
	subMu       sync.Mutex                  // guards subs
	subs        map[string]*eventSubscriber // run id -> subscriber
	streamMuted int                         // >0 while side calls to the model must not stream (see muteStreaming)
	clientState *clientToolState            // parked client tool calls (see client_tools.go)
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

	ctx, ctxCancel := context.WithCancel(context.Background())

	cs := &ChatSession{
		id:            id,
		chatRef:       chat,
		input:         make(chan *models.UserMessage, 100),
		queue:         queue, // Use MessageQueue interface
		stop:          make(chan struct{}),
		ctx:           ctx,
		ctxCancel:     ctxCancel,
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

	// Filter setup moved to message processing loop (before RAG)
	// Filters now use the unified RunScript API with rich context
	cs.preProcessors = []func(*models.UserMessage) error{}

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

	// Resolve secret references for embedding and DB connection API keys
	ds.EmbedAPIKey = secrets.GetValue(ds.EmbedAPIKey, false)
	ds.DBConnAPIKey = secrets.GetValue(ds.DBConnAPIKey, false)

	cs.stateMu.Lock()
	cs.datasources[id] = &ds
	// Validate privacy scores
	if err := cs.validatePrivacyScoresLocked(); err != nil {
		// If validation fails, remove the datasource and return the error
		delete(cs.datasources, id)
		cs.stateMu.Unlock()
		return err
	}
	cs.stateMu.Unlock()

	cs.sendStatus(fmt.Sprintf("Datasource '%s' added to room", ds.Name))

	return nil
}

func (cs *ChatSession) RemoveDatasource(id uint) {
	cs.stateMu.Lock()
	ds, ok := cs.datasources[id]
	if ok {
		delete(cs.datasources, id)
	}
	cs.stateMu.Unlock()
	if !ok {
		return
	}

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

	// Validate privacy scores with the new tool included, so a tool whose own
	// score exceeds the LLM's is rejected rather than admitted.
	cs.stateMu.Lock()
	cs.tools[id] = t
	if err := cs.validatePrivacyScoresLocked(); err != nil {
		// If validation fails, remove the tool and return the error
		delete(cs.tools, id)
		cs.stateMu.Unlock()
		return err
	}
	cs.stateMu.Unlock()

	slog.Info("tool added to chat", "tool", t.Name)
	cs.sendStatus(fmt.Sprintf("Tool '%s' added to room", t.Name))

	for i, _ := range t.FileStores {
		// base64 decode the file contents first
		content, err := base64.StdEncoding.DecodeString(t.FileStores[i].Content)
		if err != nil {
			return fmt.Errorf("error decoding file contents: %v", err)
		}

		docText := fmt.Sprintf("The following additional documentation file '%s' has been provided for the tool '%s' to help you use it:\n%s",
			t.FileStores[i].FileName,
			t.Name,
			content)
		pl := fmt.Sprintf("[CONTEXT]\n%s\n[/CONTEXT]", docText)

		err = cs.chatHistory.AddUserMessage(context.Background(), pl)
		if err != nil {
			return fmt.Errorf("error adding message to history: %v", err)
		}
		cs.emit(EventContext, ContextData{Text: docText, Source: ContextSourceToolDocs})

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

	return nil
}

func (cs *ChatSession) AddFileReference(filename, contents string) {
	cs.stateMu.Lock()
	defer cs.stateMu.Unlock()
	cs.files[filename] = contents
}

func (cs *ChatSession) GetFileReference(filename string) (string, bool) {
	cs.stateMu.Lock()
	defer cs.stateMu.Unlock()
	contents, ok := cs.files[filename]
	return contents, ok
}

func (cs *ChatSession) RemoveTool(id string) {
	cs.stateMu.Lock()
	t, ok := cs.tools[id]
	if ok {
		delete(cs.tools, id)
	}
	cs.stateMu.Unlock()
	if !ok {
		return
	}

	cs.sendStatus(fmt.Sprintf("Tool '%s' removed from room", t.Name))
}

// CurrentTools returns a copy of the tools attached to the session.
func (cs *ChatSession) CurrentTools() map[string]models.Tool {
	return cs.snapshotTools()
}

// GetCurrentDatasources returns a slice of current datasources
func (cs *ChatSession) GetCurrentDatasources() []*models.Datasource {
	cs.stateMu.Lock()
	defer cs.stateMu.Unlock()
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

// Stop ends the session: it cancels the session context (aborting any in-flight
// LLM call), stops the processing goroutine and closes the queue. It is
// idempotent and never blocks on the goroutine, so a reaper can call it on a
// busy session. The input channel is deliberately not closed: handlers that
// still hold a reference would panic on send; they get a queue-closed error
// from the session instead.
func (cs *ChatSession) Stop() {
	cs.stopOnce.Do(func() {
		cs.ctxCancel()
		close(cs.stop)
		cs.queue.Close()
	})
}

// Stopped reports whether Stop has been called.
func (cs *ChatSession) Stopped() bool {
	select {
	case <-cs.stop:
		return true
	default:
		return false
	}
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

	if cs.outputMode == OutputModeEvents {
		cs.startEventFanout()
	}

	go func() {
		for {
			select {
			case <-cs.stop:
				return
			case msg, ok := <-cs.input:
				if !ok {
					return
				}
				cs.beginRun(msg.RunID)
				if !cs.processUserMessage(msg) {
					// Nothing was dispatched to the LLM, so no response will
					// arrive on the queue: the turn is over now.
					cs.finishRun(FinishError)
				}

			case resp, ok := <-cs.queue.ConsumeLLMResponses(context.Background()):
				if !ok {
					return
				}
				// handle any response from the LLM
				continued, err := cs.HandleLLMResponse(resp)
				if err != nil {
					cs.sendError(fmt.Errorf("error handling LLM response: %v", err))
					cs.finishRun(FinishError)
					continue
				}
				if !continued {
					if cs.AwaitingClientTools() {
						cs.finishRun(FinishToolCalls)
					} else {
						cs.finishRun(FinishStop)
					}
				}
			}
		}
	}()

	return nil
}

// processUserMessage runs filters, RAG, file scanning and the LLM call for one
// user message. It returns true when an LLM response was dispatched to the
// queue (so the turn continues in HandleLLMResponse) and false when the turn
// ended here.
func (cs *ChatSession) processUserMessage(msg *models.UserMessage) bool {
	if len(msg.ToolResults) > 0 {
		return cs.resumeWithToolResults(msg.ToolResults)
	}
	// A new message (or regenerate) while client calls are parked: close
	// them out so the stored history stays valid for the model.
	cs.abandonPendingClientCalls()

	if msg.Regenerate {
		return cs.regenerateTurn()
	}

	err := cs.preProcessMessage(msg)
	if err != nil {
		cs.sendStatus(fmt.Sprintf("Content guideline violation detected. This request cannot be processed."))
		return false
	}

	// Run chat REQUEST filters on user message before RAG search (exclude response filters)
	filteredMessage := msg.Payload
	filterBlocked := false

	// Filter to only request filters (ResponseFilter = false)
	requestFilters := []*models.Filter{}
	for _, filter := range cs.filters {
		if !filter.ResponseFilter {
			requestFilters = append(requestFilters, filter)
		}
	}

	slog.Info("Chat request filters check", "filter_count", len(requestFilters))

	for _, filter := range requestFilters {
		slog.Info("Running chat request filter", "filter_name", filter.Name)
		sr := scripting.NewScriptRunner(filter.Script)

		// Create MessageContent for user message
		messages := []llms.MessageContent{
			{
				Role:  llms.ChatMessageTypeHuman,
				Parts: []llms.ContentPart{llms.TextPart(filteredMessage)},
			},
		}

		scriptInput := &scripting.ScriptInput{
			RawInput:   filteredMessage,
			Messages:   messages,
			VendorName: string(cs.chatRef.LLM.Vendor),
			ModelName:  cs.chatRef.LLMSettings.ModelName,
			Context: map[string]interface{}{
				"session_id": cs.ID(),
				"user_id":    int64(cs.userID),
				"chat_id":    int64(cs.chatRef.ID),
			},
			IsChat: true,
		}

		output, err := sr.RunScript(scriptInput, cs.service)
		if err != nil {
			cs.sendError(fmt.Errorf("filter error: %v", err))
			filterBlocked = true
			break
		}

		// Record any compliance events reported by the script
		scripting.RecordComplianceEvents(cs.ctx, output, filter.Name, "chat_request", 0, cs.userID, cs.chatRef.LLM.ID, string(cs.chatRef.LLM.Vendor), cs.chatRef.LLMSettings.ModelName)

		if output.Block {
			blockMsg := output.Message
			if blockMsg == "" {
				blockMsg = "content blocked by policy"
			}
			cs.sendError(fmt.Errorf("blocked by filter '%s': %s", filter.Name, blockMsg))
			filterBlocked = true
			break
		}

		// Apply modifications for next filter and RAG search
		if output.Payload != "" && output.Payload != filteredMessage {
			filteredMessage = output.Payload
		}
	}

	// Skip RAG and LLM if filter blocked the message
	if filterBlocked {
		return false
	}

	// Update message payload with filtered content for RAG
	msg.Payload = filteredMessage

	// handle RAG
	n := cs.chatRef.RagResultsPerSource
	if n == 0 {
		n = 10
	}
	ds := dataSession.NewDataSession(cs.snapshotDatasources())
	docs, err := ds.Search(filteredMessage, 10) //TODO this should be configurable in the future
	if err != nil {
		cs.sendError(fmt.Errorf("error searching datasources: %v", err))
		return false
	}

	// prep tools
	tools := cs.prepareTools()

	// secure file references
	scanFailureResponse, ok := cs.scanFiles(msg.FileRef)
	if !ok {
		cs.sendError(fmt.Errorf("%s", scanFailureResponse))
		return false
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
		return false
	}
	return true
}

// regenerateTurn re-runs the model on the stored history as it stands (the
// caller has already truncated the previous reply). It returns true when a
// response was dispatched to the queue.
func (cs *ChatSession) regenerateTurn() bool {
	if cs.caller == nil {
		cs.sendError(fmt.Errorf("LLM driver is not initialized"))
		return false
	}
	messages, err := cs.getMessages()
	if err != nil {
		cs.sendError(fmt.Errorf("error getting chat history: %v", err))
		return false
	}
	if len(messages) == 0 {
		cs.sendError(fmt.Errorf("nothing to regenerate: the session has no messages"))
		return false
	}
	messages = cs.PreflightTokenLengthCheck(messages)

	tools := cs.prepareTools()
	opts := cs.getOptions(cs.chatRef.LLMSettings, tools)

	cs.streamBuffer = ""
	cs.streamChunkIndex = 0
	cs.streamFilterBlocked = false
	cs.resetStreamed()

	ctx, done := context.WithTimeout(cs.runCtx(), 300*time.Second)
	defer done()
	resp, err := cs.caller.GenerateContent(ctx, messages, opts...)
	if err != nil {
		cs.sendError(fmt.Errorf("[regenerate] error generating content: %v", err))
		return false
	}

	publishCtx, publishCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer publishCancel()
	if err := cs.queue.PublishLLMResponse(publishCtx, &LLMResponseWrapper{Response: resp, Opts: opts}); err != nil {
		cs.sendError(fmt.Errorf("could not send response to llm responses queue: %v", err))
		return false
	}
	return true
}

func (cs *ChatSession) sendOutput(resp string) {
	// Disabled to prevent duplicate messages
}

func (cs *ChatSession) sendStatus(resp string) {
	if cs.outputMode == OutputModeEvents {
		cs.emit(EventStatus, StatusData{Text: resp, Level: "info"})
		return
	}

	msg := fmt.Sprintf(":::system %s:::", resp)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Send only to message channel to avoid duplicate messages
	// The SSE handler processes this via OutputMessage() which creates system-type messages
	cs.queue.PublishMessage(ctx, &ChatResponse{Payload: msg})
}

func (cs *ChatSession) sendError(err error) {
	if cs.outputMode == OutputModeEvents {
		code := ClassifyError(err)
		cs.emit(EventError, ErrorData{
			Code:      code,
			Message:   err.Error(),
			Retryable: code == ErrCodeConnection || code == ErrCodeAPI,
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if queueErr := cs.queue.PublishError(ctx, err); queueErr != nil {
		slog.Error("error sending error to queue", "queue_error", queueErr, "original_error", err)
	}
}

func (cs *ChatSession) prepareTools() []llms.Tool {
	tools := make([]llms.Tool, 0)
	ids := make(map[string]struct{})
	for _, t := range cs.snapshotTools() {
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

		case models.ToolTypeClient:
			def, err := clientToolDefinition(t)
			if err != nil {
				cs.sendError(fmt.Errorf("error creating client tool definition: %v", err))
				continue
			}
			tools = append(tools, def)
			ids[t.Name] = struct{}{}

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
			currentContent := content

			for i2, _ := range cs.filters {
				sr := scripting.NewScriptRunner(cs.filters[i2].Script)
				if sr == nil {
					cs.sendError(fmt.Errorf("error creating script runner"))
					continue
				}

				// Create MessageContent for file content (user role)
				messages := []llms.MessageContent{
					{
						Role:  llms.ChatMessageTypeHuman,
						Parts: []llms.ContentPart{llms.TextPart(currentContent)},
					},
				}

				scriptInput := &scripting.ScriptInput{
					RawInput:   currentContent,
					Messages:   messages,
					VendorName: string(cs.chatRef.LLM.Vendor),
					ModelName:  cs.chatRef.LLMSettings.ModelName,
					Context: map[string]interface{}{
						"session_id": cs.ID(),
						"user_id":    int64(cs.userID),     // Convert uint to int64 for Tengo
						"chat_id":    int64(cs.chatRef.ID), // Convert uint to int64 for Tengo
						"file_ref":   refs[i],
					},
					IsChat: true,
				}

				output, err := sr.RunScript(scriptInput, cs.service)
				if err != nil {
					return fmt.Sprintf("filter error in %s: %v", refs[i], err), false
				}

				// Record any compliance events reported by the script
				scripting.RecordComplianceEvents(cs.ctx, output, cs.filters[i2].Name, "file_reference", 0, cs.userID, cs.chatRef.LLM.ID, string(cs.chatRef.LLM.Vendor), cs.chatRef.LLMSettings.ModelName)

				if output.Block {
					msg := output.Message
					if msg == "" {
						msg = "content blocked by policy"
					}
					return fmt.Sprintf("filter denied content in %s: %s", refs[i], msg), false
				}

				// Apply modifications for next filter
				if output.Payload != "" && output.Payload != currentContent {
					currentContent = output.Payload
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
		// Add metadata if available
		metadataFields := []string{}
		if filePath, ok := doc.Metadata["file_path"].(string); ok && filePath != "" {
			metadataFields = append(metadataFields, fmt.Sprintf("File Path: %s", filePath))
		}
		if githubURL, ok := doc.Metadata["github_url"].(string); ok && githubURL != "" {
			metadataFields = append(metadataFields, fmt.Sprintf("GitHub URL: %s", githubURL))
		}
		if lineStart, ok := doc.Metadata["line_start"].(string); ok && lineStart != "" {
			if lineEnd, ok := doc.Metadata["line_end"].(string); ok && lineEnd != "" {
				metadataFields = append(metadataFields, fmt.Sprintf("Lines: %s-%s", lineStart, lineEnd))
			}
		}
		if timestamp, ok := doc.Metadata["ingestion_timestamp"].(string); ok && timestamp != "" {
			metadataFields = append(metadataFields, fmt.Sprintf("Ingested: %s", timestamp))
		}

		// Build document text with optional metadata header
		if len(metadataFields) > 0 {
			metadataHeader := "[METADATA FOR CONTENT:]\n"
			for _, field := range metadataFields {
				metadataHeader += fmt.Sprintf("- %s\n", field)
			}
			text += metadataHeader + "\n" + doc.PageContent
		} else {
			text += doc.PageContent
		}

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

// snapshotTools returns a copy of the tool map for readers on the session
// goroutine, so HTTP handlers can add and remove tools concurrently.
func (cs *ChatSession) snapshotTools() map[string]models.Tool {
	cs.stateMu.Lock()
	defer cs.stateMu.Unlock()
	out := make(map[string]models.Tool, len(cs.tools))
	for k, v := range cs.tools {
		out[k] = v
	}
	return out
}

// snapshotDatasources returns a copy of the datasource map (see snapshotTools).
func (cs *ChatSession) snapshotDatasources() map[uint]*models.Datasource {
	cs.stateMu.Lock()
	defer cs.stateMu.Unlock()
	out := make(map[uint]*models.Datasource, len(cs.datasources))
	for k, v := range cs.datasources {
		out[k] = v
	}
	return out
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

// HandleLLMResponse persists and forwards one LLM reply. When the reply asked
// for tools it executes them and dispatches a follow-up LLM call, in which case
// it returns continued=true: the turn is not over and the next reply will
// arrive on the queue. continued=false means the turn ended with this reply.
func (cs *ChatSession) HandleLLMResponse(w *LLMResponseWrapper) (continued bool, err error) {
	if config.Get("").EchoConversation {
		handleEcho("LLM", w.Response)
	}
	resp := w.Response
	if len(resp.Choices) == 0 {
		cs.sendError(fmt.Errorf("no choices in response"))
		return false, nil
	}

	toolCall := false
	parked := 0
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
			parked += cs.handleToolCalls(reply, &toolCallRequest, &toolCallResult)
			toolCall = true
		}
	}

	ctx := context.Background()

	if content != "" {
		// Execute response filters before adding to history and sending to user
		blocked, blockMsg, filterErr := ExecuteResponseFilters(
			cs.ctx,
			cs.filters,
			cs.service,
			content,
			string(cs.chatRef.LLM.Vendor),
			cs.chatRef.LLMSettings.ModelName,
			false, // isStreaming (non-streaming response)
			false, // isChunk
			0,     // chunkIndex
			"",    // currentBuffer (not used for non-streaming)
			cs.id,
			cs.userID,
			cs.chatRef.ID,
		)
		if filterErr != nil {
			slog.Error("chat response filter execution error", "error", filterErr)
			// Fail open on error - allow response through
		} else if blocked {
			// Response blocked by filter - send error to user instead
			slog.Info("chat response blocked by filter", "message", blockMsg)
			cs.sendError(fmt.Errorf("Response blocked: %s", blockMsg))
			return false, nil
		}

		// For regular messages without tool calls
		err := cs.chatHistory.AddAIMessage(ctx, content)
		if err != nil {
			cs.sendError(fmt.Errorf("error adding AI message to history: %v", err))
			return false, err
		}
		cs.noteAssistantMessageID(cs.chatHistory.LastMessageID())

		if cs.outputMode == OutputModeEvents {
			// A vendor that did not stream (or a reply that arrived whole)
			// still has to reach the client as text.
			if !cs.hasStreamed() {
				cs.emit(EventTextDelta, TextDeltaData{Delta: content})
			}
			cs.emit(EventTextEnd, nil)
		} else if !toolCall {
			// Only send to output stream if this is not a tool call
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
			return false, err
		}

		err = cs.chatHistory.AddMessage(ctx, toolCallRequest)
		if err != nil {
			cs.sendError(fmt.Errorf("error adding tool call to history: %v", err))
			return false, err
		}
		cs.noteAssistantMessageID(cs.chatHistory.LastMessageID())

		if parked > 0 {
			// Some calls belong to the user. Keep the server-side results
			// aside; they are persisted together with the user's answers when
			// the turn resumes. The turn ends here with FinishToolCalls.
			cs.parkTurn(toolCallResult)
			_ = history
			return false, nil
		}

		err = cs.chatHistory.AddMessage(ctx, toolCallResult)
		if err != nil {
			cs.sendError(fmt.Errorf("error adding tool call to history: %v", err))
			return false, err
		}

		if !cs.callModelAfterTools() {
			return false, fmt.Errorf("model call after tool results failed")
		}
		return true, nil
	}

	return false, nil
}

func (cs *ChatSession) HandleUserMessage(msg *models.UserMessage, docs []schema.Document, tools []llms.Tool, files map[string]string) (*llms.ContentResponse, error) {
	opts := cs.getOptions(cs.chatRef.LLMSettings, tools)
	if cs.caller == nil {
		return nil, fmt.Errorf("LLM driver is not initialized")
	}

	// Derived from the run context so a cancel (or session stop) aborts the call.
	ctx, done := context.WithTimeout(cs.runCtx(), 300*time.Second)
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

	if config.Get("").EchoConversation {
		type ComboObj struct {
			Message *models.UserMessage
			Docs    []schema.Document
		}

		handleEcho("USER", ComboObj{Message: msg, Docs: docs})
	}

	if len(docs) > 0 {
		cs.emit(EventContext, ContextData{Text: cs.joinDocuments(docs, "\n\n"), Source: ContextSourceRAG})
	}
	pl := cs.prepHumanMessage(msg.Payload, docs).Content
	err := cs.chatHistory.AddUserMessage(context.Background(), pl)
	if err != nil {
		return nil, fmt.Errorf("error adding message to history: %v", err)
	}
	cs.noteUserMessageID(cs.chatHistory.LastMessageID())

	messages, err := cs.getMessages()
	if err != nil {
		return nil, fmt.Errorf("error getting chat history: %v", err)
	}

	// need to make sure we stay in content window
	messages = cs.PreflightTokenLengthCheck(messages)

	// Reset streaming buffer/index for new request
	cs.streamBuffer = ""
	cs.streamChunkIndex = 0
	cs.streamFilterBlocked = false
	cs.resetStreamed()

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
		cs.chatRef.LLM.DontLogBodies,
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

	unmute := cs.muteStreaming()
	resp, err := cs.caller.GenerateContent(ctx, messages, opts...)
	unmute()
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
	if cs.outputMode == OutputModeRaw {
		cs.sendStatus(errMsg)
	}
	cs.emit(EventToolResult, ToolResultData{ToolCallID: toolCallID, Result: errMsg, IsError: true})

	toolResp := llms.ToolCallResponse{
		ToolCallID: toolCallID,
		Name:       functionName,
		Content:    "ERROR: " + errMsg,
	}

	toolResult.Parts = append(toolResult.Parts, toolResp)
}

// handleToolCalls executes the server-side tools a reply asked for, appending
// the requests to toolCall and the results to toolResult. Client tools are
// parked instead (see client_tools.go); the return value is how many were.
func (cs *ChatSession) handleToolCalls(choice *llms.ContentChoice, toolCall, toolResult *llms.MessageContent) (parked int) {
	currentTools := cs.snapshotTools()
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

		// The model hands us complete arguments, so the three structured
		// events are emitted back to back; the client sees the same shape a
		// token-streamed tool call would produce.
		cs.emit(EventToolCallStart, ToolCallStartData{ToolCallID: t.ID, ToolName: t.FunctionCall.Name})
		cs.emit(EventToolCallDelta, ToolCallDeltaData{ToolCallID: t.ID, ArgsText: t.FunctionCall.Arguments})
		cs.emit(EventToolCallEnd, ToolCallEndData{ToolCallID: t.ID})

		toolDefIndex := ""
		for i, tool := range currentTools {
			if tool.ToolType == models.ToolTypeClient && tool.ClientOperation() == t.FunctionCall.Name {
				toolDefIndex = i
				break
			}
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

		toolDef, ok := currentTools[toolDefIndex]
		if !ok {
			errMsg := fmt.Sprintf("tool not found: %s", t.FunctionCall.Name)
			cs.handleToolError(errMsg, t.ID, t.FunctionCall.Name, toolResult)
			continue
		}

		switch toolDef.ToolType {
		case models.ToolTypeREST:
			cs.executeRESTToolCall(t, toolDef, toolResult)
		case models.ToolTypeClient:
			cs.parkClientCall(t)
			parked++
		default:
			// Unknown types are skipped without a result part (existing behaviour).
			slog.Warn("skipping tool call for unsupported tool type", "tool", toolDef.Name, "type", toolDef.ToolType)
		}
	}
	return parked
}

// executeRESTToolCall runs a single REST tool call end to end: governance
// filters on the arguments, the call itself, then governance filters on the
// response. Every failure path returns, so a blocked call contributes exactly
// one error part to the result and never the payload it blocked.
func (cs *ChatSession) executeRESTToolCall(t llms.ToolCall, toolDef models.Tool, toolResult *llms.MessageContent) {
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
		return
	}

	t0 := time.Now()
	args, err := cs.convertLLMArgsToUniversalClientInputs([]byte(t.FunctionCall.Arguments), t.FunctionCall.Name, uc)
	if err != nil {
		errMsg := fmt.Sprintf("error converting LLM args to universal client inputs: %v", err)
		cs.handleToolError(errMsg, t.ID, t.FunctionCall.Name, toolResult)
		return
	}

	identity := scripting.ToolFilterIdentity{
		ToolID:    toolDef.ID,
		ToolName:  toolDef.Name,
		UserID:    cs.userID,
		SessionID: cs.ID(),
		CallID:    t.ID,
		IsChat:    true,
	}

	// Request-side filters see the arguments before they leave for the tool,
	// so sensitive content can be stopped or redacted on the way out.
	callArgs := scripting.ToolCallArgs{
		OperationID: t.FunctionCall.Name,
		Parameters:  args.Parameters,
		Payload:     args.Body,
		Headers:     args.Headers,
	}

	if cs.outputMode == OutputModeRaw {
		// v2 clients get the structured tool-call events instead.
		cs.sendStatus(fmt.Sprintf("Using function: `%s()`", t.FunctionCall.Name))
		cs.sendStatus(fmt.Sprintf("Parameters: `%s`", t.FunctionCall.Arguments))
	}
	if config.Get("").EchoConversation {
		slog.Info("[TOOL-CALL]", "[FUNCTION]", t.FunctionCall.Name)
		slog.Info("[TOOL-CALL]", "[PARAMS]", t.FunctionCall.Arguments)
	}

	callArgs, block, err := scripting.RunToolInputFilters(cs.ctx, toolDef.Filters, cs.service, callArgs, identity)
	if block != nil || err != nil {
		// The model is told only that policy stopped the call. Naming the
		// filter would map the governance configuration for anyone able to
		// steer the conversation, and a script error carries Tengo's own
		// diagnostics, which can quote the filter source.
		slog.Info("tool call blocked before dispatch",
			"tool", toolDef.Name, "operation", t.FunctionCall.Name,
			"session_id", cs.ID(), "error", err, "block", block)
		cs.handleToolError(scripting.ToolBlockedMessage, t.ID, t.FunctionCall.Name, toolResult)
		return
	}

	resp, err := uc.CallOperation(t.FunctionCall.Name, callArgs.Parameters, callArgs.Payload, callArgs.Headers)
	if err != nil {
		if config.Get("").EchoConversation {
			slog.Info("[TOOL-CALL]", "[ERROR]", err)
		}

		errMsg := fmt.Sprintf("error calling tool operation [%s]: %v", t.FunctionCall.Name, err)
		cs.handleToolError(errMsg, t.ID, t.FunctionCall.Name, toolResult)
		return
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
		return
	}

	t1 := time.Now()

	if config.Get("").EchoConversation {
		fmt.Println("===============================================")
		slog.Info("[TOOL CALL]", "[FUNCTION]", t.FunctionCall.Name)
		fmt.Println(asStr)
		fmt.Println("===============================================")
	}

	// Response-side filters see what the tool returned before the model does.
	if len(toolDef.Filters) > 0 {
		cs.sendStatus("Running governance filters")
	}

	filtered, block, err := scripting.RunToolOutputFilters(cs.ctx, toolDef.Filters, cs.service, asStr, identity)
	if block != nil || err != nil {
		slog.Info("tool response blocked",
			"tool", toolDef.Name, "operation", t.FunctionCall.Name,
			"session_id", cs.ID(), "error", err, "block", block)
		cs.handleToolError(scripting.ToolBlockedMessage, t.ID, t.FunctionCall.Name, toolResult)
		return
	}
	asStr = filtered

	toolResp := llms.ToolCallResponse{
		ToolCallID: t.ID,
		Name:       t.FunctionCall.Name,
		Content:    asStr,
	}

	if cs.outputMode == OutputModeRaw {
		cs.sendStatus(fmt.Sprintf("Function `%s()` returned: `%d` bytes", t.FunctionCall.Name, len(asStr)))
	}
	cs.emit(EventToolResult, ToolResultData{ToolCallID: t.ID, Result: asStr, Bytes: len(asStr)})
	if config.Get("").EchoConversation && len(toolDef.Filters) > 0 {
		slog.Info("[TOOL-CALL]", "[FILTERED]", t.FunctionCall.Name)
		fmt.Println("===============================================")
		fmt.Println(asStr)
		fmt.Println("===============================================")
	}

	toolResult.Parts = append(toolResult.Parts, toolResp)

	analytics.RecordToolCall(
		context.Background(),
		t.FunctionCall.Name,
		time.Now(),
		int(t1.Sub(t0).Milliseconds()), toolDef.ID)
}

func (cs *ChatSession) streamingFunc(ctx context.Context, chunk []byte) error {
	if cs.streamingMuted() {
		return nil
	}
	// Try to parse as JSON to check if it's a final message
	var msg llms.MessageContent
	if err := json.Unmarshal(chunk, &msg); err != nil {
		// Not JSON, this is a streaming chunk
		chunkText := string(chunk)

		// Accumulate in buffer for response filters
		cs.streamBuffer += chunkText
		currentChunkIndex := cs.streamChunkIndex
		cs.streamChunkIndex++

		// Execute response filters on each chunk (if configured)
		// Only run filters if chatRef and LLM are properly initialized
		var blocked bool
		var blockMsg string
		var filterErr error

		if cs.chatRef != nil && cs.chatRef.LLM != nil && cs.chatRef.LLMSettings != nil {
			blocked, blockMsg, filterErr = ExecuteResponseFilters(
				cs.ctx,
				cs.filters,
				cs.service,
				chunkText,
				string(cs.chatRef.LLM.Vendor),
				cs.chatRef.LLMSettings.ModelName,
				true,              // isStreaming
				true,              // isChunk
				currentChunkIndex, // chunkIndex
				cs.streamBuffer,   // currentBuffer (accumulated so far)
				cs.id,
				cs.userID,
				cs.chatRef.ID,
			)
		}

		if filterErr != nil {
			slog.Error("chat streaming filter execution error", "error", filterErr, "chunk_index", currentChunkIndex)
			// Fail open on error - continue streaming
		} else if blocked {
			// Response blocked mid-stream - send error and mark as blocked
			slog.Info("chat streaming response blocked by filter", "message", blockMsg, "chunk_index", currentChunkIndex)
			cs.streamFilterBlocked = true

			// Send error message to user
			errorMsg := fmt.Sprintf("Response blocked: %s", blockMsg)
			if cs.outputMode == OutputModeEvents {
				cs.emit(EventError, ErrorData{Code: ErrCodeFilter, Message: errorMsg})
				return fmt.Errorf("streaming blocked by filter: %s", blockMsg)
			}

			streamCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
			defer cancel()

			if queueErr := cs.queue.PublishStream(streamCtx, []byte(errorMsg)); queueErr != nil {
				return fmt.Errorf("error publishing block message: %v", queueErr)
			}

			// Stop accepting further chunks
			return fmt.Errorf("streaming blocked by filter: %s", blockMsg)
		}

		// Filter passed - send chunk to queue
		if cs.outputMode == OutputModeEvents {
			cs.markStreamed()
			cs.emit(EventTextDelta, TextDeltaData{Delta: chunkText})
			return nil
		}

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

// validatePrivacyScores takes stateMu and runs validatePrivacyScoresLocked.
func (cs *ChatSession) validatePrivacyScores() error {
	cs.stateMu.Lock()
	defer cs.stateMu.Unlock()
	return cs.validatePrivacyScoresLocked()
}

// validatePrivacyScoresLocked checks tool/datasource privacy scores against the
// LLM's. The caller must hold stateMu.
func (cs *ChatSession) validatePrivacyScoresLocked() error {
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

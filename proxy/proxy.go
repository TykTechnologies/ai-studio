package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/config"
	dataSession "github.com/TykTechnologies/midsommar/v2/data_session"
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/TykTechnologies/midsommar/v2/universalclient"
	"github.com/gorilla/mux"
	"github.com/gosimple/slug"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/pb33f/libopenapi"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
)

const (
	LLMPRefix        = "/llm/"
	DatasourcePrefix = "/datasource/"
	ToolPrefix       = "/tools/"
	MCPSuffix        = "/mcp"
)

type EndpointMap struct {
	LLMs        map[string]string
	Datasources map[string]string
}

type ErrorResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

type SearchQuery struct {
	Query string `json:"query"`
	N     int    `json:"n"`
}

type SearchResults struct {
	Documents []schema.Document `json:"documents"`
}

type OpenAPICache struct {
	Document    libopenapi.Document
	Operations  []string
	ToolVersion int64
	CreatedAt   time.Time
}

type MCPServerCache struct {
	SSEServer        *server.SSEServer
	StreamableServer *server.StreamableHTTPServer
	MCPServer        *server.MCPServer
	ToolVersion      int64
	OperationHash    string
}

type Proxy struct {
	gatewayService          services.ServiceInterface
	budgetService           services.BudgetServiceInterface
	server                  *http.Server
	llms                    map[string]*models.LLM
	datasources             map[string]*models.Datasource
	mu                      sync.RWMutex
	config                  *Config
	credValidator              *CredentialValidator
	modelValidator             *ModelValidator
	messageExtractorRegistry   *MessageExtractorRegistry
	messageReconstructorRegistry *MessageReconstructorRegistry
	filters                    []*models.Filter
	authService             *auth.AuthService
	mcpServers              map[string]*MCPServerCache
	mcpServersMu            sync.RWMutex
	openAPICache            map[string]*OpenAPICache
	openAPICacheMu          sync.RWMutex
	responseHookManager     ResponseHookManager // REST-only response hooks
}

type Config struct {
	Port int
}

// New creates a new Proxy instance using the unified services interface.
// This is the new interface-based constructor that supports flexible backends.
func New(gatewayService services.ServiceInterface, budgetService services.BudgetServiceInterface, cfg *Config) *Proxy {
	p := &Proxy{
		gatewayService:      gatewayService,
		budgetService:       budgetService,
		llms:                make(map[string]*models.LLM),
		datasources:         make(map[string]*models.Datasource),
		config:              cfg,
		filters:             make([]*models.Filter, 0),
		mcpServers:          make(map[string]*MCPServerCache),
		openAPICache:        make(map[string]*OpenAPICache),
		responseHookManager: NewDefaultResponseHookManager(), // Initialize REST-only response hooks
	}

	val := NewCredentialValidator(gatewayService, p)
	val.RegisterValidator(strings.ToLower(string(models.OPENAI)), OpenAIValidator)
	val.RegisterValidator(strings.ToLower(string(models.ANTHROPIC)), AnthropicValidator)
	val.RegisterValidator(strings.ToLower(string(models.GOOGLEAI)), GoogleAIValidator)
	val.RegisterValidator(strings.ToLower(string(models.VERTEX)), VertexValidator)
	val.RegisterValidator(strings.ToLower(string(models.HUGGINGFACE)), HuggingFaceValidator)
	val.RegisterValidator(strings.ToLower(string(models.OLLAMA)), OpenAIValidator)
	val.RegisterValidator(strings.ToLower(string(models.MOCK_VENDOR)), MockValidator)
	val.RegisterValidator("dummy", DummyValidator)
	p.credValidator = val

	modelVal := NewModelValidator(nil)
	modelVal.RegisterExtractor(strings.ToLower(string(models.OPENAI)), OpenAIModelExtractor)
	modelVal.RegisterExtractor(strings.ToLower(string(models.ANTHROPIC)), AnthropicModelExtractor)
	modelVal.RegisterExtractor(strings.ToLower(string(models.GOOGLEAI)), GoogleAIModelExtractor)
	modelVal.RegisterExtractor(strings.ToLower(string(models.VERTEX)), VertexModelExtractor)
	modelVal.RegisterExtractor(strings.ToLower(string(models.HUGGINGFACE)), HuggingFaceModelExtractor)
	modelVal.RegisterExtractor(strings.ToLower(string(models.OLLAMA)), OpenAIModelExtractor)
	modelVal.RegisterExtractor(strings.ToLower(string(models.MOCK_VENDOR)), OpenAIModelExtractor)
	p.modelValidator = modelVal

	// Initialize message extractor registry for filter scripts
	messageExtractorRegistry := NewMessageExtractorRegistry()
	messageExtractorRegistry.Register(&OpenAIMessageExtractor{})
	messageExtractorRegistry.Register(&AnthropicMessageExtractor{})
	messageExtractorRegistry.Register(&GoogleAIMessageExtractor{})
	messageExtractorRegistry.Register(&VertexMessageExtractor{})
	messageExtractorRegistry.Register(&OllamaMessageExtractor{})
	p.messageExtractorRegistry = messageExtractorRegistry

	// Initialize message reconstructor registry for message modification
	messageReconstructorRegistry := NewMessageReconstructorRegistry()
	messageReconstructorRegistry.Register(&OpenAIMessageReconstructor{})
	messageReconstructorRegistry.Register(&AnthropicMessageReconstructor{})
	messageReconstructorRegistry.Register(&GoogleAIMessageReconstructor{})
	messageReconstructorRegistry.Register(&VertexMessageReconstructor{})
	messageReconstructorRegistry.Register(&OllamaMessageReconstructor{})
	p.messageReconstructorRegistry = messageReconstructorRegistry

	return p
}

// NewProxy creates a new Proxy instance using the existing concrete services.
// This is the legacy constructor that maintains backward compatibility.
func NewProxy(service *services.Service, cfg *Config, budgetService services.BudgetServiceInterface) *Proxy {
	// Use the new unified interface constructor
	return New(service, budgetService, cfg)
}

func (p *Proxy) Start() error {
	if err := p.loadResources(); err != nil {
		return fmt.Errorf("failed to load resources: %w", err)
	}
	handler := fixDoubleSlash(p.createHandler())

	debugHTTPProxy := os.Getenv("DEBUG_HTTP_PROXY") == "true"
	if debugHTTPProxy {
		originalHandler := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logPath := false
			if strings.HasPrefix(r.URL.Path, "/llm/") || strings.HasPrefix(r.URL.Path, "/tools/") || strings.HasPrefix(r.URL.Path, "/datasource/") || strings.HasPrefix(r.URL.Path, "/.well-known/") || strings.HasPrefix(r.URL.Path, "/ai/") {
				logPath = true
			}
			if logPath {
				fmt.Printf("\n[DEBUG PROXY] Incoming Request to AI Proxy Server (:%d)\n", p.config.Port)
				fmt.Printf("[DEBUG PROXY] Method: %v | Path: %v\n", r.Method, r.URL.Path)
				if r.Body != nil {
					bodyBytes, _ := readBodyWithoutConsuming(r) // Assuming readBodyWithoutConsuming is defined
					if bodyBytes != nil {
						fmt.Printf("[DEBUG PROXY] Request Body: %s\n", string(bodyBytes))
					}
				}
			}
			originalHandler.ServeHTTP(w, r)
		})
	}

	p.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", p.config.Port),
		Handler:      handler,
		ReadTimeout:  300 * time.Second,
		WriteTimeout: 600 * time.Second,
		IdleTimeout:  300 * time.Second,
	}
	log.Printf("Starting proxy server on port %d", p.config.Port)
	return p.server.ListenAndServe()
}

type loggingResponseWriter struct { // Kept for debug middleware, if used.
	http.ResponseWriter
	statusCode int
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (p *Proxy) Stop(ctx context.Context) error { return p.server.Shutdown(ctx) }

func (p *Proxy) Reload() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Println("proxy reloading resources...")
	return p.loadResources()
}

// Handler returns the HTTP handler for the proxy, allowing it to be used
// with existing HTTP servers instead of starting its own server
func (p *Proxy) Handler() http.Handler {
	return fixDoubleSlash(p.createHandler())
}

func (p *Proxy) loadResources() error {
	llms, err := p.gatewayService.GetActiveLLMs()
	if err != nil {
		return fmt.Errorf("failed to get LLMs: %w", err)
	}
	datasources, err := p.gatewayService.GetActiveDatasources()
	if err != nil {
		return fmt.Errorf("failed to get datasources: %w", err)
	}
	newLLMs := make(map[string]*models.LLM)
	for i := range llms {
		llm := llms[i]
		newLLMs[slug.Make(llm.Name)] = &llm
	}
	newDatasources := make(map[string]*models.Datasource)
	for i := range datasources {
		ds := datasources[i]
		newDatasources[slug.Make(ds.Name)] = &ds
	}
	p.llms = newLLMs
	p.datasources = newDatasources
	return nil
}

func fixDoubleSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := r.URL.Path
		for strings.Contains(cleanPath, "//") {
			cleanPath = strings.ReplaceAll(cleanPath, "//", "/")
		}
		r.URL.Path = cleanPath
		next.ServeHTTP(w, r)
	})
}


func (p *Proxy) createHandler() http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/.well-known/oauth-protected-resource", p.handleOAuthProtectedResourceMetadata).Methods("GET", "OPTIONS")
	r.HandleFunc("/llm/rest/{llmSlug}/{rest:.*}", p.handleLLMRequest).Methods("POST").Handler(p.modelValidationMiddleware(http.HandlerFunc(p.handleLLMRequest)))
	r.HandleFunc("/llm/stream/{llmSlug}/{rest:.*}", p.handleStreamingLLMRequest).Methods("POST").Handler(p.modelValidationMiddleware(http.HandlerFunc(p.handleStreamingLLMRequest)))
	// Unified endpoint that automatically detects streaming vs non-streaming based on request
	r.HandleFunc("/llm/call/{llmSlug}/{rest:.*}", p.handleUnifiedLLMRequest).Methods("POST").Handler(
		p.streamDetectionMiddleware(
			p.modelValidationMiddleware(
				http.HandlerFunc(p.handleUnifiedLLMRequest))))

	// OpenAI-compatible translation endpoints
	r.HandleFunc("/ai/{routeId}/v1/chat/completions", p.CreateChatCompletionHandler).Methods("POST")
	r.HandleFunc("/ai/{routeId}/v1/completions", p.CreateCompletionHandler).Methods("POST")

	r.HandleFunc("/datasource/{dsSlug}", p.handleDatasourceRequest).Methods("POST")
	r.HandleFunc("/tools/{toolSlug}", p.handleToolRequest).Methods("GET", "POST", "PUT", "DELETE")
	r.HandleFunc("/tools/{toolSlug}/mcp", p.handleMCPToolStreamable).Methods("POST")
	r.HandleFunc("/tools/{toolSlug}/mcp/sse", p.handleMCPToolSSE).Methods("GET")
	r.HandleFunc("/tools/{toolSlug}/mcp/message", p.handleMCPToolMessage).Methods("POST")
	// Middleware chain (innermost to outermost):
	// 1. requestIDMiddleware - Generate canonical request ID (MUST BE FIRST)
	// 2. credValidator.Middleware - Authenticate requests
	// 3. outboundRequestMiddleware - Prepare outbound requests
	// 4. cloudflareHeadersMiddleware - Add Cloudflare headers
	return p.cloudflareHeadersMiddleware(p.outboundRequestMiddleware(p.credValidator.Middleware(r)))
}

func (p *Proxy) handleOAuthProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers to allow * origins
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
	w.Header().Set("Access-Control-Max-Age", "43200") // 12 hours

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	appConf := config.Get("")
	if appConf == nil {
		respondWithError(w, http.StatusInternalServerError, "Server configuration not loaded", nil, false)
		return
	}
	authServerMetadataURL := appConf.AuthServerURL
	if !strings.HasSuffix(authServerMetadataURL, "/") {
		authServerMetadataURL += "/"
	}
	authServerMetadataURL += ".well-known/oauth-authorization-server"

	proxyScheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		proxyScheme = "https"
	}
	var resourceBaseURI string
	if appConf.ProxyOAuthMetadataURL != "" {
		if parsedMetaURL, err := url.Parse(appConf.ProxyOAuthMetadataURL); err == nil && parsedMetaURL.Scheme != "" && parsedMetaURL.Host != "" {
			resourceBaseURI = fmt.Sprintf("%s://%s", parsedMetaURL.Scheme, parsedMetaURL.Host)
		}
	}
	if resourceBaseURI == "" && appConf.ProxyURL != "" {
		if parsedProxyURL, err := url.Parse(appConf.ProxyURL); err == nil && parsedProxyURL.Scheme != "" && parsedProxyURL.Host != "" {
			resourceBaseURI = strings.TrimSuffix(appConf.ProxyURL, "/")
		}
	}
	if resourceBaseURI == "" {
		resourceBaseURI = fmt.Sprintf("%s://%s", proxyScheme, r.Host)
	}

	metadata := map[string]interface{}{
		"resource": resourceBaseURI, "authorization_servers": []string{authServerMetadataURL},
		"scopes_supported": []string{"mcp", "mcp_read", "mcp_write"}, "bearer_methods_supported": []string{"auth_header"},
		"mcp_protocol_version": "1.0",
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(metadata)
}

func (p *Proxy) AddFilter(filter *models.Filter) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.filters = append(p.filters, filter)
}

func (p *Proxy) outboundRequestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to read request body", err, false)
			return
		}
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		for _, filter := range p.filters {
			runner := scripting.NewScriptRunner(filter.Script)
			// Note: For now, we'll pass nil since the scripting doesn't depend on the service interface methods
			// This should be refactored if the scripting needs access to data
			if err := runner.RunFilter(string(bodyBytes), nil); err != nil {
				respondWithError(w, http.StatusForbidden, fmt.Sprintf("Policy error: %s", filter.Name), nil, false)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (p *Proxy) cloudflareHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Keep-Alive", "timeout=300")
		w.Header().Set("X-Accel-Buffering", "no")
		next.ServeHTTP(w, r)
	})
}

func respondWithError(w http.ResponseWriter, status int, message string, err error, wwwAuthenticate bool) {
	slog.Error("api client error", "message", message, "status", status, "error", err)
	response := ErrorResponse{Status: status, Message: message}
	if err != nil {
		response.Error = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	if status == http.StatusUnauthorized && wwwAuthenticate {
		appConf := config.Get("")
		metadataURL := appConf.ProxyOAuthMetadataURL
		if metadataURL != "" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer realm=\"MCPResources\", resource_metadata_uri=\"%s\"", metadataURL))
		} else {
			w.Header().Set("WWW-Authenticate", `Bearer realm="MCPResources"`)
		}
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response)
}

func respondWithOAIError(w http.ResponseWriter, status int, message string, err error, wwwAuthenticate bool) {
	httpStatusText := http.StatusText(status)
	// Assuming APIError and OAIErrorResponse are defined in oai_error.go
	apiError := &APIError{Code: status, Message: message, HTTPStatus: httpStatusText, HTTPStatusCode: status}
	if err != nil {
		apiError.Message = fmt.Sprintf("[ERROR] msg: %s err: %s", message, err.Error())
	}
	response := OAIErrorResponse{Error: apiError}
	w.Header().Set("Content-Type", "application/json")
	if status == http.StatusUnauthorized && wwwAuthenticate {
		appConf := config.Get("")
		metadataURL := appConf.ProxyOAuthMetadataURL
		if metadataURL != "" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer realm=\"MCPResources\", resource_metadata_uri=\"%s\"", metadataURL))
		} else {
			w.Header().Set("WWW-Authenticate", `Bearer realm="MCPResources"`)
		}
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response)
}

func (p *Proxy) handleLLMRequest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	llmSlug := vars["llmSlug"]
	p.mu.RLock()
	llm, ok := p.llms[llmSlug]
	p.mu.RUnlock()
	if !ok {
		respondWithError(w, http.StatusNotFound, fmt.Sprintf("[rest] LLM not found: %s", llmSlug), nil, false)
		return
	}
	reqBody, err := helpers.CopyRequestBody(r)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to read request body", err, false)
		return
	}
	appObj := r.Context().Value("app")
	if appObj == nil {
		respondWithError(w, http.StatusUnauthorized, "App context not found, authentication likely failed.", nil, true)
		return
	}
	app, ok := appObj.(*models.App)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "app context invalid", nil, false)
		return
	}
	if _, _, err := p.budgetService.CheckBudget(app, llm); err != nil {
		// Error body for analytics should be constructed carefully if needed
		go p.analyzeResponse(llm, app, http.StatusForbidden, []byte(fmt.Sprintf(`{"error":"budget exceeded: %s"}`, err.Error())), reqBody, r)
		respondWithError(w, http.StatusForbidden, "Budget limit exceeded", err, false)
		return
	}
	if err := p.screenProxyRequestByVendor(llm, r, false); err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error(), err, false)
		return
	}
	upstreamURL, err := url.Parse(llm.APIEndpoint)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "invalid upstream URL", err, false)
		return
	}

	proxyDirector := func(req *http.Request) {
		req.URL.Scheme = upstreamURL.Scheme
		req.URL.Host = upstreamURL.Host
		req.URL.Path = strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/llm/rest/%s", llmSlug))
		req.Host = upstreamURL.Host
		if err := p.setVendorAuthHeader(req, llm); err != nil {
			log.Printf("ERROR setting vendor auth header in director: %v", err)
			// Cannot write http error from director. This needs robust handling or pre-flight check.
		}
	}
	httpProxy := &httputil.ReverseProxy{Director: proxyDirector} // Renamed variable

	// Use buffered response capture only if hooks are actually configured
	if p.responseHookManager != nil && p.hasResponseHooks() {
		bufferedCapture := newBufferedResponseCapture(w)
		httpProxy.ServeHTTP(bufferedCapture, r)

		// Execute REST-only response hooks (hooks modify the buffered response in-place)
		if err := p.executeBufferedResponseHooks(bufferedCapture, llm, app, r); err != nil {
			log.Printf("Response hook execution failed: %v", err)
		}

		// AI Gateway proxy writes the final (potentially modified) response to client
		bufferedCapture.WriteToClient()

		go p.analyzeResponse(llm, app, bufferedCapture.statusCode, bufferedCapture.buffer.Bytes(), reqBody, r)
	} else {
		capture := newResponseCapture(w)
		httpProxy.ServeHTTP(capture, r)
		go p.analyzeResponse(llm, app, capture.statusCode, capture.buffer.Bytes(), reqBody, r)
	}
}

// executeBufferedResponseHooks executes response hooks on the buffered response (REST-only)
func (p *Proxy) executeBufferedResponseHooks(capture *bufferedResponseCapture, llm *models.LLM, app *models.App, r *http.Request) error {
	if p.responseHookManager == nil {
		return nil // No hooks configured
	}

	// Get canonical request ID from context (set by requestIDMiddleware at proxy entry)
	// This MUST exist - if it doesn't, the middleware chain is broken
	requestID := ""
	if reqID := r.Context().Value("request_id"); reqID != nil {
		requestID = reqID.(string)
	}
	if requestID == "" {
		// CRITICAL: Request ID middleware didn't run - fail loudly
		return fmt.Errorf("request ID not found in context - requestIDMiddleware not configured")
	}

	// Create plugin context
	pluginCtx := &PluginContext{
		RequestID: requestID, // Use canonical request ID from context
		LLMSlug:   llm.Name,
		LLMID:     llm.ID,
		AppID:     app.ID,
		UserID:    app.UserID,
		Metadata:  make(map[string]string),
	}

	ctx := context.Background()

	// Execute OnBeforeWriteHeaders hook
	headerReq := &HeadersRequest{
		Headers: make(map[string]string),
		Context: pluginCtx,
	}

	// Convert captured headers to map
	for key, values := range capture.header {
		if len(values) > 0 {
			headerReq.Headers[key] = values[0]
		}
	}

	headerResp, err := p.responseHookManager.ExecuteOnBeforeWriteHeaders(ctx, headerReq)
	if err != nil {
		return fmt.Errorf("header hook failed: %w", err)
	}

	// Apply header modifications if any
	if headerResp.Modified {
		capture.ModifyHeaders(headerResp.Headers)
	}

	// Execute OnBeforeWrite hook with current headers
	currentHeaders := headerResp.Headers
	if !headerResp.Modified {
		currentHeaders = headerReq.Headers // Use original if not modified
	}

	writeReq := &ResponseWriteRequest{
		Body:    capture.CapturedBody(),
		Headers: currentHeaders,
		Context: pluginCtx,
	}

	writeResp, err := p.responseHookManager.ExecuteOnBeforeWrite(ctx, writeReq)
	if err != nil {
		return fmt.Errorf("body hook failed: %w", err)
	}

	// Apply body modifications if any
	if writeResp.Modified {
		capture.ModifyBody(writeResp.Body)
		// Also apply any header changes from the write hook
		capture.ModifyHeaders(writeResp.Headers)
	}

	return nil
}

// hasResponseHooks checks if there are any response hooks configured
func (p *Proxy) hasResponseHooks() bool {
	if manager, ok := p.responseHookManager.(*DefaultResponseHookManager); ok {
		return manager.HasHooks()
	}
	return false
}

// AddResponseHook adds a response hook to the proxy (for embeddable AI Gateway)
func (p *Proxy) AddResponseHook(hook ResponseHook) {
	if p.responseHookManager == nil {
		p.responseHookManager = NewDefaultResponseHookManager()
	}

	if manager, ok := p.responseHookManager.(*DefaultResponseHookManager); ok {
		manager.AddHook(hook)
		log.Printf("Added response hook: %s", hook.GetName())
	}
}

// SetAuthHooks sets authentication lifecycle hooks
func (p *Proxy) SetAuthHooks(hooks *AuthHooks) {
	if p.credValidator != nil {
		p.credValidator.SetAuthHooks(hooks)
	}
}

// SetPostAuthCallback is deprecated, use SetAuthHooks instead
// Kept for backward compatibility
func (p *Proxy) SetPostAuthCallback(callback PostAuthCallback) {
	if p.credValidator != nil {
		p.credValidator.SetPostAuthCallback(callback)
	}
}

// GetResponseHookManager returns the response hook manager for external configuration
func (p *Proxy) GetResponseHookManager() ResponseHookManager {
	return p.responseHookManager
}

func (p *Proxy) handleToolRequest(w http.ResponseWriter, r *http.Request) {
	toolCtx := r.Context().Value("tool")
	if toolCtx == nil {
		respondWithError(w, http.StatusUnauthorized, "Tool context not found, authentication likely failed.", nil, true)
		return
	}
	tool, ok := toolCtx.(*models.Tool)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "invalid tool type in context", nil, false)
		return
	}
	var input struct {
		OperationID string                 `json:"operation_id"`
		Parameters  map[string][]string    `json:"parameters"`
		Payload     map[string]interface{} `json:"payload"`
		Headers     map[string][]string    `json:"headers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid request body", err, false)
		return
	}
	t0 := time.Now()
	result, err := p.gatewayService.CallToolOperation(tool.ID, input.OperationID, input.Parameters, input.Payload, input.Headers)
	t1 := time.Now()
	if err != nil {
		analytics.RecordToolCall(input.OperationID, time.Now(), int(t1.Sub(t0).Milliseconds()), tool.ID)
		respondWithError(w, http.StatusInternalServerError, "failed to call tool operation", err, false)
		return
	}
	analytics.RecordToolCall(input.OperationID, time.Now(), int(t1.Sub(t0).Milliseconds()), tool.ID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if str, ok := result.(string); ok && (strings.HasPrefix(str, "{") || strings.HasPrefix(str, "[")) {
		w.Write([]byte(str))
	} else {
		json.NewEncoder(w).Encode(result)
	}
}

func (p *Proxy) handleDatasourceRequest(w http.ResponseWriter, r *http.Request) {
	dsSlug := mux.Vars(r)["dsSlug"]
	ds, ok := p.GetDatasource(dsSlug)
	if !ok {
		respondWithError(w, http.StatusNotFound, fmt.Sprintf("datasource not found: %s", dsSlug), nil, false)
		return
	}
	session := dataSession.NewDataSession(map[uint]*models.Datasource{ds.ID: ds})
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to read request body", err, false)
		return
	}
	var query SearchQuery
	if err := json.Unmarshal(body, &query); err != nil {
		respondWithError(w, http.StatusBadRequest, "failed to unmarshal request body", err, false)
		return
	}
	results, err := session.Search(query.Query, query.N)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to search", err, false)
		return
	}
	resJSON, err := json.Marshal(SearchResults{Documents: results})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to marshal response", err, false)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(resJSON)
}

func (p *Proxy) handleStreamingLLMRequest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	llmSlug := vars["llmSlug"]
	p.mu.RLock()
	llm, ok := p.llms[llmSlug]
	p.mu.RUnlock()
	if !ok {
		respondWithError(w, http.StatusNotFound, "[streaming] LLM not found", nil, false)
		return
	}
	reqBody, err := helpers.CopyRequestBody(r)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to read streaming request body", err, false)
		return
	}
	appObj := r.Context().Value("app")
	if appObj == nil {
		respondWithError(w, http.StatusUnauthorized, "App context not found, authentication likely failed.", nil, true)
		return
	}
	app, ok := appObj.(*models.App)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "app context invalid for streaming", nil, false)
		return
	}
	if _, _, err := p.budgetService.CheckBudget(app, llm); err != nil {
		go p.analyzeStreamingResponse(llm, app, r, http.StatusForbidden, []byte(fmt.Sprintf(`{"error":"budget exceeded: %s"}`, err.Error())), reqBody, nil, time.Now())
		respondWithError(w, http.StatusForbidden, "Budget limit exceeded for streaming", err, false)
		return
	}
	if err := p.screenProxyRequestByVendor(llm, r, true); err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error(), err, false)
		return
	}
	upstreamURL, err := url.Parse(llm.APIEndpoint)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "invalid upstream URL for streaming", err, false)
		return
	}

	// Strip the gateway prefix and use the remaining path directly (consistent with REST handler)
	upstreamPath := strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/llm/stream/%s", llmSlug))
	upstreamURL.Path = upstreamPath
	upstreamURL.RawQuery = r.URL.RawQuery

	// Use r.Body directly as CopyRequestBody has already replaced it with a readable one.
	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL.String(), r.Body)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to create upstream request for streaming", err, false)
		return
	}
	upstreamReq.Header = r.Header.Clone()
	upstreamReq.Host = upstreamURL.Host
	if err := p.setVendorAuthHeader(upstreamReq, llm); err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to set vendor auth header for streaming", err, false)
		return
	}

	client := &http.Client{
		Timeout: 240 * time.Second,
	}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to make upstream request for streaming", err, false)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	var fullResponse bytes.Buffer
	buffer := make([]byte, 1024)
	var responses [][]byte
	isErr := false
	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buffer[:n])
			responses = append(responses, chunk)
			fullResponse.Write(chunk)
			if _, werr := w.Write(chunk); werr != nil {
				isErr = true
				break
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			isErr = true
			break
		}
	}
	if !isErr {
		go p.analyzeStreamingResponse(llm, app, upstreamReq, resp.StatusCode, fullResponse.Bytes(), reqBody, responses, time.Now())
	}
}

// handleUnifiedLLMRequest is a unified handler that automatically routes to either
// handleLLMRequest (REST) or handleStreamingLLMRequest (streaming) based on the
// streaming intent detected by streamDetectionMiddleware
func (p *Proxy) handleUnifiedLLMRequest(w http.ResponseWriter, r *http.Request) {
	// Get streaming decision from context (set by streamDetectionMiddleware)
	isStreaming, ok := r.Context().Value("is_streaming_request").(bool)
	if !ok {
		// This should never happen if middleware is properly configured
		slog.Error("Streaming detection context missing in unified handler")
		respondWithError(w, http.StatusInternalServerError, "Internal routing error", nil, false)
		return
	}

	// Rewrite the URL path from /llm/call/{slug}/... to /llm/rest/{slug}/... or /llm/stream/{slug}/...
	// This allows the existing handlers to correctly strip the prefix when forwarding to vendors
	vars := mux.Vars(r)
	llmSlug := vars["llmSlug"]

	// Clone the request to avoid modifying the original
	r = r.Clone(r.Context())

	originalPath := r.URL.Path
	if isStreaming {
		// Rewrite path from /llm/call/{slug}/... to /llm/stream/{slug}/...
		r.URL.Path = strings.Replace(r.URL.Path, fmt.Sprintf("/llm/call/%s", llmSlug), fmt.Sprintf("/llm/stream/%s", llmSlug), 1)
		slog.Debug("Unified handler routing to streaming", "original_path", originalPath, "rewritten_path", r.URL.Path, "llm_slug", llmSlug)
		p.handleStreamingLLMRequest(w, r)
	} else {
		// Rewrite path from /llm/call/{slug}/... to /llm/rest/{slug}/...
		r.URL.Path = strings.Replace(r.URL.Path, fmt.Sprintf("/llm/call/%s", llmSlug), fmt.Sprintf("/llm/rest/%s", llmSlug), 1)
		slog.Debug("Unified handler routing to REST", "original_path", originalPath, "rewritten_path", r.URL.Path, "llm_slug", llmSlug)
		p.handleLLMRequest(w, r)
	}
}

func (p *Proxy) analyzeResponse(llm *models.LLM, app *models.App, statusCode int, body []byte, reqBody []byte, r *http.Request) {
	AnalyzeResponse(p.gatewayService, llm, app, statusCode, body, reqBody, r)
}
func (p *Proxy) setVendorAuthHeader(r *http.Request, llm *models.LLM) error {
	return switches.SetVendorAuthHeader(r, llm)
}
func (p *Proxy) GetDatasource(name string) (*models.Datasource, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	ds, ok := p.datasources[name]
	return ds, ok
}
func (p *Proxy) GetLLM(name string) (*models.LLM, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	llm, ok := p.llms[name]
	return llm, ok
}
func (p *Proxy) screenProxyRequestByVendor(llm *models.LLM, r *http.Request, isStreamingChannel bool) error {
	bodyBytes, err := helpers.CopyRequestBody(r)
	if err != nil {
		return err
	}

	// Extract model from context (set by modelValidationMiddleware)
	modelName, _ := r.Context().Value("model_name").(string)

	// Extract messages using the message extractor registry
	messages, err := p.messageExtractorRegistry.Extract(string(llm.Vendor), r, bodyBytes)
	if err != nil {
		slog.Warn("Failed to extract messages for filter", "vendor", llm.Vendor, "error", err)
		messages = []llms.MessageContent{} // Continue with empty messages
	}

	// Get app from context if available
	var appID uint
	if app, ok := r.Context().Value("app").(*models.App); ok {
		appID = app.ID
	}

	// Build script input with rich context
	scriptInput := &scripting.ScriptInput{
		RawInput:   string(bodyBytes),
		Messages:   messages,
		VendorName: string(llm.Vendor),
		ModelName:  modelName,
		Context: map[string]interface{}{
			"llm_id":     int64(llm.ID),    // Convert uint to int64 for Tengo
			"app_id":     int64(appID),      // Convert uint to int64 for Tengo
			"request_id": r.Header.Get("X-Request-ID"),
		},
		IsChat: false,
	}

	// Run filters in chain
	for _, filter := range llm.Filters {
		runner := scripting.NewScriptRunner(filter.Script)
		output, err := runner.RunScript(scriptInput, nil)
		if err != nil {
			return fmt.Errorf("script error in filter '%s': %v", filter.Name, err)
		}

		// Check if request should be blocked
		if output.Block {
			msg := output.Message
			if msg == "" {
				msg = "blocked by policy"
			}
			return fmt.Errorf("Policy error: %s - %s", filter.Name, msg)
		}

		// Apply modifications to the request body for next filter
		// Prefer Messages array if provided, otherwise use Payload
		if len(output.Messages) > 0 {
			// Use message reconstructor to rebuild vendor-specific JSON
			reconstructed, err := p.messageReconstructorRegistry.Reconstruct(string(llm.Vendor), output.Messages, bodyBytes)
			if err != nil {
				return fmt.Errorf("failed to reconstruct request in filter '%s': %v", filter.Name, err)
			}
			scriptInput.RawInput = string(reconstructed)
			bodyBytes = reconstructed

			// Re-extract messages from reconstructed payload for next filter
			if newMessages, err := p.messageExtractorRegistry.Extract(string(llm.Vendor), r, bodyBytes); err == nil {
				scriptInput.Messages = newMessages
			}
		} else if output.Payload != "" && output.Payload != scriptInput.RawInput {
			scriptInput.RawInput = output.Payload
			bodyBytes = []byte(output.Payload)

			// Re-extract messages from modified payload for next filter
			if newMessages, err := p.messageExtractorRegistry.Extract(string(llm.Vendor), r, bodyBytes); err == nil {
				scriptInput.Messages = newMessages
			}
		}
	}

	// Update request body with final modified payload
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	r.ContentLength = int64(len(bodyBytes))

	v, ok := switches.VendorMap[llm.Vendor]
	if !ok {
		return fmt.Errorf("vendor not found")
	}
	return v().ProxyScreenRequest(llm, r, isStreamingChannel)
}
func (p *Proxy) analyzeStreamingResponse(llm *models.LLM, app *models.App, req *http.Request, code int, fullResponse []byte, reqBody []byte, chunks [][]byte, timestamp time.Time) {
	AnalyzeStreamingResponse(p.gatewayService, llm, app, code, fullResponse, reqBody, req, chunks, timestamp)
}
func readBodyWithoutConsuming(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewBuffer(body))
	return body, nil
}
func (p *Proxy) getParsedOpenAPISpec(toolModel *models.Tool) (*universalclient.Client, []string, error) {
	if toolModel.OASSpec == "" {
		return nil, nil, fmt.Errorf("tool has no OpenAPI specification")
	}

	cacheKey := fmt.Sprintf("tool_%d", toolModel.ID)

	// Check cache first
	p.openAPICacheMu.RLock()
	cache, exists := p.openAPICache[cacheKey]
	p.openAPICacheMu.RUnlock()

	// Check if cache is valid (tool version matches and cache is recent)
	cacheExpiry := 30 * time.Minute
	if exists && cache.ToolVersion == toolModel.UpdatedAt.UnixNano() &&
		time.Since(cache.CreatedAt) < cacheExpiry {
		// Create client from cached document
		decodedSpec, err := base64.StdEncoding.DecodeString(toolModel.OASSpec)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode Base64 OpenAPI spec: %w", err)
		}

		client, err := universalclient.NewClient(decodedSpec, "")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create universalclient: %w", err)
		}

		return client, cache.Operations, nil
	}

	// Cache miss or expired - parse the spec
	decodedSpec, err := base64.StdEncoding.DecodeString(toolModel.OASSpec)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode Base64 OpenAPI spec: %w", err)
	}

	client, err := universalclient.NewClient(decodedSpec, "")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create universalclient: %w", err)
	}

	operations, err := client.ListOperations()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list operations: %w", err)
	}

	// Update cache
	p.openAPICacheMu.Lock()
	p.openAPICache[cacheKey] = &OpenAPICache{
		Operations:  operations,
		ToolVersion: toolModel.UpdatedAt.UnixNano(),
		CreatedAt:   time.Now(),
	}
	p.openAPICacheMu.Unlock()

	return client, operations, nil
}

func (p *Proxy) generateOperationHash(toolModel *models.Tool) string {
	// Use a simple hash of the tool version and allowed operations
	// This is much faster than parsing the entire spec
	allowedOps := toolModel.GetOperations()
	if len(allowedOps) == 0 {
		return ""
	}

	// Create hash from tool version + allowed operations
	hashData := fmt.Sprintf("%d:%s", toolModel.UpdatedAt.UnixNano(), strings.Join(allowedOps, ","))
	h := sha256.New()
	h.Write([]byte(hashData))
	return fmt.Sprintf("%x", h.Sum(nil))
}
func parseNumber(val string) (interface{}, error) {
	// Try to parse as integer first
	if intVal, err := strconv.Atoi(val); err == nil {
		return intVal, nil
	}
	// Try to parse as float
	if floatVal, err := strconv.ParseFloat(val, 64); err == nil {
		return floatVal, nil
	}
	return nil, fmt.Errorf("not a valid number: %s", val)
}

func convertMCPParameterValue(val string, paramType string) interface{} {
	switch paramType {
	case "number", "integer":
		if num, err := parseNumber(val); err == nil {
			return num
		}
		return val
	case "boolean":
		if boolVal, err := strconv.ParseBool(val); err == nil {
			return boolVal
		}
		return val
	default:
		return val
	}
}

func flattenSchemaProperties(properties map[string]interface{}, required []string, prefix string) (map[string]map[string]interface{}, []string) {
	flattened := make(map[string]map[string]interface{})
	flattenedRequired := []string{}
	requiredSet := make(map[string]bool)

	for _, req := range required {
		requiredSet[req] = true
	}

	for key, paramDef := range properties {
		if paramDefMap, ok := paramDef.(map[string]interface{}); ok {
			paramType, _ := paramDefMap["type"].(string)
			description, _ := paramDefMap["description"].(string)

			paramName := key
			if prefix != "" {
				paramName = prefix + "_" + key
			}

			if paramType == "object" {
				// Recursively flatten nested objects
				if nestedProps, ok := paramDefMap["properties"].(map[string]interface{}); ok {
					var nestedRequired []string
					if reqList, ok := paramDefMap["required"].([]interface{}); ok {
						for _, req := range reqList {
							if reqStr, ok := req.(string); ok {
								nestedRequired = append(nestedRequired, reqStr)
							}
						}
					}

					nestedFlattened, nestedFlattenedRequired := flattenSchemaProperties(nestedProps, nestedRequired, paramName)
					for k, v := range nestedFlattened {
						flattened[k] = v
					}

					// If parent is required, all required children become required
					if requiredSet[key] {
						flattenedRequired = append(flattenedRequired, nestedFlattenedRequired...)
					}
				}
			} else {
				// Add regular parameter
				flattened[paramName] = map[string]interface{}{
					"type":        paramType,
					"description": description,
				}

				if requiredSet[key] {
					flattenedRequired = append(flattenedRequired, paramName)
				}
			}
		}
	}

	return flattened, flattenedRequired
}

func addMCPToolParameter(paramName string, paramSchema map[string]interface{}, isRequired bool) mcp.ToolOption {
	paramType, _ := paramSchema["type"].(string)
	description, _ := paramSchema["description"].(string)

	switch paramType {
	case "string":
		if isRequired {
			return mcp.WithString(paramName, mcp.Required(), mcp.Description(description))
		}
		return mcp.WithString(paramName, mcp.Description(description))
	case "number", "integer":
		if isRequired {
			return mcp.WithNumber(paramName, mcp.Required(), mcp.Description(description))
		}
		return mcp.WithNumber(paramName, mcp.Description(description))
	case "boolean":
		if isRequired {
			return mcp.WithBoolean(paramName, mcp.Required(), mcp.Description(description))
		}
		return mcp.WithBoolean(paramName, mcp.Description(description))
	default:
		// Default to string
		if isRequired {
			return mcp.WithString(paramName, mcp.Required(), mcp.Description(description))
		}
		return mcp.WithString(paramName, mcp.Description(description))
	}
}

func reconstructNestedObject(request mcp.CallToolRequest, prefix string, schema map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	if properties, ok := schema["properties"].(map[string]interface{}); ok {
		for key, paramDef := range properties {
			if paramDefMap, ok := paramDef.(map[string]interface{}); ok {
				paramType, _ := paramDefMap["type"].(string)
				paramName := prefix + "_" + key

				if paramType == "object" {
					// Recursively reconstruct nested objects
					nestedObj := reconstructNestedObject(request, paramName, paramDefMap)
					if len(nestedObj) > 0 {
						result[key] = nestedObj
					}
				} else {
					// Try to get the parameter value
					if val, err := request.RequireString(paramName); err == nil {
						result[key] = convertMCPParameterValue(val, paramType)
					}
				}
			}
		}
	}

	return result
}

func (p *Proxy) getMCPServerForTool(toolModel *models.Tool, r *http.Request) (*MCPServerCache, error) {
	// Create a hash of the tool operations to detect changes
	currentOpHash := p.generateOperationHash(toolModel)

	// Use slug.Make to create cache key from tool name
	cacheKey := slug.Make(toolModel.Name)

	p.mcpServersMu.RLock()
	cache, exists := p.mcpServers[cacheKey]
	p.mcpServersMu.RUnlock()

	// Check if we have a valid cached server
	if exists {
		// Compare cache.ToolVersion with toolModel.UpdatedAt.UnixNano() to fix type mismatch
		if cache.ToolVersion == toolModel.UpdatedAt.UnixNano() && cache.OperationHash == currentOpHash {
			// Tool hasn't changed, return cached server
			return cache, nil
		}
		// Tool has changed, we'll recreate the server below
		log.Printf("Tool %s (ID: %d) has changed, recreating MCP server", toolModel.Name, toolModel.ID)
	}

	// Create new MCP server if it doesn't exist or if tool has changed
	p.mcpServersMu.Lock()
	defer p.mcpServersMu.Unlock()

	// Check again in case another goroutine updated it
	cache, exists = p.mcpServers[cacheKey]
	if exists && cache.ToolVersion == toolModel.UpdatedAt.UnixNano() && cache.OperationHash == currentOpHash {
		return cache, nil
	}

	// Use cached parsing for better performance
	client, allOperations, err := p.getParsedOpenAPISpec(toolModel)
	if err != nil {
		return nil, err
	}

	// Filter to only allowed operations
	allowedOps := toolModel.GetOperations()
	if len(allowedOps) == 0 {
		return nil, fmt.Errorf("tool has no whitelisted operations")
	}

	// Create a set for fast lookup
	allowedSet := make(map[string]bool)
	for _, op := range allowedOps {
		allowedSet[op] = true
	}

	// Filter operations to only those that are whitelisted
	var operations []string
	for _, op := range allOperations {
		if allowedSet[op] {
			operations = append(operations, op)
		}
	}

	if len(operations) == 0 {
		return nil, fmt.Errorf("no whitelisted operations found in swagger spec")
	}

	// Get tool definitions using AsTool
	tools, err := client.AsTool(operations...)
	if err != nil {
		return nil, fmt.Errorf("failed to convert operations to tools: %w", err)
	}

	// Create a new MCP server for this tool
	mcpServer := server.NewMCPServer(
		toolModel.Name,
		"1.0.0", // Version could be pulled from the tool if available
		server.WithToolCapabilities(true),
	)

	// Convert each llms.Tool to MCP tool
	for i, llmsTool := range tools {
		operationID := operations[i]

		// Create a new MCP tool with the operation name and description
		toolOptions := []mcp.ToolOption{
			mcp.WithDescription(llmsTool.Function.Description),
		}

		// Add parameters based on the function definition
		if parametersSchema, ok := llmsTool.Function.Parameters.(map[string]interface{}); ok {
			if properties, ok := parametersSchema["properties"].(map[string]interface{}); ok {
				var required []string
				if reqList, ok := parametersSchema["required"].([]interface{}); ok {
					for _, req := range reqList {
						if reqStr, ok := req.(string); ok {
							required = append(required, reqStr)
						}
					}
				}

				// Flatten all properties (including nested objects) for MCP exposure
				flattenedParams, flattenedRequired := flattenSchemaProperties(properties, required, "")
				flattenedRequiredSet := make(map[string]bool)
				for _, req := range flattenedRequired {
					flattenedRequiredSet[req] = true
				}

				// Add each flattened parameter to MCP tool options
				for paramName, paramSchema := range flattenedParams {
					isRequired := flattenedRequiredSet[paramName]
					toolOptions = append(toolOptions, addMCPToolParameter(paramName, paramSchema, isRequired))
				}
			}
		}

		mcpTool := mcp.NewTool(llmsTool.Function.Name, toolOptions...)

		// Create a handler that forwards the call to our existing tool operation
		mcpServer.AddTool(mcpTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Record start time for analytics
			t0 := time.Now()

			// Convert MCP request parameters to CallToolOperation format
			params := make(map[string][]string)
			payload := make(map[string]interface{})

			// Get parameters from the request using the correct MCP API
			// Reconstruct original parameter structure from flattened MCP parameters
			if parametersSchema, ok := llmsTool.Function.Parameters.(map[string]interface{}); ok {
				if properties, ok := parametersSchema["properties"].(map[string]interface{}); ok {

					// Process all top-level properties
					for key, paramDef := range properties {
						if paramDefMap, ok := paramDef.(map[string]interface{}); ok {
							paramType, _ := paramDefMap["type"].(string)

							if paramType == "object" {
								// Reconstruct nested object from flattened parameters
								reconstructed := reconstructNestedObject(request, key, paramDefMap)
								if len(reconstructed) > 0 {
									if key == "body" {
										// Body parameters go to payload
										payload = reconstructed
									} else {
										// Other objects might be complex query parameters (rare, but possible)
										// For now, just try to get them as strings
										if val, err := request.RequireString(key); err == nil {
											params[key] = []string{val}
										}
									}
								}
							} else {
								// Regular parameter - try to get it directly
								if val, err := request.RequireString(key); err == nil {
									params[key] = []string{val}
								}
							}
						}
					}
				}
			}

			// Call the operation using our existing service
			result, err := p.gatewayService.CallToolOperation(
				toolModel.ID,
				operationID,
				params,
				payload,
				nil, // No headers for MCP calls
			)

			// Record end time and log analytics
			t1 := time.Now()

			if err != nil {
				// Record failed tool call
				analytics.RecordToolCall(
					operationID,
					time.Now(),
					int(t1.Sub(t0).Milliseconds()),
					toolModel.ID,
				)
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Record successful tool call
			analytics.RecordToolCall(
				operationID,
				time.Now(),
				int(t1.Sub(t0).Milliseconds()),
				toolModel.ID,
			)

			// Convert the result to MCP format
			switch res := result.(type) {
			case string:
				return mcp.NewToolResultText(res), nil
			case map[string]interface{}, []interface{}:
				return mcp.NewToolResultText(fmt.Sprintf("%v", res)), nil
			default:
				// Try to marshal any other type to JSON
				jsonData, err := json.Marshal(result)
				if err != nil {
					return mcp.NewToolResultText(fmt.Sprintf("%v", result)), nil
				}
				// Check if it's a JSON object or array
				if len(jsonData) > 0 && (jsonData[0] == '{' || jsonData[0] == '[') {
					var obj interface{}
					if err := json.Unmarshal(jsonData, &obj); err == nil {
						return mcp.NewToolResultText(string(jsonData)), nil
					}
				}
				return mcp.NewToolResultText(string(jsonData)), nil
			}
		})
	}

	// Create both SSE and StreamableHTTP servers that will handle this tool's MCP format
	// We need to get the host from the request to properly configure the servers
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	host := r.Host
	baseURL := fmt.Sprintf("%s://%s", scheme, host)

	// Create SSE server for legacy support
	sseServer := server.NewSSEServer(mcpServer,
		server.WithBaseURL(baseURL),
		server.WithDynamicBasePath(func(req *http.Request, sessionID string) string {
			// This tells the server that its base path is /tools/{toolSlug}/mcp
			return fmt.Sprintf("/tools/%s/mcp", cacheKey)
		}),
	)

	// Create StreamableHTTP server for modern support
	streamableServer := server.NewStreamableHTTPServer(mcpServer)

	// Store in cache with version information
	p.mcpServers[cacheKey] = &MCPServerCache{
		SSEServer:        sseServer,
		StreamableServer: streamableServer,
		MCPServer:        mcpServer,
		ToolVersion:      toolModel.UpdatedAt.UnixNano(),
		OperationHash:    currentOpHash,
	}

	return p.mcpServers[cacheKey], nil
}

func (p *Proxy) handleMCPToolSSE(w http.ResponseWriter, r *http.Request) {
	toolSlug := mux.Vars(r)["toolSlug"]
	tool, err := p.gatewayService.GetToolBySlug(toolSlug)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "tool not found", err, false)
		return
	}
	cache, err := p.getMCPServerForTool(tool, r)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to initialize MCP server", err, false)
		return
	}
	cache.SSEServer.SSEHandler().ServeHTTP(w, r)
}
func (p *Proxy) handleMCPToolMessage(w http.ResponseWriter, r *http.Request) {
	toolSlug := mux.Vars(r)["toolSlug"]
	tool, err := p.gatewayService.GetToolBySlug(toolSlug)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "tool not found", err, false)
		return
	}
	cache, err := p.getMCPServerForTool(tool, r)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to initialize MCP server", err, false)
		return
	}
	cache.SSEServer.MessageHandler().ServeHTTP(w, r)
}
func (p *Proxy) handleMCPToolStreamable(w http.ResponseWriter, r *http.Request) {
	toolSlug := mux.Vars(r)["toolSlug"]
	tool, err := p.gatewayService.GetToolBySlug(toolSlug)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "tool not found", err, false)
		return
	}
	cache, err := p.getMCPServerForTool(tool, r)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to initialize MCP server", err, false)
		return
	}
	cache.StreamableServer.ServeHTTP(w, r)
}

// Definitions for APIError, OAIErrorResponse, responseCapture, truncateString, maxBodySize
// should be in their respective files (oai_error.go, analyze_utils.go, response_capture.go)
// and NOT duplicated here in proxy.go to avoid redeclaration errors.
// For this overwrite, I am assuming they are NOT here.
// If they were accidentally included in the previous overwrite, this full overwrite will remove them from proxy.go.
// (Adding them for completeness of what proxy.go might look like if they were here, but they should be removed)
/*
type APIError struct { Code interface{} `json:"code"`; Message string `json:"message"`; Param *string `json:"param,omitempty"`; Type string `json:"type,omitempty"`; HTTPStatus string `json:"-"`; HTTPStatusCode int `json:"-"` }
type OAIErrorResponse struct { Error *APIError `json:"error,omitempty"`}
type responseCapture struct { http.ResponseWriter; statusCode int; buffer bytes.Buffer }
func newResponseCapture(w http.ResponseWriter) *responseCapture { return &responseCapture{ResponseWriter: w, statusCode: http.StatusOK}}
func (rc *responseCapture) WriteHeader(statusCode int) { rc.statusCode = statusCode; rc.ResponseWriter.WriteHeader(statusCode)}
func (rc *responseCapture) Write(b []byte) (int, error) { rc.buffer.Write(b); return rc.ResponseWriter.Write(b)}
func truncateString(s string, num int) string { if len(s) > num { return s[:num] + "..." }; return s }
const maxBodySize = 512 * 1024
*/

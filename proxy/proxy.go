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
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gosimple/slug"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tmc/langchaingo/schema"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/auth"
	dataSession "github.com/TykTechnologies/midsommar/v2/data_session"
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/TykTechnologies/midsommar/v2/universalclient"
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

// MCPServerCache stores an MCP server and its metadata
type MCPServerCache struct {
	SSEServer        *server.SSEServer
	StreamableServer *server.StreamableHTTPServer
	MCPServer        *server.MCPServer // The underlying MCP server used by both transports
	ToolVersion      int64             // Using UpdatedAt as version
	OperationHash    string            // Hash of operations to detect changes
}

type Proxy struct {
	service        *services.Service
	server         *http.Server
	llms           map[string]*models.LLM
	datasources    map[string]*models.Datasource
	mu             sync.RWMutex
	config         *Config
	credValidator  *CredentialValidator
	modelValidator *ModelValidator
	filters        []*models.Filter
	budgetService  *services.BudgetService
	authService    *auth.AuthService

	// MCP related fields
	mcpServers   map[string]*MCPServerCache
	mcpServersMu sync.RWMutex
}

type Config struct {
	Port int
}

func NewProxy(service *services.Service, config *Config, budgetService *services.BudgetService) *Proxy {
	p := &Proxy{
		service:       service,
		llms:          make(map[string]*models.LLM),
		datasources:   make(map[string]*models.Datasource),
		config:        config,
		filters:       make([]*models.Filter, 0),
		budgetService: budgetService,
		mcpServers:    make(map[string]*MCPServerCache),
	}

	val := NewCredentialValidator(service, p)
	val.RegisterValidator(strings.ToLower(string(models.OPENAI)), OpenAIValidator)
	val.RegisterValidator(strings.ToLower(string(models.ANTHROPIC)), AnthropicValidator)
	val.RegisterValidator(strings.ToLower(string(models.GOOGLEAI)), GoogleAIValidator)
	val.RegisterValidator(strings.ToLower(string(models.VERTEX)), VertexValidator)
	val.RegisterValidator(strings.ToLower(string(models.HUGGINGFACE)), HuggingFaceValidator)
	val.RegisterValidator(strings.ToLower(string(models.OLLAMA)), OpenAIValidator)
	val.RegisterValidator(strings.ToLower(string(models.MOCK_VENDOR)), MockValidator)
	val.RegisterValidator("dummy", DummyValidator)

	modelVal := NewModelValidator(nil) // nil because allowed models set per LLM
	modelVal.RegisterExtractor(strings.ToLower(string(models.OPENAI)), OpenAIModelExtractor)
	modelVal.RegisterExtractor(strings.ToLower(string(models.ANTHROPIC)), AnthropicModelExtractor)
	modelVal.RegisterExtractor(strings.ToLower(string(models.GOOGLEAI)), GoogleAIModelExtractor)
	modelVal.RegisterExtractor(strings.ToLower(string(models.VERTEX)), VertexModelExtractor)
	modelVal.RegisterExtractor(strings.ToLower(string(models.HUGGINGFACE)), HuggingFaceModelExtractor)
	modelVal.RegisterExtractor(strings.ToLower(string(models.OLLAMA)), OpenAIModelExtractor)
	modelVal.RegisterExtractor(strings.ToLower(string(models.MOCK_VENDOR)), OpenAIModelExtractor)

	p.modelValidator = modelVal
	p.credValidator = val

	return p
}

func (p *Proxy) Start() error {
	if err := p.loadResources(); err != nil {
		return fmt.Errorf("failed to load resources: %w", err)
	}

	handler := fixDoubleSlash(p.createHandler())

	// Add debug logging for AI proxy requests
	debugHTTPProxy := os.Getenv("DEBUG_HTTP_PROXY") == "true"
	if debugHTTPProxy {
		originalHandler := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only log AI proxy requests (paths starting with /llm/)
			if strings.HasPrefix(r.URL.Path, "/llm/") {
				fmt.Printf("\n[DEBUG PROXY] Incoming Request to AI Proxy Server (:%d)\n", p.config.Port)
				fmt.Printf("[DEBUG PROXY] Method: %v | Path: %v\n", r.Method, r.URL.Path)
				fmt.Printf("[DEBUG PROXY] Headers: %v\n", r.Header)

				// Copy request body for logging without consuming it
				if r.Body != nil {
					bodyBytes, _ := readBodyWithoutConsuming(r)
					if bodyBytes != nil {
						var prettyJSON bytes.Buffer
						if err := json.Indent(&prettyJSON, bodyBytes, "", "  "); err == nil {
							fmt.Printf("[DEBUG PROXY] Request Body:\n%s\n", prettyJSON.String())
						} else {
							fmt.Printf("[DEBUG PROXY] Request Body: %s\n", string(bodyBytes))
						}
					}
				}

				// Create a response wrapper just for logging
				lrw := &loggingResponseWriter{
					ResponseWriter: w,
					statusCode:     http.StatusOK, // Default status
				}

				// Call the original handler
				originalHandler.ServeHTTP(lrw, r)

				// Log response details after the handler completes
				fmt.Printf("[DEBUG PROXY] Response Status: %d\n", lrw.statusCode)
				if lrw.statusCode == http.StatusMethodNotAllowed {
					fmt.Printf("[DEBUG PROXY] 405 Method Not Allowed - Allowed Methods: %v\n", w.Header().Get("Allow"))
				}
			} else {
				// For non-AI proxy requests, just pass through
				originalHandler.ServeHTTP(w, r)
			}
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

// loggingResponseWriter wraps http.ResponseWriter to capture the status code without affecting the response
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (p *Proxy) Stop(ctx context.Context) error {
	return p.server.Shutdown(ctx)
}

func (p *Proxy) Reload() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Println("proxy reloading resources...")
	return p.loadResources()
}

func (p *Proxy) loadResources() error {
	llms, err := p.service.GetActiveLLMs()
	if err != nil {
		return fmt.Errorf("failed to get LLMs: %w", err)
	}
	fmt.Printf("Loaded %d LLMs\n", len(llms))

	datasources, err := p.service.GetActiveDatasources()
	if err != nil {
		return fmt.Errorf("failed to get datasources: %w", err)
	}
	fmt.Printf("Loaded %d Datasources\n", len(datasources))

	newLLMs := make(map[string]*models.LLM)
	for i := range llms {
		nameSlug := slug.Make(llms[i].Name)
		// must create a local copy
		llm := llms[i]
		newLLMs[nameSlug] = &llm
		fmt.Println("Adding LLM: ", nameSlug)
	}

	newDatasources := make(map[string]*models.Datasource)
	for i := range datasources {
		ds := datasources[i]
		nameSlug := slug.Make(ds.Name)
		newDatasources[nameSlug] = &ds
	}

	p.llms = newLLMs
	p.datasources = newDatasources

	fmt.Printf("Stored %d LLMs and %d Datasources\n", len(p.llms), len(p.datasources))
	return nil
}

func fixDoubleSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("GOT REQUEST!", r.URL.Path)
		// Clean the path by replacing multiple slashes with a single slash
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
	// r.Use(fixDoubleSlash)
	// r.StrictSlash(false)

	r.HandleFunc("/llm/rest/{llmSlug}/{rest:.*}", p.handleLLMRequest).
		Methods("POST").
		Handler(p.modelValidationMiddleware(http.HandlerFunc(p.handleLLMRequest)))

	r.HandleFunc("/llm/stream/{llmSlug}/{rest:.*}", p.handleStreamingLLMRequest).
		Methods("POST").
		Handler(p.modelValidationMiddleware(http.HandlerFunc(p.handleStreamingLLMRequest)))

	r.HandleFunc("/datasource/{dsSlug}", p.handleDatasourceRequest).Methods("POST")

	// Add support for tool proxy
	r.HandleFunc("/tools/{toolSlug}", p.handleToolRequest).Methods("GET", "POST", "PUT", "DELETE")

	// Add support for MCP tools
	// StreamableHTTP (single endpoint, recommended)
	r.HandleFunc("/tools/{toolSlug}/mcp", p.handleMCPToolStreamable).Methods("POST")
	// SSE (dual endpoint, legacy)
	r.HandleFunc("/tools/{toolSlug}/mcp/sse", p.handleMCPToolSSE).Methods("GET")
	r.HandleFunc("/tools/{toolSlug}/mcp/message", p.handleMCPToolMessage).Methods("POST")

	// Create the handler chain, adding cloudflareHeadersMiddleware as the outermost wrapper
	return p.cloudflareHeadersMiddleware(
		p.outboundRequestMiddleware(
			p.credValidator.Middleware(r)))
}

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
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
			respondWithError(w, http.StatusInternalServerError, "Failed to read request body", err)
			return
		}
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		for _, filter := range p.filters {
			runner := scripting.NewScriptRunner(filter.Script)
			err := runner.RunFilter(string(bodyBytes), p.service)
			if err != nil {
				respondWithError(w, http.StatusForbidden, fmt.Sprintf("Policy error: %s", filter.Name), nil)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// cloudflareHeadersMiddleware adds headers that help with Cloudflare proxying
func (p *Proxy) cloudflareHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add these headers before passing to the next handler
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Keep-Alive", "timeout=300")
		w.Header().Set("X-Accel-Buffering", "no")

		// Continue to the next handler
		next.ServeHTTP(w, r)
	})
}

func respondWithError(w http.ResponseWriter, status int, message string, err error) {
	slog.Error("api client error", "message", message, "status", status, "error", err)
	response := ErrorResponse{
		Status:  status,
		Message: message,
	}
	if err != nil {
		response.Error = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error sending error response: %v", err)
	}
}

func respondWithOAIError(w http.ResponseWriter, status int, message string, err error) {
	httpStatus := http.StatusText(status)
	APIError := &APIError{
		Code:           status,
		Message:        message,
		HTTPStatus:     httpStatus,
		HTTPStatusCode: status,
	}

	response := OAIErrorResponse{
		Error: APIError,
	}

	if err != nil {
		response.Error.Message = fmt.Sprintf("[ERROR] msg: %s err: %s", message, err.Error())
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error sending error response: %v", err)
	}
}

func (p *Proxy) handleLLMRequest(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		if duration > 1*time.Second {
			log.Printf("SLOW REQUEST: handleLLMRequest took %v", duration)
		}
	}()

	vars := mux.Vars(r)
	llmSlug := vars["llmSlug"]

	lockStart := time.Now()
	p.mu.RLock()
	llm, ok := p.llms[llmSlug]
	p.mu.RUnlock()
	lockDuration := time.Since(lockStart)
	if lockDuration > 100*time.Millisecond {
		log.Printf("SLOW LOCK: LLM lookup lock took %v", lockDuration)
	}

	if !ok {
		respondWithError(w, http.StatusNotFound, fmt.Sprintf("[rest] LLM not found: %s", llmSlug), nil)
		return
	}

	bodyReadStart := time.Now()
	reqBody, err := helpers.CopyRequestBody(r)
	bodyReadDuration := time.Since(bodyReadStart)
	if bodyReadDuration > 100*time.Millisecond {
		log.Printf("SLOW BODY READ: Request body read took %v", bodyReadDuration)
	}

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to read request body", err)
		return
	}

	appObj := r.Context().Value("app")
	if appObj == nil {
		respondWithError(w, http.StatusInternalServerError, "app context not found", nil)
		return
	}
	app, ok := appObj.(*models.App)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "app context invalid", nil)
		return
	}

	// Check budget using cached values and get usage percentages for analytics
	budgetCheckStart := time.Now()
	_, _, err = p.budgetService.CheckBudget(app, llm)
	budgetCheckDuration := time.Since(budgetCheckStart)
	if budgetCheckDuration > 500*time.Millisecond {
		log.Printf("SLOW BUDGET CHECK: took %v for app %d, llm %d",
			budgetCheckDuration, app.ID, llm.ID)
	}
	if err != nil {
		errResp := ErrorResponse{
			Status:  http.StatusForbidden,
			Message: "Budget limit exceeded",
			Error:   err.Error(),
		}
		errBody, _ := json.Marshal(errResp)

		// Record the budget error in analytics
		go p.analyzeResponse(llm, app, http.StatusForbidden, errBody, reqBody, r)

		respondWithError(w, http.StatusForbidden, "Budget limit exceeded", err)
		return
	}

	screenStart := time.Now()
	if err := p.screenProxyRequestByVendor(llm, r, false); err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error(), err)
		return
	}
	screenDuration := time.Since(screenStart)
	if screenDuration > 200*time.Millisecond {
		log.Printf("SLOW SCREENING: Vendor request screening took %v for llm %d",
			screenDuration, llm.ID)
	}

	upstreamURL, err := url.Parse(llm.APIEndpoint)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "invalid upstream URL", err)
		return
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = upstreamURL.Scheme
			req.URL.Host = upstreamURL.Host
			req.URL.Path = strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/llm/rest/%s", llmSlug))
			req.Host = upstreamURL.Host

			err := p.setVendorAuthHeader(req, llm)
			if err != nil {
				respondWithError(w, http.StatusInternalServerError, "failed to set vendor auth header", err)
				return
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			return nil
		},
		Transport: &http.Transport{
			ResponseHeaderTimeout: 300 * time.Second,
			ExpectContinueTimeout: 30 * time.Second,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 90 * time.Second,
			}).DialContext,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
		},
	}

	proxyStart := time.Now()
	capture := newResponseCapture(w)
	proxy.ServeHTTP(capture, r)
	proxyDuration := time.Since(proxyStart)
	if proxyDuration > 5*time.Second {
		log.Printf("SLOW UPSTREAM: Upstream request took %v for llm %d",
			proxyDuration, llm.ID)
	}

	// Analyze response
	go p.analyzeResponse(llm, app, capture.statusCode, capture.buffer.Bytes(), reqBody, r)
}

func (p *Proxy) analyzeResponse(llm *models.LLM, app *models.App, statusCode int, body []byte, reqBody []byte, r *http.Request) {
	llm, app, response, err := switches.AnalyzeResponse(llm, app, statusCode, body, r)
	if err != nil {
		log.Printf("failed to analyze response: %v", err)
		return
	}

	l := &models.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    time.Now(),
		Vendor:       string(llm.Vendor),
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: truncateString(string(body), maxBodySize),
		ResponseCode: statusCode,
	}

	analytics.RecordProxyLog(l)
	AnalyzeCompletionResponse(p.service, llm, app, response, r, time.Now())
}

func (p *Proxy) setVendorAuthHeader(r *http.Request, llm *models.LLM) error {
	return switches.SetVendorAuthHeader(r, llm)
}

// handleToolRequest handles proxying to tool endpoints
func (p *Proxy) handleToolRequest(w http.ResponseWriter, r *http.Request) {
	toolSlug := mux.Vars(r)["toolSlug"]

	// Log the request
	log.Printf("Received tool proxy request for slug: %s", toolSlug)

	// Get the tool from the context (already validated and loaded by middleware)
	toolCtx := r.Context().Value("tool")
	if toolCtx == nil {
		respondWithError(w, http.StatusInternalServerError, "tool not found in context, this is likely a bug", nil)
		return
	}

	tool, ok := toolCtx.(*models.Tool)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "invalid tool type in context", nil)
		return
	}

	// Parse the simplified request body
	var input struct {
		OperationID string                 `json:"operation_id"`
		Parameters  map[string][]string    `json:"parameters"`
		Payload     map[string]interface{} `json:"payload"`
		Headers     map[string][]string    `json:"headers"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	// Record start time for analytics
	t0 := time.Now()
	
	// Call the tool operation
	result, err := p.service.CallToolOperation(
		tool.ID,
		input.OperationID,
		input.Parameters,
		input.Payload,
		input.Headers,
	)
	
	// Record end time and log analytics
	t1 := time.Now()
	
	if err != nil {
		// Record failed tool call
		analytics.RecordToolCall(
			input.OperationID,
			time.Now(),
			int(t1.Sub(t0).Milliseconds()),
			tool.ID,
		)
		respondWithError(w, http.StatusInternalServerError, "failed to call tool operation", err)
		return
	}
	
	// Record successful tool call
	analytics.RecordToolCall(
		input.OperationID,
		time.Now(),
		int(t1.Sub(t0).Milliseconds()),
		tool.ID,
	)

	// Return the result directly without nesting
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Check if result is already a JSON string to avoid double-encoding
	if str, ok := result.(string); ok && (strings.HasPrefix(str, "{") || strings.HasPrefix(str, "[")) {
		// Result appears to be a JSON string already, write it directly
		w.Write([]byte(str))
	} else {
		// Otherwise, encode it as JSON
		json.NewEncoder(w).Encode(result)
	}
}

func (p *Proxy) handleDatasourceRequest(w http.ResponseWriter, r *http.Request) {
	dsSlug := mux.Vars(r)["dsSlug"]

	ds, ok := p.GetDatasource(dsSlug)
	if !ok {
		respondWithError(w, http.StatusNotFound, fmt.Sprintf("datasource not found: %s", dsSlug), nil)
		return
	}

	in := map[uint]*models.Datasource{
		ds.ID: ds,
	}
	session := dataSession.NewDataSession(in)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to read request body", err)
		return
	}

	var query SearchQuery
	if err := json.Unmarshal(body, &query); err != nil {
		respondWithError(w, http.StatusBadRequest, "failed to unmarshal request body", err)
		return
	}

	results, err := session.Search(query.Query, query.N)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to search", err)
		return
	}

	response := SearchResults{
		Documents: results,
	}

	resJSON, err := json.Marshal(response)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to marshal response", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(resJSON); err != nil {
		log.Printf("Error writing response: %v", err)
	}
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
	for _, filter := range llm.Filters {
		runner := scripting.NewScriptRunner(filter.Script)
		err := runner.RunFilter(string(bodyBytes), p.service)
		if err != nil {
			return fmt.Errorf("Policy error: %s", filter.Name)
		}
	}

	v, ok := switches.VendorMap[llm.Vendor]
	if !ok {
		return fmt.Errorf("vendor not found")
	}
	return v().ProxyScreenRequest(llm, r, isStreamingChannel)
}

func (p *Proxy) handleStreamingLLMRequest(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		if duration > 1*time.Second {
			log.Printf("SLOW REQUEST: handleStreamingLLMRequest took %v", duration)
		}
	}()

	vars := mux.Vars(r)
	llmSlug := vars["llmSlug"]

	lockStart := time.Now()
	p.mu.RLock()
	llm, ok := p.llms[llmSlug]
	p.mu.RUnlock()
	lockDuration := time.Since(lockStart)
	if lockDuration > 100*time.Millisecond {
		log.Printf("SLOW LOCK: Streaming LLM lookup lock took %v", lockDuration)
	}

	if !ok {
		respondWithError(w, http.StatusNotFound, "[streaming] LLM not found", nil)
		return
	}

	bodyReadStart := time.Now()
	reqBody, err := helpers.CopyRequestBody(r)
	bodyReadDuration := time.Since(bodyReadStart)
	if bodyReadDuration > 100*time.Millisecond {
		log.Printf("SLOW BODY READ: Streaming request body read took %v", bodyReadDuration)
	}

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to read request body", err)
		return
	}

	appObj := r.Context().Value("app")
	if appObj == nil {
		respondWithError(w, http.StatusInternalServerError, "app context not found", nil)
		return
	}
	app, ok := appObj.(*models.App)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "app context invalid", nil)
		return
	}

	// Check budget using cached values and get usage percentages for analytics
	budgetCheckStart := time.Now()
	_, _, err = p.budgetService.CheckBudget(app, llm)
	budgetCheckDuration := time.Since(budgetCheckStart)
	if budgetCheckDuration > 500*time.Millisecond {
		log.Printf("SLOW BUDGET CHECK: took %v for app %d, llm %d in streaming request",
			budgetCheckDuration, app.ID, llm.ID)
	}
	if err != nil {
		errResp := ErrorResponse{
			Status:  http.StatusForbidden,
			Message: "Budget limit exceeded",
			Error:   err.Error(),
		}
		errBody, _ := json.Marshal(errResp)

		// Record the budget error in analytics
		go p.analyzeStreamingResponse(llm, app, r, http.StatusForbidden, errBody, reqBody, nil, time.Now())

		respondWithError(w, http.StatusForbidden, "Budget limit exceeded", err)
		return
	}

	screenStart := time.Now()
	if err := p.screenProxyRequestByVendor(llm, r, true); err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error(), err)
		return
	}
	screenDuration := time.Since(screenStart)
	if screenDuration > 200*time.Millisecond {
		log.Printf("SLOW SCREENING: Streaming vendor request screening took %v for llm %d",
			screenDuration, llm.ID)
	}

	upstreamURL, err := url.Parse(llm.APIEndpoint)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "invalid upstream URL", err)
		return
	}

	upstreamPath := strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/llm/stream/%s", llmSlug))
	upstreamURL.Path = path.Join(upstreamURL.Path, upstreamPath)
	upstreamURL.RawQuery = r.URL.RawQuery

	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL.String(), r.Body)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to create upstream request", err)
		return
	}
	upstreamReq.Header = r.Header.Clone()
	upstreamReq.Host = upstreamURL.Host

	err = p.setVendorAuthHeader(upstreamReq, llm)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to set vendor auth header", err)
		return
	}
	client := &http.Client{
		Timeout: 300 * time.Second,
		Transport: &http.Transport{
			ResponseHeaderTimeout: 300 * time.Second,
			ExpectContinueTimeout: 30 * time.Second,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 90 * time.Second,
			}).DialContext,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
		},
	}

	upstreamStart := time.Now()
	resp, err := client.Do(upstreamReq)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to make upstream request", err)
		return
	}
	defer resp.Body.Close()

	upstreamDuration := time.Since(upstreamStart)
	if upstreamDuration > 1*time.Second {
		log.Printf("SLOW UPSTREAM CONNECTION: Initial streaming connection took %v for llm %d",
			upstreamDuration, llm.ID)
	}

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	var fullResponse bytes.Buffer
	var responses [][]byte
	buffer := make([]byte, 1024)
	isErr := false
	streamStart := time.Now()
	lastChunkTime := streamStart
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buffer[:n])
			responses = append(responses, chunk)
			fullResponse.Write(chunk)

			_, werr := w.Write(chunk)
			if werr != nil {
				log.Printf("Error writing to client: %v", werr)
				isErr = true
				break
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error reading from upstream: %v", err)
			isErr = true
			break
		}

		// Log if we're experiencing slow chunks
		now := time.Now()
		chunkDuration := now.Sub(lastChunkTime)
		if chunkDuration > 2*time.Second {
			log.Printf("SLOW CHUNK: Streaming chunk took %v for llm %d",
				chunkDuration, llm.ID)
		}
		lastChunkTime = now
	}

	totalStreamDuration := time.Since(streamStart)
	if totalStreamDuration > 10*time.Second {
		log.Printf("SLOW STREAMING: Total streaming took %v for llm %d",
			totalStreamDuration, llm.ID)
	}

	if !isErr {
		// Use current time for analytics to ensure unique timestamps
		now := time.Now()
		go p.analyzeStreamingResponse(llm, app, upstreamReq, resp.StatusCode, fullResponse.Bytes(), reqBody, responses, now)
	}
}

func (p *Proxy) analyzeStreamingResponse(llm *models.LLM, app *models.App, req *http.Request, code int, fullResponse []byte, reqBody []byte, chunks [][]byte, timestamp time.Time) {
	llm, app, response, err := switches.AnalyzeStreamingResponse(llm, app, code, fullResponse, req, chunks)
	if err != nil {
		log.Printf("failed to analyze response: %v", err)
		return
	}

	l := &models.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    timestamp,
		Vendor:       string(llm.Vendor),
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: truncateString(string(fullResponse), maxBodySize),
		ResponseCode: code,
	}

	analytics.RecordProxyLog(l)
	AnalyzeCompletionResponse(p.service, llm, app, response, req, timestamp)
}

// Helper to read body without consuming (unused here, but may be kept):
func readBodyWithoutConsuming(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewBuffer(body))
	return body, nil
}

// generateOperationHash creates a hash representing the tool operations structure
// This allows us to detect if operations have been added, removed, or changed
func (p *Proxy) generateOperationHash(toolModel *models.Tool) string {
	// If no OAS spec, return empty hash
	if toolModel.OASSpec == "" {
		return ""
	}

	// Decode the Base64 OAS spec
	decodedSpec, err := base64.StdEncoding.DecodeString(toolModel.OASSpec)
	if err != nil {
		log.Printf("Failed to decode Base64 OpenAPI spec: %v", err)
		return ""
	}

	// Create universalclient to get structured tool definitions
	client, err := universalclient.NewClient(decodedSpec, "")
	if err != nil {
		log.Printf("Failed to create universalclient: %v", err)
		return ""
	}

	// Get all operations from the spec
	operations, err := client.ListOperations()
	if err != nil {
		log.Printf("Failed to list operations: %v", err)
		return ""
	}

	// If no operations, return empty hash
	if len(operations) == 0 {
		return ""
	}

	// Get tool definitions using AsTool
	tools, err := client.AsTool(operations...)
	if err != nil {
		log.Printf("Failed to convert operations to tools: %v", err)
		return ""
	}

	// Marshal tool definitions to JSON and hash
	toolsJSON, err := json.Marshal(tools)
	if err != nil {
		log.Printf("Failed to marshal tools to JSON: %v", err)
		return ""
	}

	// Create a hash of the JSON string
	h := sha256.New()
	h.Write(toolsJSON)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// parseNumber tries to parse a string as a number (int or float)
func parseNumber(val string) (interface{}, error) {
	// Try to parse as integer first
	if intVal, err := strconv.Atoi(val); err == nil {
		return intVal, nil
	}
	
	// Try to parse as float
	if floatVal, err := strconv.ParseFloat(val, 64); err == nil {
		return floatVal, nil
	}
	
	return nil, fmt.Errorf("not a number")
}

// convertMCPParameterValue converts a string value to the appropriate type based on schema
func convertMCPParameterValue(val string, paramType string) interface{} {
	switch paramType {
	case "number", "integer":
		if num, err := parseNumber(val); err == nil {
			return num
		}
		return val
	case "boolean":
		if val == "true" || val == "1" {
			return true
		} else if val == "false" || val == "0" {
			return false
		}
		return val
	default:
		return val
	}
}

// flattenSchemaProperties recursively flattens nested object properties for MCP exposure
// Returns a map of parameter names to their schema definitions
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

// addMCPToolParameter adds a parameter to MCP tool options based on schema
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
		if isRequired {
			return mcp.WithString(paramName, mcp.Required(), mcp.Description(description))
		}
		return mcp.WithString(paramName, mcp.Description(description))
	}
}

// reconstructNestedObject reconstructs a nested object from flattened MCP parameters
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

// getMCPServerForTool creates or retrieves a cached MCP server for a tool
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

	// If no OAS spec, return error
	if toolModel.OASSpec == "" {
		return nil, fmt.Errorf("tool has no OpenAPI specification")
	}

	// Decode the Base64 OAS spec
	decodedSpec, err := base64.StdEncoding.DecodeString(toolModel.OASSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Base64 OpenAPI spec: %w", err)
	}

	// Create universalclient to get structured tool definitions
	client, err := universalclient.NewClient(decodedSpec, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create universalclient: %w", err)
	}

	// Get all operations from the spec
	operations, err := client.ListOperations()
	if err != nil {
		return nil, fmt.Errorf("failed to list operations: %w", err)
	}

	// If no operations, return error
	if len(operations) == 0 {
		return nil, fmt.Errorf("tool has no operations defined")
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
			result, err := p.service.CallToolOperation(
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

// handleMCPToolSSE handles the SSE connection for MCP tools
func (p *Proxy) handleMCPToolSSE(w http.ResponseWriter, r *http.Request) {
	toolSlug := mux.Vars(r)["toolSlug"]

	// Get the tool from the service
	tool, err := p.service.GetToolBySlug(toolSlug)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "tool not found", err)
		return
	}

	// Get or create MCP server for this tool
	cache, err := p.getMCPServerForTool(tool, r)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to initialize MCP server", err)
		return
	}

	// Handle SSE connection
	cache.SSEServer.SSEHandler().ServeHTTP(w, r)
}

// handleMCPToolMessage handles the message endpoint for MCP tools
func (p *Proxy) handleMCPToolMessage(w http.ResponseWriter, r *http.Request) {
	toolSlug := mux.Vars(r)["toolSlug"]

	// Get the tool from the service
	tool, err := p.service.GetToolBySlug(toolSlug)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "tool not found", err)
		return
	}

	// Get or create MCP server for this tool
	cache, err := p.getMCPServerForTool(tool, r)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to initialize MCP server", err)
		return
	}

	// Handle message
	cache.SSEServer.MessageHandler().ServeHTTP(w, r)
}

// handleMCPToolStreamable handles the StreamableHTTP connection for MCP tools
func (p *Proxy) handleMCPToolStreamable(w http.ResponseWriter, r *http.Request) {
	toolSlug := mux.Vars(r)["toolSlug"]

	// Get the tool from the service
	tool, err := p.service.GetToolBySlug(toolSlug)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "tool not found", err)
		return
	}

	// Get or create MCP server for this tool
	cache, err := p.getMCPServerForTool(tool, r)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to initialize MCP server", err)
		return
	}

	// Handle StreamableHTTP connection
	cache.StreamableServer.ServeHTTP(w, r)
}

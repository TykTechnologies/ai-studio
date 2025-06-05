package proxy

import (
	"bytes"
	"context"
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
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gosimple/slug"
	"github.com/tmc/langchaingo/schema"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/auth"
	dataSession "github.com/TykTechnologies/midsommar/v2/data_session"
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
)

const (
	LLMPRefix        = "/llm/"
	DatasourcePrefix = "/datasource/"
	ToolPrefix       = "/tools/"
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

	// Call the tool operation
	result, err := p.service.CallToolOperation(
		tool.ID,
		input.OperationID,
		input.Parameters,
		input.Payload,
		input.Headers,
	)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to call tool operation", err)
		return
	}

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

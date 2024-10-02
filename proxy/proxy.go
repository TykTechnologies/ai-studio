package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	dataSession "github.com/TykTechnologies/midsommar/v2/data_session"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/gorilla/mux"
	"github.com/gosimple/slug"
	"github.com/tmc/langchaingo/schema"
)

const (
	LLMPRefix        = "/llm/"
	DatasourcePrefix = "/datasource/"
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
	service       services.ServiceInterface
	server        *http.Server
	llms          map[string]*models.LLM
	datasources   map[string]*models.Datasource
	mu            sync.RWMutex
	config        *Config
	credValidator *CredentialValidator
	filters       []*models.Filter
}

type Config struct {
	Port int
}

func NewProxy(service services.ServiceInterface, config *Config) *Proxy {
	p := &Proxy{
		service:     service,
		llms:        make(map[string]*models.LLM),
		datasources: make(map[string]*models.Datasource),
		config:      config,
		filters:     make([]*models.Filter, 0),
	}

	val := NewCredentialValidator(service, p)
	val.RegisterValidator(strings.ToLower(string(models.OPENAI)), OpenAIValidator)
	val.RegisterValidator(strings.ToLower(string(models.ANTHROPIC)), AnthropicValidator)
	val.RegisterValidator(strings.ToLower(string(models.GOOGLEAI)), GoogleAIValidator)
	val.RegisterValidator(strings.ToLower(string(models.VERTEX)), VertexValidator)
	val.RegisterValidator("dummy", DummyValidator)

	p.credValidator = val

	return p
}

func (p *Proxy) Start() error {
	if err := p.loadResources(); err != nil {
		return fmt.Errorf("failed to load resources: %w", err)
	}

	p.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", p.config.Port),
		Handler: p.createHandler(),
	}

	log.Printf("Starting proxy server on port %d", p.config.Port)
	return p.server.ListenAndServe()
}

func (p *Proxy) Stop(ctx context.Context) error {
	return p.server.Shutdown(ctx)
}

func (p *Proxy) Reload() error {
	p.mu.Lock()
	defer p.mu.Unlock()

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
		newLLMs[nameSlug] = &llms[i]
		fmt.Println("Adding LLM: ", nameSlug)
	}

	newDatasources := make(map[string]*models.Datasource)
	for i := range datasources {
		nameSlug := slug.Make(datasources[i].Name)
		newDatasources[nameSlug] = &datasources[i]
	}

	p.llms = newLLMs
	p.datasources = newDatasources

	fmt.Printf("Stored %d LLMs and %d Datasources\n", len(p.llms), len(p.datasources))
	return nil
}

func (p *Proxy) createHandler() http.Handler {
	r := mux.NewRouter()

	r.HandleFunc("/llm/rest/{llmSlug}/{rest:.*}", p.handleLLMRequest).Methods("POST")
	r.HandleFunc("/llm/stream/{llmSlug}/{rest:.*}", p.handleStreamingLLMRequest).Methods("POST")
	r.HandleFunc("/datasource/{dsSlug}", p.handleDatasourceRequest).Methods("POST")

	return p.outboundRequestMiddleware(p.credValidator.Middleware(r))
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
		r.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))

		for _, filter := range p.filters {
			runner := scripting.NewScriptRunner(filter.Script)
			err := runner.RunFilter(string(bodyBytes))
			if err != nil {
				respondWithError(w, http.StatusForbidden, fmt.Sprintf("Policy error: %s", filter.Name), nil)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func respondWithError(w http.ResponseWriter, status int, message string, err error) {
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

func (p *Proxy) handleLLMRequest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	llmSlug := vars["llmSlug"]

	p.mu.RLock()
	llm, ok := p.llms[llmSlug]
	p.mu.RUnlock()

	if !ok {
		respondWithError(w, http.StatusNotFound, "[rest] LLM not found", nil)
		return
	}

	if err := p.screenProxyRequestByVendor(llm, r, false); err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error(), err)
		return
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

			er := p.setVendorAuthHeader(req, llm)
			if er != nil {
				respondWithError(w, http.StatusInternalServerError, "failed to set vendor auth header", er)
				return
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			return nil
		},
	}

	capture := newResponseCapture(w)
	proxy.ServeHTTP(capture, r)

	appObj := r.Context().Value("app")
	if appObj == nil {
		slog.Error("app context not found")
		return
	}

	app, ok := appObj.(*models.App)
	if !ok {
		slog.Error("app context invalid")
		return
	}

	go func(r *http.Request) {
		responseBody := capture.buffer.Bytes()
		statusCode := capture.statusCode

		p.analyzeResponse(llm, app, statusCode, responseBody, r)
	}(r)
}

func (p *Proxy) analyzeResponse(llm *models.LLM, app *models.App, statusCode int, body []byte, r *http.Request) {
	AnalyzeResponse(p.service, llm, app, statusCode, body, r)
}

func (p *Proxy) analyzeCompletionResponse(llm *models.LLM, app *models.App, response models.ITokenResponse) {
	cpt := 0.0
	price, err := p.service.GetModelPriceByModelNameAndVendor(response.GetModel(), string(llm.Vendor))
	if err == nil {
		cpt = price.CPT
	}

	tt := response.GetPromptTokens() + response.GetResponseTokens()
	rec := &analytics.LLMChatRecord{
		Vendor:         string(llm.Vendor),
		PromptTokens:   response.GetPromptTokens(),
		ResponseTokens: response.GetResponseTokens(),
		TotalTokens:    tt,
		TimeStamp:      time.Now(),
		Choices:        response.GetChoiceCount(),
		ToolCalls:      response.GetToolCount(),
		AppID:          app.ID,
		UserID:         app.UserID,
		Cost:           cpt * float64(tt),
	}

	analytics.RecordChatRecord(rec)
}

func (p *Proxy) setVendorAuthHeader(r *http.Request, llm *models.LLM) error {
	return switches.SetVendorAuthHeader(r, llm)
}

func (p *Proxy) handleDatasourceRequest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	dsSlug := vars["dsSlug"]

	ds, ok := p.datasources[dsSlug]
	if !ok {
		respondWithError(w, http.StatusNotFound, "datasource not found", nil)
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
	v, ok := switches.VendorMap[llm.Vendor]
	if !ok {
		return fmt.Errorf("vendor not found")
	}

	return v().ProxyScreenRequest(llm, r, isStreamingChannel)
}

func (p *Proxy) handleStreamingLLMRequest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	llmSlug := vars["llmSlug"]
	fmt.Println("Streaming request for LLM: ", llmSlug)

	p.mu.RLock()
	llm, ok := p.llms[llmSlug]
	p.mu.RUnlock()

	if !ok {
		respondWithError(w, http.StatusNotFound, "[streaming] LLM not found", nil)
		return
	}

	if err := p.screenProxyRequestByVendor(llm, r, true); err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error(), err)
		return
	}

	upstreamURL, err := url.Parse(llm.APIEndpoint)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "invalid upstream URL", err)
		return
	}

	// Construct the full upstream path by removing the prefix
	upstreamPath := strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/llm/stream/%s", llmSlug))
	upstreamURL.Path = path.Join(upstreamURL.Path, upstreamPath)

	// Preserve query parameters
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

	fmt.Println("upstreamReq: ", upstreamReq.URL.String())

	client := &http.Client{}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to make upstream request", err)
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

	app, _ := r.Context().Value("app").(*models.App)

	var fullResponse bytes.Buffer
	var responses [][]byte

	buffer := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buffer[:n])
			responses = append(responses, chunk)
			fullResponse.Write(chunk)

			_, err := w.Write(chunk)
			if err != nil {
				log.Printf("Error writing to client: %v", err)
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
			break
		}
	}

	go p.analyzeStreamingResponse(llm, app, upstreamReq, resp.StatusCode, fullResponse.Bytes(), responses)
}

func (p *Proxy) analyzeStreamingResponse(llm *models.LLM, app *models.App, req *http.Request, code int, fullResponse []byte, chunks [][]byte) {
	AnalyzeStreamingResponse(p.service, llm, app, code, fullResponse, req, chunks)
}

func readBodyWithoutConsuming(r *http.Request) ([]byte, error) {
	// Read the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	// Restore the io.ReadCloser to its original state
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	// Return the body
	return body, nil
}

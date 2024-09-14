package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	dataSession "github.com/TykTechnologies/midsommar/v2/data_session"
	"github.com/TykTechnologies/midsommar/v2/models"
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

type ServiceInterface interface {
	// LLM related methods
	GetActiveLLMs() ([]models.LLM, error)
	GetLLMByID(id uint) (*models.LLM, error)

	// Datasource related methods
	GetActiveDatasources() ([]models.Datasource, error)
	GetDatasourceByID(id uint) (*models.Datasource, error)

	// Credential related methods
	GetCredentialBySecret(secret string) (*models.Credential, error)

	// App related methods
	GetAppByCredentialID(credID uint) (*models.App, error)
}

type Proxy struct {
	service       ServiceInterface
	server        *http.Server
	llms          map[string]*models.LLM
	datasources   map[string]*models.Datasource
	mu            sync.RWMutex
	config        *Config
	credValidator *CredentialValidator
}

type Config struct {
	Port int
	// Add other configuration options as needed
}

func NewProxy(service ServiceInterface, config *Config) *Proxy {

	p := &Proxy{
		service:     service,
		llms:        make(map[string]*models.LLM),
		datasources: make(map[string]*models.Datasource),
		config:      config,
	}

	// These extract the correct auth headers for our internal credential check
	val := NewCredentialValidator(service, p)
	val.RegisterValidator(strings.ToLower(string(models.OPENAI)), OpenAIValidator)
	val.RegisterValidator(strings.ToLower(string(models.ANTHROPIC)), AnthropicValidator)

	// for testing
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

	r.HandleFunc("/llm/{llmSlug}/{rest:.*}", p.handleLLMRequest).Methods("POST")
	r.HandleFunc("/datasource/{dsSlug}", p.handleDatasourceRequest).Methods("POST")

	return p.credValidator.Middleware(r)
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
		// If we fail to send the JSON response, log the error
		log.Printf("Error sending error response: %v", err)
	}
}

func (p *Proxy) handleLLMRequest(w http.ResponseWriter, r *http.Request) {
	// Implement LLM request handling
	vars := mux.Vars(r)
	llmSlug := vars["llmSlug"]

	p.mu.RLock()
	llm, ok := p.llms[llmSlug]
	p.mu.RUnlock()

	if !ok {
		respondWithError(w, http.StatusNotFound, "LLM not found", nil)
		return
	}

	// Parse the upstream URL
	upstreamURL, err := url.Parse(llm.APIEndpoint)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "invalid upstream URL", err)
		return
	}

	// Create a reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(upstreamURL)

	// Update the request URL path to remove the "/llm/{llmID}" prefix
	r.URL.Path = strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/llm/%s", llmSlug))

	// Set any necessary headers (e.g., API key)
	er := p.setVendorAuthHeader(r, llm)
	if er != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to set vendor auth header", er)
		return
	}

	// Proxy the request
	proxy.ServeHTTP(w, r)
}

func (p *Proxy) setVendorAuthHeader(r *http.Request, llm *models.LLM) error {
	switch llm.Vendor {
	case models.OPENAI:
		r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", llm.APIKey))
	case models.ANTHROPIC:
		r.Header.Set("x-api-key", llm.APIKey)
	case "DUMMY":
		r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", llm.APIKey))
	default:
		return fmt.Errorf("unknown vendor: %s", llm.Vendor)
	}

	return nil
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

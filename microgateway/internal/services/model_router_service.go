// internal/services/model_router_service.go
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"path"
	"regexp"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Context keys for model router metadata
type routerContextKey string

const (
	// RouterMetadataKey is the context key for router metadata
	RouterMetadataKey routerContextKey = "router_metadata"
)

// RouterMetadata contains routing decision information for analytics
type RouterMetadata struct {
	RouterSlug     string
	PoolName       string
	SourceModel    string // Original model from request
	TargetModel    string // Model after mapping
	SelectionAlgo  string // "round_robin" or "weighted"
	SelectedLLM    string // LLM slug that was selected
	SelectedWeight int    // Weight of selected vendor (for weighted algo)
}

// GetRouterMetadataFromContext extracts router metadata from request context
func GetRouterMetadataFromContext(ctx context.Context) *RouterMetadata {
	if meta, ok := ctx.Value(RouterMetadataKey).(*RouterMetadata); ok {
		return meta
	}
	return nil
}

// RouterMetadataStore provides a concurrent-safe store for router metadata
// keyed by request identifiers. This allows the analytics handler to retrieve
// router metadata even when it doesn't have access to the HTTP request context.
type RouterMetadataStore struct {
	store sync.Map
}

// Global router metadata store
var routerMetadataStore = &RouterMetadataStore{}

// GetRouterMetadataStore returns the global router metadata store
func GetRouterMetadataStore() *RouterMetadataStore {
	return routerMetadataStore
}

// StoreMetadata stores router metadata with a TTL for automatic cleanup
func (s *RouterMetadataStore) StoreMetadata(key string, meta *RouterMetadata) {
	s.store.Store(key, meta)
	// Auto-cleanup after 60 seconds to prevent memory leaks
	go func() {
		time.Sleep(60 * time.Second)
		s.store.Delete(key)
	}()
}

// GetMetadata retrieves and removes router metadata by key
func (s *RouterMetadataStore) GetMetadata(key string) *RouterMetadata {
	if val, ok := s.store.LoadAndDelete(key); ok {
		if meta, ok := val.(*RouterMetadata); ok {
			return meta
		}
	}
	return nil
}

// PeekMetadata retrieves router metadata without removing it
func (s *RouterMetadataStore) PeekMetadata(key string) *RouterMetadata {
	if val, ok := s.store.Load(key); ok {
		if meta, ok := val.(*RouterMetadata); ok {
			return meta
		}
	}
	return nil
}

var (
	// ErrRouterNotFound is returned when a router is not found
	ErrRouterNotFound = errors.New("model router not found")

	// ErrNoMatchingPool is returned when no pool matches the model
	ErrNoMatchingPool = errors.New("no pool matches the requested model")

	// ErrNoActiveVendors is returned when a pool has no active vendors
	ErrNoActiveVendors = errors.New("no active vendors in matching pool")
)

// ModelRouterService handles model routing logic
type ModelRouterService struct {
	db          *gorm.DB
	routers     map[string]*CompiledRouter // slug -> compiled router
	routerMutex sync.RWMutex
}

// CompiledRouter is a router with pre-compiled glob patterns for efficient matching
type CompiledRouter struct {
	Router        *database.ModelRouter
	CompiledPools []*CompiledPool
}

// CompiledPool is a pool with its pattern ready for matching
type CompiledPool struct {
	Pool    *database.ModelPool
	Pattern string // Pattern for path.Match
	Counter uint64 // Atomic counter for round-robin selection
}

// VendorSelection represents the result of vendor selection
type VendorSelection struct {
	Vendor      *database.PoolVendor
	Pool        *database.ModelPool
	TargetModel string // Model name after any mapping is applied
}

// NewModelRouterService creates a new model router service
func NewModelRouterService(db *gorm.DB) *ModelRouterService {
	svc := &ModelRouterService{
		db:      db,
		routers: make(map[string]*CompiledRouter),
	}
	// Initialize random seed for weighted selection
	rand.Seed(time.Now().UnixNano())
	return svc
}

// GetRouterCount returns the number of loaded routers
func (s *ModelRouterService) GetRouterCount() int {
	s.routerMutex.RLock()
	defer s.routerMutex.RUnlock()
	return len(s.routers)
}

// LoadRouters loads and compiles all active routers from the database
func (s *ModelRouterService) LoadRouters(namespace string) error {
	s.routerMutex.Lock()
	defer s.routerMutex.Unlock()

	log.Debug().Str("namespace", namespace).Msg("Loading model routers from database")

	var routers []database.ModelRouter
	query := s.db.Preload("Pools.Vendors.LLM").Preload("Pools.Mappings").
		Where("is_active = ?", true)

	if namespace != "" {
		// Include both namespace-specific and global (empty namespace) routers
		query = query.Where("(namespace = ? OR namespace = '')", namespace)
	}

	if err := query.Find(&routers).Error; err != nil {
		log.Error().Err(err).Str("namespace", namespace).Msg("Failed to query model routers")
		return err
	}

	log.Debug().Str("namespace", namespace).Int("found_count", len(routers)).Msg("Model routers found in database")

	// Clear existing routers
	s.routers = make(map[string]*CompiledRouter)

	// Compile each router
	for i := range routers {
		router := &routers[i]
		log.Debug().
			Str("slug", router.Slug).
			Str("name", router.Name).
			Bool("is_active", router.IsActive).
			Str("router_namespace", router.Namespace).
			Int("pool_count", len(router.Pools)).
			Msg("Processing router")

		compiled, err := s.compileRouter(router)
		if err != nil {
			log.Warn().Err(err).Str("router", router.Slug).Msg("Failed to compile router, skipping")
			continue
		}
		s.routers[router.Slug] = compiled
		log.Debug().Str("router", router.Slug).Int("pools", len(compiled.CompiledPools)).Msg("Loaded model router")
	}

	log.Info().Int("count", len(s.routers)).Str("namespace", namespace).Msg("Model routers loaded")
	return nil
}

// compileRouter compiles a router's pool patterns
func (s *ModelRouterService) compileRouter(router *database.ModelRouter) (*CompiledRouter, error) {
	compiled := &CompiledRouter{
		Router:        router,
		CompiledPools: make([]*CompiledPool, 0, len(router.Pools)),
	}

	// Sort pools by priority (descending)
	pools := make([]database.ModelPool, len(router.Pools))
	copy(pools, router.Pools)
	sort.Slice(pools, func(i, j int) bool {
		return pools[i].Priority > pools[j].Priority
	})

	for i := range pools {
		pool := &pools[i]
		// Validate that the pattern is valid for path.Match
		_, err := path.Match(pool.ModelPattern, "test")
		if err != nil {
			return nil, err
		}

		compiled.CompiledPools = append(compiled.CompiledPools, &CompiledPool{
			Pool:    pool,
			Pattern: pool.ModelPattern,
			Counter: 0,
		})
	}

	return compiled, nil
}

// GetRouter returns a compiled router by slug
func (s *ModelRouterService) GetRouter(slug string) (*CompiledRouter, bool) {
	s.routerMutex.RLock()
	defer s.routerMutex.RUnlock()
	router, ok := s.routers[slug]
	return router, ok
}

// SelectVendor selects a vendor for the given model in the router
func (s *ModelRouterService) SelectVendor(routerSlug string, modelName string) (*VendorSelection, error) {
	router, ok := s.GetRouter(routerSlug)
	if !ok {
		return nil, ErrRouterNotFound
	}

	// Find matching pool
	var matchedPool *CompiledPool
	for _, pool := range router.CompiledPools {
		matched, err := path.Match(pool.Pattern, modelName)
		if err != nil {
			log.Warn().Err(err).Str("pattern", pool.Pattern).Msg("Invalid pattern")
			continue
		}
		if matched {
			matchedPool = pool
			break
		}
	}

	if matchedPool == nil {
		return nil, ErrNoMatchingPool
	}

	// Get active vendors
	var activeVendors []*database.PoolVendor
	for i := range matchedPool.Pool.Vendors {
		vendor := &matchedPool.Pool.Vendors[i]
		if vendor.IsActive {
			activeVendors = append(activeVendors, vendor)
		}
	}

	if len(activeVendors) == 0 {
		return nil, ErrNoActiveVendors
	}

	// Select vendor based on algorithm
	var selectedVendor *database.PoolVendor
	switch matchedPool.Pool.SelectionAlgorithm {
	case "weighted":
		selectedVendor = s.selectWeightedVendor(activeVendors)
	default: // round_robin
		selectedVendor = s.selectRoundRobinVendor(matchedPool, activeVendors)
	}

	// Apply model mapping if present
	targetModel := modelName
	for _, mapping := range matchedPool.Pool.Mappings {
		if mapping.SourceModel == modelName {
			targetModel = mapping.TargetModel
			log.Debug().
				Str("source", modelName).
				Str("target", targetModel).
				Msg("Applied model mapping")
			break
		}
	}

	return &VendorSelection{
		Vendor:      selectedVendor,
		Pool:        matchedPool.Pool,
		TargetModel: targetModel,
	}, nil
}

// selectRoundRobinVendor selects a vendor using round-robin
func (s *ModelRouterService) selectRoundRobinVendor(pool *CompiledPool, vendors []*database.PoolVendor) *database.PoolVendor {
	index := atomic.AddUint64(&pool.Counter, 1) - 1
	return vendors[index%uint64(len(vendors))]
}

// selectWeightedVendor selects a vendor based on weights
func (s *ModelRouterService) selectWeightedVendor(vendors []*database.PoolVendor) *database.PoolVendor {
	// Calculate total weight
	var totalWeight int
	for _, v := range vendors {
		if v.Weight <= 0 {
			totalWeight += 1 // Default weight of 1
		} else {
			totalWeight += v.Weight
		}
	}

	// Generate random number
	r := rand.Intn(totalWeight)

	// Select vendor based on cumulative weight
	var cumulative int
	for _, v := range vendors {
		weight := v.Weight
		if weight <= 0 {
			weight = 1
		}
		cumulative += weight
		if r < cumulative {
			return v
		}
	}

	// Fallback (shouldn't happen)
	return vendors[0]
}

// RouterExists checks if a router exists by slug
func (s *ModelRouterService) RouterExists(slug string) bool {
	s.routerMutex.RLock()
	defer s.routerMutex.RUnlock()
	_, ok := s.routers[slug]
	return ok
}

// GetRouterSlugs returns all loaded router slugs
func (s *ModelRouterService) GetRouterSlugs() []string {
	s.routerMutex.RLock()
	defer s.routerMutex.RUnlock()
	slugs := make([]string, 0, len(s.routers))
	for slug := range s.routers {
		slugs = append(slugs, slug)
	}
	return slugs
}

// ModelRouterHandler is a Gin handler for model routing
type ModelRouterHandler struct {
	routerService *ModelRouterService
	chatHandler   http.HandlerFunc // The /ai/{routeId}/v1/chat/completions handler
}

// NewModelRouterHandler creates a new model router handler
func NewModelRouterHandler(routerService *ModelRouterService, chatHandler http.HandlerFunc) *ModelRouterHandler {
	return &ModelRouterHandler{
		routerService: routerService,
		chatHandler:   chatHandler,
	}
}

// ChatCompletionRequest is the minimal structure needed to extract the model
type ChatCompletionRequest struct {
	Model string `json:"model"`
}

// GinHandler returns a Gin handler function for model routing
func (h *ModelRouterHandler) GinHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract routerSlug from Gin path parameter (like other handlers in the codebase)
		routerSlug := c.Param("routerSlug")

		w := c.Writer
		r := c.Request

		// Read the full request body first (we need to preserve it for downstream)
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "Failed to read request body: "+err.Error())
			return
		}
		r.Body.Close()

		// Parse just the model field from the body
		var req ChatCompletionRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			respondWithError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
			return
		}

		// Select vendor based on model
		selection, err := h.routerService.SelectVendor(routerSlug, req.Model)
		if err != nil {
			switch err {
			case ErrRouterNotFound:
				respondWithError(w, http.StatusNotFound, "Router not found")
			case ErrNoMatchingPool:
				respondWithError(w, http.StatusBadRequest, "No pool matches model: "+req.Model)
			case ErrNoActiveVendors:
				respondWithError(w, http.StatusServiceUnavailable, "No active vendors available")
			default:
				respondWithError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}

		// Create router metadata for analytics
		routerMeta := &RouterMetadata{
			RouterSlug:     routerSlug,
			PoolName:       selection.Pool.Name,
			SourceModel:    req.Model, // Original model before any mapping
			TargetModel:    selection.TargetModel,
			SelectionAlgo:  selection.Pool.SelectionAlgorithm,
			SelectedLLM:    selection.Vendor.LLMSlug,
			SelectedWeight: selection.Vendor.Weight,
		}

		// Store router metadata keyed by app+timestamp pattern that analytics will use
		// This allows the analytics handler to retrieve router info when recording events
		// We store with multiple potential keys since timing can vary slightly
		timestamp := time.Now()
		metadataKey := fmt.Sprintf("router_%d_%d", 0, timestamp.Unix()) // AppID will be determined later
		GetRouterMetadataStore().StoreMetadata(metadataKey, routerMeta)

		// Log the routing decision
		log.Debug().
			Str("router", routerSlug).
			Str("model", req.Model).
			Str("target_model", selection.TargetModel).
			Str("llm_slug", selection.Vendor.LLMSlug).
			Str("pool", selection.Pool.Name).
			Str("algorithm", selection.Pool.SelectionAlgorithm).
			Str("metadata_key", metadataKey).
			Msg("Model router routing request")

		// Inject router metadata into request context for analytics
		ctx := context.WithValue(r.Context(), RouterMetadataKey, routerMeta)
		r = r.WithContext(ctx)

		// Inject the LLM slug as the routeId using mux.SetURLVars
		// This allows the downstream /ai/ handler to use the correct LLM
		r = mux.SetURLVars(r, map[string]string{
			"routeId":    selection.Vendor.LLMSlug,
			"routerSlug": routerSlug, // Preserve for analytics
		})

		// Rewrite the URL path from /router/{slug}/v1/... to /ai/{llm_slug}/v1/...
		// This is needed because the gateway handler expects /ai/{llm_slug}/... format
		originalPath := r.URL.Path
		routerPathRegex := regexp.MustCompile(`^/router/[^/]+/(.*)$`)
		if matches := routerPathRegex.FindStringSubmatch(originalPath); len(matches) >= 2 {
			newPath := "/ai/" + selection.Vendor.LLMSlug + "/" + matches[1]
			r.URL.Path = newPath
			log.Debug().
				Str("original_path", originalPath).
				Str("new_path", newPath).
				Msg("Rewrote URL path for gateway routing")
		}

		// Restore the request body - apply model mapping if needed
		finalBody := bodyBytes
		if selection.TargetModel != req.Model {
			// Need to update the model field in the body
			// Parse as generic map to preserve all fields
			var bodyMap map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &bodyMap); err == nil {
				bodyMap["model"] = selection.TargetModel
				if modifiedBody, err := json.Marshal(bodyMap); err == nil {
					finalBody = modifiedBody
					log.Debug().
						Str("original_model", req.Model).
						Str("target_model", selection.TargetModel).
						Msg("Applied model mapping to request body")
				}
			}
		}

		// Set the body for downstream handler
		r.Body = io.NopCloser(bytes.NewReader(finalBody))
		r.ContentLength = int64(len(finalBody))

		// Call the chat completion handler directly (no HTTP hop)
		h.chatHandler(w, r)
	}
}

// respondWithError writes a JSON error response
func respondWithError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    "model_router_error",
			"code":    statusCode,
		},
	})
}

// newReaderCloser creates a new ReadCloser from a struct
func newReaderCloser(r *http.Request, data interface{}) *bodyReader {
	body, _ := json.Marshal(data)
	return &bodyReader{data: body, offset: 0}
}

type bodyReader struct {
	data   []byte
	offset int
}

func (b *bodyReader) Read(p []byte) (n int, err error) {
	if b.offset >= len(b.data) {
		return 0, io.EOF
	}
	n = copy(p, b.data[b.offset:])
	b.offset += n
	return n, nil
}

func (b *bodyReader) Close() error {
	return nil
}

// internal/services/model_router_service.go
package services

import (
	"errors"
	"math/rand"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

var (
	// ErrRouterNotFound is returned when a router is not found
	ErrRouterNotFound = errors.New("model router not found")

	// ErrNoMatchingPool is returned when no pool matches the model
	ErrNoMatchingPool = errors.New("no pool matches the requested model")

	// ErrNoActiveVendors is returned when a pool has no active vendors
	ErrNoActiveVendors = errors.New("no active vendors in matching pool")

	// ErrNoPermittedVendors is returned when the matching pool has active
	// vendors, but none the caller may use.
	ErrNoPermittedVendors = errors.New("no vendor in the matching pool is available to this app")
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
	query := s.db.Preload("Pools.Vendors.LLM").Preload("Pools.Vendors.Mappings").
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
		// Validate that the pattern is valid (supports comma-separated patterns)
		_, err := matchModelPattern(pool.ModelPattern, "test")
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

// matchModelPattern checks if modelName matches any pattern in a comma-separated pattern string.
// This allows patterns like "gpt-*,claude-*" to match both GPT and Claude models.
func matchModelPattern(pattern, modelName string) (bool, error) {
	patterns := strings.Split(pattern, ",")
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		matched, err := path.Match(p, modelName)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
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
	return s.SelectVendorFor(routerSlug, modelName, nil)
}

// SelectVendorFor selects a vendor for the given model in the router among
// the vendors whose LLM allow accepts (nil accepts every vendor). Pools are
// matched on the model alone, first match wins, so a restriction never sends
// a model to a different pool than an unrestricted caller would get.
func (s *ModelRouterService) SelectVendorFor(routerSlug string, modelName string, allow func(llmID uint) bool) (*VendorSelection, error) {
	router, ok := s.GetRouter(routerSlug)
	if !ok {
		return nil, ErrRouterNotFound
	}

	// Find matching pool
	var matchedPool *CompiledPool
	for _, pool := range router.CompiledPools {
		matched, err := matchModelPattern(pool.Pattern, modelName)
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

	// Get active vendors, then the ones the caller may use
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

	if allow != nil {
		var permitted []*database.PoolVendor
		for _, v := range activeVendors {
			if allow(v.LLMID) {
				permitted = append(permitted, v)
			}
		}
		if len(permitted) == 0 {
			return nil, ErrNoPermittedVendors
		}
		activeVendors = permitted
	}

	// Select vendor based on algorithm
	var selectedVendor *database.PoolVendor
	switch matchedPool.Pool.SelectionAlgorithm {
	case "weighted":
		selectedVendor = s.selectWeightedVendor(activeVendors)
	default: // round_robin
		selectedVendor = s.selectRoundRobinVendor(matchedPool, activeVendors)
	}

	// Apply vendor-specific model mapping if present
	targetModel := modelName
	for _, mapping := range selectedVendor.Mappings {
		if mapping.SourceModel == modelName {
			targetModel = mapping.TargetModel
			log.Debug().
				Str("source", modelName).
				Str("target", targetModel).
				Str("vendor", selectedVendor.LLMSlug).
				Msg("Applied vendor-specific model mapping")
			break
		}
	}

	return &VendorSelection{
		Vendor:      selectedVendor,
		Pool:        matchedPool.Pool,
		TargetModel: targetModel,
	}, nil
}

// Reaches reports whether any active vendor of the router's pools is the LLM.
func (s *ModelRouterService) Reaches(routerSlug string, routerID uint, llmID uint) bool {
	router, ok := s.GetRouter(routerSlug)
	if !ok || router.Router.ID != routerID {
		return false
	}
	for _, pool := range router.CompiledPools {
		for _, v := range pool.Pool.Vendors {
			if v.IsActive && v.LLMID == llmID {
				return true
			}
		}
	}
	return false
}

// GetRouterByID returns a loaded router by id (a Semantic Router hands off to
// a Model Router by id).
func (s *ModelRouterService) GetRouterByID(id uint) (*CompiledRouter, bool) {
	s.routerMutex.RLock()
	defer s.routerMutex.RUnlock()
	for _, r := range s.routers {
		if r.Router.ID == id {
			return r, true
		}
	}
	return nil, false
}

// ReachesModel reports whether the router, asked for model, can pick the
// LLM: an active vendor of the pool that model matches.
func (s *ModelRouterService) ReachesModel(routerID uint, model string, llmID uint) bool {
	router, ok := s.GetRouterByID(routerID)
	if !ok {
		return false
	}
	for _, pool := range router.CompiledPools {
		matched, err := matchModelPattern(pool.Pattern, model)
		if err != nil || !matched {
			continue
		}
		for _, v := range pool.Pool.Vendors {
			if v.IsActive && v.LLMID == llmID {
				return true
			}
		}
		return false // the first matching pool is the one SelectVendorFor uses
	}
	return false
}

// AdvertisedModels lists the model names a router can be asked for by name:
// the literal (glob-free) entries of its pool patterns and the source models
// of its vendor mappings. Pure wildcard pools advertise nothing.
func (s *ModelRouterService) AdvertisedModels(routerSlug string) []string {
	router, ok := s.GetRouter(routerSlug)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(m string) {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] || strings.ContainsAny(m, "*?[") {
			return
		}
		seen[m] = true
		out = append(out, m)
	}
	for _, pool := range router.CompiledPools {
		for _, p := range strings.Split(pool.Pattern, ",") {
			add(p)
		}
		for _, v := range pool.Pool.Vendors {
			for _, m := range v.Mappings {
				add(m.SourceModel)
			}
		}
	}
	sort.Strings(out)
	return out
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

// ModelRouterHandler serves the legacy /router/{slug}/v1/... endpoints. They
// are an alias of the /ai/ chain: the path is rewritten to /ai/{slug}/v1/...
// and routing happens there, after authentication, exactly as it does for
// {"model": "{slug}/{model}"} on the unified ingress (see ModelRouterResolver).
type ModelRouterHandler struct {
	gatewayHandler http.HandlerFunc // the AI Gateway handler
}

// NewModelRouterHandler creates a new model router handler
func NewModelRouterHandler(gatewayHandler http.HandlerFunc) *ModelRouterHandler {
	return &ModelRouterHandler{gatewayHandler: gatewayHandler}
}

var routerPathRegex = regexp.MustCompile(`^/router/([^/]+)/(.*)$`)

// GinHandler returns a Gin handler function for model routing
func (h *ModelRouterHandler) GinHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		r := c.Request
		if m := routerPathRegex.FindStringSubmatch(r.URL.Path); len(m) == 3 {
			r.URL.Path = "/ai/" + m[1] + "/" + m[2]
			r.URL.RawPath = ""
		}
		h.gatewayHandler(c.Writer, r)
	}
}

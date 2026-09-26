// internal/services/hybrid_gateway_service.go
package services

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// TokenCacheEntry represents a cached token validation result
type TokenCacheEntry struct {
	Result    *TokenValidationResult
	CachedAt  time.Time
	ExpiresAt time.Time
	// refreshing is set once a request has started this entry's
	// refresh-ahead, so only one revalidation runs per entry.
	refreshing atomic.Bool
}

// refreshAheadFraction is how far through its TTL a cached validation must be
// before a request starts revalidating it in the background. The entry is
// served meanwhile and still expires at its TTL, so revocation takes effect no
// later than before; the difference is that a busy token no longer expires
// under load with every in-flight request missing at once.
const refreshAheadFraction = 0.8

// HybridGatewayService wraps DatabaseGatewayService and overrides token validation
// for on-demand validation while keeping all other operations local (database-backed)
type HybridGatewayService struct {
	*DatabaseGatewayService                                  // Embed DatabaseGatewayService for all other methods
	edgeClient     interface{}                               // Edge client for on-demand token validation
	edgeNamespace  string                                    // Edge namespace for token validation
	
	// Token validation cache
	tokenCache     map[string]*TokenCacheEntry              // token -> validation result
	cacheMutex     sync.RWMutex                            // Protects token cache
	cacheConfig    config.HubSpokeConfig                    // Cache configuration
	stopCleanup    chan bool                               // Signal to stop cleanup goroutine

	// validations makes concurrent validations of one token share a single
	// call to the control instance (and a single App upsert).
	validations singleflight.Group
	// warnings rate-limits the warnings for failed validations.
	warnings warnLimiter

	// apps caches GetAppByTokenID, which runs on every authenticated request
	// (the app with its LLMs, tools and datasources: five or more queries).
	// Invalidated by any configuration write; see database.GenCache.
	apps *database.GenCache[uint, *database.App]
}

// NewHybridGatewayService creates a hybrid gateway service for edge instances
func NewHybridGatewayService(db *gorm.DB, repo *database.Repository, edgeNamespace string, cacheConfig config.HubSpokeConfig) *HybridGatewayService {
	dbService := NewDatabaseGatewayService(db, repo)
	
	service := &HybridGatewayService{
		DatabaseGatewayService: dbService.(*DatabaseGatewayService),
		edgeNamespace:         edgeNamespace,
		tokenCache:           make(map[string]*TokenCacheEntry),
		cacheConfig:          cacheConfig,
		stopCleanup:          make(chan bool),
		apps:                 database.NewGenCache[uint, *database.App](),
	}
	if err := database.EnsureConfigGenerationCallbacks(db); err != nil {
		log.Warn().Err(err).Msg("Could not register config generation callbacks; app lookups fall back to a short TTL")
	}
	
	// Start cache cleanup goroutine if caching is enabled
	if cacheConfig.TokenCacheEnabled {
		go service.cacheCleanupWorker()
		log.Debug().
			Dur("ttl", cacheConfig.TokenCacheTTL).
			Int("max_size", cacheConfig.TokenCacheMaxSize).
			Dur("cleanup_interval", cacheConfig.TokenCacheCleanupInt).
			Msg("Token validation cache enabled for hybrid gateway service")
	}
	
	return service
}

// SetEdgeClient sets the edge client reference for on-demand token validation
func (h *HybridGatewayService) SetEdgeClient(edgeClient interface{}) {
	h.edgeClient = edgeClient
	log.Debug().Msg("Edge client set for hybrid gateway service on-demand token validation")
}

// ValidateAPIToken overrides DatabaseGatewayService to use cached on-demand validation
//
// A cached result is served until its TTL; from refreshAheadFraction of the
// TTL on, the first request to see it starts one background revalidation.
// Validations that do reach the control instance are shared per token: the
// requests that miss together wait for one call. Before this, a busy token's
// entry expired every TTL with every in-flight request missing at once, each
// calling the hub and rewriting the App in SQLite, which stalled the gateway
// for about a second every five minutes under load and, with a slow hub,
// refused valid requests.
func (h *HybridGatewayService) ValidateAPIToken(token string) (*TokenValidationResult, error) {
	if h.cacheConfig.TokenCacheEnabled {
		if cachedResult, refresh := h.cachedForRequest(token); cachedResult != nil {
			if refresh {
				go h.refreshAhead(token)
			}
			return cachedResult, nil
		}
	}

	v, err, _ := h.validations.Do(token, func() (interface{}, error) {
		return h.validateWithControl(token)
	})
	if err != nil {
		return nil, err
	}
	return v.(*TokenValidationResult), nil
}

// refreshAhead revalidates a token whose cached result is near its expiry.
// Success replaces the entry; a rejection removes it (validateWithControl);
// any other failure leaves it to expire at its TTL as before.
func (h *HybridGatewayService) refreshAhead(token string) {
	_, _, _ = h.validations.Do(token, func() (interface{}, error) {
		return h.validateWithControl(token)
	})
}

// validateWithControl validates a token with the control instance, caches the
// result and makes sure its App is in local SQLite.
func (h *HybridGatewayService) validateWithControl(token string) (*TokenValidationResult, error) {
	tokenPrefix := token
	if len(token) > 8 {
		tokenPrefix = token[:8]
	}

	log.Debug().Str("token_prefix", tokenPrefix).Msg("HybridGatewayService: using on-demand token validation")

	// A hub that cannot be reached is not a hub that said no. Within the
	// configured grace an entry that has only just expired is still served,
	// so a control-plane outage does not take every edge request down with
	// it. An explicit rejection below never comes through here.
	staleFallback := func(cause error) (*TokenValidationResult, error) {
		if stale, age := h.getStaleFromCache(token); stale != nil {
			log.Warn().
				Str("token_prefix", tokenPrefix).
				Dur("stale_for", age).
				Err(cause).
				Msg("Control instance unreachable - serving stale token validation result within grace")
			return stale, nil
		}
		return nil, cause
	}

	if h.edgeClient == nil {
		return staleFallback(fmt.Errorf("edge client not available for on-demand token validation"))
	}

	// Cast to edge client and make validation call
	if edgeClient, ok := h.edgeClient.(interface{ ValidateTokenOnDemand(string) (*pb.TokenValidationResponse, error) }); ok {
		resp, err := edgeClient.ValidateTokenOnDemand(token)
		if err != nil {
			if h.warnings.allow("control-call") {
				log.Warn().Err(err).Msg("Token validation: call to the control instance failed")
			}
			return staleFallback(fmt.Errorf("token validation failed: %w", err))
		}

		if !resp.Valid {
			if h.warnings.allow("rejected") {
				log.Warn().Str("reason", resp.ErrorMessage).Msg("Token validation: rejected by the control instance")
			}
			// The hub has spoken: whatever we cached for this token is no longer true.
			h.removeFromCache(token)
			return nil, fmt.Errorf("invalid token: %s", resp.ErrorMessage)
		}

		log.Debug().
			Str("token_prefix", tokenPrefix).
			Uint32("app_id", resp.AppId).
			Str("app_name", resp.AppName).
			Msg("On-demand token validation successful")

		// Store App if provided in response (pull-on-miss sync)
		if resp.App != nil {
			if err := h.storeAppFromPullOnMiss(resp.App); err != nil {
				log.Warn().Err(err).Uint32("app_id", resp.AppId).
					Msg("Failed to store App from pull-on-miss, will try local lookup")
			} else {
				log.Debug().
					Uint32("app_id", resp.AppId).
					Str("app_name", resp.App.Name).
					Int("llm_count", len(resp.App.LlmIds)).
					Msg("Stored App from pull-on-miss token validation")
			}
		}

		// Get the app from local SQLite database (should now exist after pull-on-miss sync)
		app, err := h.DatabaseGatewayService.GetAppByTokenID(uint(resp.AppId))
		if err != nil {
			// App not found in SQLite - try direct lookup with LLM preload
			log.Debug().Uint32("app_id", resp.AppId).Msg("App not found via GetAppByTokenID, trying direct lookup")

			var dbApp database.App
			if err := h.db.Where("id = ?", resp.AppId).Preload("LLMs").Preload("ModelRouters").Preload("SemanticRouters").First(&dbApp).Error; err != nil {
				if h.warnings.allow("app-missing") {
					log.Warn().Err(err).Uint32("app_id", resp.AppId).Msg("Token validation: the control instance accepted the token but its App is not in local SQLite")
				}
				return nil, fmt.Errorf("app %d not found in synced SQLite: %w", resp.AppId, err)
			}

			app = &dbApp
		}

		result := &TokenValidationResult{
			TokenID:   uint(resp.AppId), // Use app_id as pseudo token ID  
			TokenName: "on-demand-validated",
			AppID:     uint(resp.AppId),
			App:       app,
		}

		// Cache the result if caching is enabled
		if h.cacheConfig.TokenCacheEnabled {
			h.storeInCache(token, result)
			log.Debug().Str("token_prefix", tokenPrefix).Dur("ttl", h.cacheConfig.TokenCacheTTL).Msg("Token validation result cached")
		}

		return result, nil
	}

	return staleFallback(fmt.Errorf("edge client does not support token validation"))
}

// GetAppByTokenID overrides to handle pseudo token IDs from on-demand validation
func (h *HybridGatewayService) GetAppByTokenID(tokenID uint) (*database.App, error) {
	app, err := h.apps.Load(tokenID, func() (*database.App, error) { return h.loadAppByTokenID(tokenID) })
	if err != nil {
		return nil, err
	}
	// Each caller gets its own copy, so none can change the cached app.
	return database.DeepCopy(app), nil
}

func (h *HybridGatewayService) loadAppByTokenID(tokenID uint) (*database.App, error) {
	log.Debug().Uint("token_id", tokenID).Msg("HybridGatewayService.GetAppByTokenID called")

	// For on-demand validation, token_id equals app_id
	// Get the app directly from local SQLite (now has full relationships!)
	var app database.App
	if err := h.db.Where("id = ?", tokenID).Preload("LLMs").Preload("Tools.Filters").Preload("Datasources").Preload("ModelRouters").Preload("SemanticRouters").First(&app).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Debug().Uint("app_id", tokenID).Msg("App not found in local SQLite")
			return nil, fmt.Errorf("app not found: %d", tokenID)
		}
		return nil, fmt.Errorf("failed to get app from SQLite: %w", err)
	}

	log.Debug().
		Uint("token_id", tokenID).
		Uint("app_id", app.ID).
		Str("app_name", app.Name).
		Int("llm_count", len(app.LLMs)).
		Msg("Successfully found app with LLM relationships from synced SQLite")

	return &app, nil
}

// cachedForRequest returns the cached result for a token if it is still valid,
// and whether this request should start its refresh-ahead: the entry is past
// refreshAheadFraction of its TTL and no refresh has been started for it.
func (h *HybridGatewayService) cachedForRequest(token string) (*TokenValidationResult, bool) {
	h.cacheMutex.RLock()
	entry, exists := h.tokenCache[token]
	h.cacheMutex.RUnlock()
	if !exists {
		return nil, false
	}
	now := time.Now()
	if now.After(entry.ExpiresAt) {
		return nil, false
	}
	ttl := entry.ExpiresAt.Sub(entry.CachedAt)
	due := now.Sub(entry.CachedAt) >= time.Duration(float64(ttl)*refreshAheadFraction)
	return entry.Result, due && entry.refreshing.CompareAndSwap(false, true)
}

// warnLimiter lets a warning through at most once per warnInterval per key,
// so a failure that hits every request does not flood the log.
type warnLimiter struct {
	mu   sync.Mutex
	last map[string]time.Time
}

const warnInterval = 10 * time.Second

func (l *warnLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if l.last == nil {
		l.last = map[string]time.Time{}
	}
	if now.Sub(l.last[key]) < warnInterval {
		return false
	}
	l.last[key] = now
	return true
}

// getFromCache retrieves a token validation result from cache if valid
func (h *HybridGatewayService) getFromCache(token string) *TokenValidationResult {
	if !h.cacheConfig.TokenCacheEnabled {
		return nil
	}

	h.cacheMutex.RLock()
	defer h.cacheMutex.RUnlock()

	entry, exists := h.tokenCache[token]
	if !exists {
		return nil
	}

	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		return nil
	}

	return entry.Result
}

// getStaleFromCache returns an expired entry that is still within the stale
// grace, together with how long ago it expired. Only the hub-unreachable path
// asks for one; a fresh entry is served by getFromCache before we get here.
func (h *HybridGatewayService) getStaleFromCache(token string) (*TokenValidationResult, time.Duration) {
	if !h.cacheConfig.TokenCacheEnabled || h.cacheConfig.TokenCacheStaleGrace <= 0 {
		return nil, 0
	}

	h.cacheMutex.RLock()
	defer h.cacheMutex.RUnlock()

	entry, exists := h.tokenCache[token]
	if !exists {
		return nil, 0
	}

	now := time.Now()
	if now.After(entry.ExpiresAt.Add(h.cacheConfig.TokenCacheStaleGrace)) {
		return nil, 0
	}

	return entry.Result, now.Sub(entry.ExpiresAt)
}

// removeFromCache drops a token's cached result, fresh or stale.
func (h *HybridGatewayService) removeFromCache(token string) {
	if !h.cacheConfig.TokenCacheEnabled {
		return
	}

	h.cacheMutex.Lock()
	defer h.cacheMutex.Unlock()

	delete(h.tokenCache, token)
}

// retireAt is the moment an entry is of no further use: its expiry plus the
// stale grace during which it may still be served if the hub is unreachable.
func (h *HybridGatewayService) retireAt(entry *TokenCacheEntry) time.Time {
	if h.cacheConfig.TokenCacheStaleGrace <= 0 {
		return entry.ExpiresAt
	}
	return entry.ExpiresAt.Add(h.cacheConfig.TokenCacheStaleGrace)
}

// storeInCache stores a token validation result in cache
func (h *HybridGatewayService) storeInCache(token string, result *TokenValidationResult) {
	if !h.cacheConfig.TokenCacheEnabled {
		return
	}

	h.cacheMutex.Lock()
	defer h.cacheMutex.Unlock()

	// Check cache size limit and evict oldest entries if needed
	if len(h.tokenCache) >= h.cacheConfig.TokenCacheMaxSize {
		h.evictOldestEntry()
	}

	// Store new entry
	now := time.Now()
	h.tokenCache[token] = &TokenCacheEntry{
		Result:    result,
		CachedAt:  now,
		ExpiresAt: now.Add(h.cacheConfig.TokenCacheTTL),
	}
}

// evictOldestEntry removes the oldest cache entry (simple LRU). An entry that
// is past its stale grace is useless on every path and goes first.
func (h *HybridGatewayService) evictOldestEntry() {
	var oldestToken string
	var oldestTime time.Time

	now := time.Now()
	for token, entry := range h.tokenCache {
		if now.After(h.retireAt(entry)) {
			delete(h.tokenCache, token)
			return
		}
		if oldestToken == "" || entry.CachedAt.Before(oldestTime) {
			oldestToken = token
			oldestTime = entry.CachedAt
		}
	}

	if oldestToken != "" {
		delete(h.tokenCache, oldestToken)
	}
}

// cacheCleanupWorker periodically removes expired entries
func (h *HybridGatewayService) cacheCleanupWorker() {
	ticker := time.NewTicker(h.cacheConfig.TokenCacheCleanupInt)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.cleanupExpiredEntries()
		case <-h.stopCleanup:
			return
		}
	}
}

// cleanupExpiredEntries removes entries that are past expiry and past the
// stale grace, so they can no longer be served on any path.
func (h *HybridGatewayService) cleanupExpiredEntries() {
	h.cacheMutex.Lock()
	defer h.cacheMutex.Unlock()

	now := time.Now()
	expiredCount := 0

	for token, entry := range h.tokenCache {
		if now.After(h.retireAt(entry)) {
			delete(h.tokenCache, token)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		log.Debug().
			Int("expired_count", expiredCount).
			Int("cache_size", len(h.tokenCache)).
			Msg("Cleaned up expired token validation cache entries")
	}
}

// Stop stops the cache cleanup worker
func (h *HybridGatewayService) Stop() {
	if h.cacheConfig.TokenCacheEnabled {
		close(h.stopCleanup)
		log.Debug().Int("cache_size", len(h.tokenCache)).Msg("Stopped token validation cache cleanup worker")
	}
}

// storeAppFromPullOnMiss stores an App received during token validation (pull-on-miss sync).
// This ensures the App exists in local SQLite for subsequent LLM access checks.
// Uses upsert semantics - creates if not exists, updates if exists.
func (h *HybridGatewayService) storeAppFromPullOnMiss(pbApp *pb.AppConfig) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		// Build App record from proto
		app := &database.App{
			Model: gorm.Model{
				ID:        uint(pbApp.Id),
				CreatedAt: pbApp.CreatedAt.AsTime(),
				UpdatedAt: pbApp.UpdatedAt.AsTime(),
			},
			Name:          pbApp.Name,
			Description:   pbApp.Description,
			OwnerEmail:    pbApp.OwnerEmail,
			UserID:        uint(pbApp.UserId),
			IsActive:      pbApp.IsActive,
			MonthlyBudget: pbApp.MonthlyBudget,
			RateLimitRPM:  int(pbApp.RateLimitRpm),
			Namespace:     pbApp.Namespace,
		}

		// Handle budget start date if available
		if pbApp.BudgetStartDate != "" {
			if startDate, err := time.Parse(time.RFC3339, pbApp.BudgetStartDate); err == nil {
				app.BudgetStartDate = &startDate
			}
		}

		// Handle metadata JSON
		if pbApp.Metadata != "" {
			app.Metadata = datatypes.JSON(pbApp.Metadata)
		}

		// Upsert: Create if not exists, Update if exists
		// Use Clauses for proper upsert with all fields
		if err := tx.Where("id = ?", pbApp.Id).
			Assign(map[string]interface{}{
				"name":              app.Name,
				"description":       app.Description,
				"owner_email":       app.OwnerEmail,
				"user_id":           app.UserID,
				"is_active":         app.IsActive,
				"monthly_budget":    app.MonthlyBudget,
				"budget_start_date": app.BudgetStartDate,
				"rate_limit_rpm":    app.RateLimitRPM,
				"namespace":         app.Namespace,
				"metadata":          app.Metadata,
				"updated_at":        time.Now(),
			}).
			FirstOrCreate(app).Error; err != nil {
			return fmt.Errorf("failed to upsert app: %w", err)
		}

		// Clear existing app_llms for this app and recreate
		if err := tx.Exec("DELETE FROM app_llms WHERE app_id = ?", pbApp.Id).Error; err != nil {
			return fmt.Errorf("failed to clear app_llms: %w", err)
		}

		// Recreate app_llms join table entries
		for _, llmID := range pbApp.LlmIds {
			appLLM := &database.AppLLM{
				AppID:     uint(pbApp.Id),
				LLMID:     uint(llmID),
				IsActive:  true,
				CreatedAt: time.Now(),
			}
			if err := tx.Create(appLLM).Error; err != nil {
				return fmt.Errorf("failed to create app_llm (app=%d, llm=%d): %w", pbApp.Id, llmID, err)
			}
		}

		// Clear existing router grants for this app and recreate
		if err := tx.Exec("DELETE FROM app_model_routers WHERE app_id = ?", pbApp.Id).Error; err != nil {
			return fmt.Errorf("failed to clear app_model_routers: %w", err)
		}
		for _, routerID := range pbApp.ModelRouterIds {
			grant := &database.AppModelRouter{AppID: uint(pbApp.Id), ModelRouterID: uint(routerID), CreatedAt: time.Now()}
			if err := tx.Create(grant).Error; err != nil {
				return fmt.Errorf("failed to create app_model_router (app=%d, router=%d): %w", pbApp.Id, routerID, err)
			}
		}

		if err := tx.Exec("DELETE FROM app_semantic_routers WHERE app_id = ?", pbApp.Id).Error; err != nil {
			return fmt.Errorf("failed to clear app_semantic_routers: %w", err)
		}
		for _, routerID := range pbApp.SemanticRouterIds {
			grant := &database.AppSemanticRouter{AppID: uint(pbApp.Id), SemanticRouterID: uint(routerID), CreatedAt: time.Now()}
			if err := tx.Create(grant).Error; err != nil {
				return fmt.Errorf("failed to create app_semantic_router (app=%d, router=%d): %w", pbApp.Id, routerID, err)
			}
		}

		// Clear existing app_tools for this app and recreate
		if err := tx.Exec("DELETE FROM app_tools WHERE app_id = ?", pbApp.Id).Error; err != nil {
			log.Warn().Err(err).Uint32("app_id", pbApp.Id).Msg("Failed to clear app_tools (table may not exist)")
		}

		// Recreate app_tools join table entries
		for _, toolID := range pbApp.ToolIds {
			appTool := &database.AppTool{
				AppID:     uint(pbApp.Id),
				ToolID:    uint(toolID),
				CreatedAt: time.Now(),
			}
			if err := tx.Create(appTool).Error; err != nil {
				return fmt.Errorf("failed to create app_tool (app=%d, tool=%d): %w", pbApp.Id, toolID, err)
			}
		}

		// Clear existing app_datasources for this app and recreate
		if err := tx.Exec("DELETE FROM app_datasources WHERE app_id = ?", pbApp.Id).Error; err != nil {
			log.Warn().Err(err).Uint32("app_id", pbApp.Id).Msg("Failed to clear app_datasources (table may not exist)")
		}

		// Recreate app_datasources join table entries
		for _, dsID := range pbApp.DatasourceIds {
			appDS := &database.AppDatasource{
				AppID:        uint(pbApp.Id),
				DatasourceID: uint(dsID),
				CreatedAt:    time.Now(),
			}
			if err := tx.Create(appDS).Error; err != nil {
				return fmt.Errorf("failed to create app_datasource (app=%d, ds=%d): %w", pbApp.Id, dsID, err)
			}
		}

		// Note: Budget usage is NOT initialized here for pull-on-miss sync.
		// The edge gateway tracks budget locally via its own analytics.
		// Initializing it here with 0 would overwrite existing local budget data.

		log.Debug().
			Uint32("app_id", pbApp.Id).
			Str("app_name", pbApp.Name).
			Int("llm_count", len(pbApp.LlmIds)).
			Int("tool_count", len(pbApp.ToolIds)).
			Int("ds_count", len(pbApp.DatasourceIds)).
			Msg("Stored App from pull-on-miss token validation")

		return nil
	})
}
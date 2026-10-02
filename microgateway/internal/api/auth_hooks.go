package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/rs/zerolog/log"
)

// CreateAuthHooks creates authentication lifecycle hooks for microgateway plugins
func CreateAuthHooks(serviceContainer *services.ServiceContainer, pluginManager *plugins.PluginManager) *proxy.AuthHooks {
	llms := newHookLLMLookup(serviceContainer)
	return &proxy.AuthHooks{
		PreAuth:    createPreAuthHook(serviceContainer, pluginManager, llms),
		CustomAuth: createCustomAuthHook(serviceContainer, pluginManager, llms),
		PostAuth:   createPostAuthHook(serviceContainer, pluginManager, llms),
	}
}

// ===================================
// PRE-AUTH HOOK
// ===================================
// Executes BEFORE authentication, NO access to user/app data

func createPreAuthHook(serviceContainer *services.ServiceContainer, pluginManager *plugins.PluginManager, llms *hookLLMLookup) func(http.ResponseWriter, *http.Request) bool {
	return func(w http.ResponseWriter, r *http.Request) bool {
		// Only process LLM requests
		if !strings.HasPrefix(r.URL.Path, "/llm/") {
			return false
		}

		// Extract LLM slug: /llm/rest/{slug}/... or /llm/stream/{slug}/...
		llmSlug := extractLLMSlugFromPath(r.URL.Path)
		if llmSlug == "" {
			return false
		}

		// Get LLM by slug
		llmInterface, err := llms.get(llmSlug)
		if err != nil {
			return false // Let normal flow handle 404
		}

		var llmID uint
		var vendor string
		var dbLLM *database.LLM
		if llm, ok := llmInterface.(*database.LLM); ok {
			llmID = llm.ID
			vendor = llm.Vendor
			dbLLM = llm
		} else {
			return false
		}

		// Check for pre-auth plugins
		pluginList, err := pluginManager.GetPluginsForLLM(llmID, "pre_auth")
		if err != nil || isEmptySlice(pluginList) {
			return false // No plugins, continue
		}

		// Get canonical request ID from context (set by RequestIDMiddleware)
		requestID := ""
		if reqID := r.Context().Value("request_id"); reqID != nil {
			requestID = reqID.(string)
		}
		if requestID == "" {
			log.Error().Msg("Request ID not found in context - RequestIDMiddleware not configured")
			respondWithError(w, http.StatusInternalServerError, "Internal server error", nil)
			return true
		}

		// Create plugin context (NO app_id yet - not authenticated)
		// Include edge identity in metadata for plugin context
		metadata := make(map[string]interface{})
		if serviceContainer.EdgeID != "" {
			metadata["edge_id"] = serviceContainer.EdgeID
		}
		if serviceContainer.EdgeNamespace != "" {
			metadata["edge_namespace"] = serviceContainer.EdgeNamespace
		}
		database.AddGovernedMetadataToContext(metadata, dbLLM)

		pluginCtx := &interfaces.PluginContext{
			RequestID:    requestID, // Use canonical request ID from context
			LLMID:        llmID,
			LLMSlug:      llmSlug,
			Vendor:       vendor,
			AppID:        uint(0), // NOT authenticated yet
			UserID:       uint(0),
			Metadata:     metadata,
			TraceContext: make(map[string]string),
		}

		// Create plugin request
		headers := make(map[string]string)
		for key, values := range r.Header {
			if len(values) > 0 {
				headers[key] = values[0]
			}
		}

		bodyBytes, _ := readBodyWithoutConsuming(r)

		// Create plugin request matching interfaces.PluginRequest structure
		pluginReq := &interfaces.PluginRequest{
			Method:     r.Method,
			Path:       r.URL.Path,
			Headers:    headers,
			Body:       bodyBytes,
			RemoteAddr: r.RemoteAddr,
			Context:    pluginCtx,
		}

		// Execute pre-auth plugin chain
		result, err := pluginManager.ExecutePluginChain(llmID, "pre_auth", pluginReq, pluginCtx)
		if err != nil {
			log.Error().Err(err).Msg("Pre-auth plugin chain failed")
			respondWithError(w, http.StatusInternalServerError, "Plugin execution failed", nil)
			return true
		}

		// Check if plugin blocked the request
		if pluginResp, ok := result.(*interfaces.PluginResponse); ok {
			if pluginResp.Block {
				writeBlockedPluginResponse(w, pluginResp)
				return true // Block request
			}

			// Apply modifications if the plugin modified the request
			if pluginResp.Modified {
				// Apply header modifications
				for key, value := range pluginResp.Headers {
					r.Header.Set(key, value)
				}
				// Apply body modifications
				if len(pluginResp.Body) > 0 {
					r.Body = io.NopCloser(bytes.NewReader(pluginResp.Body))
					r.ContentLength = int64(len(pluginResp.Body))
				}
			}
		}

		return false // Continue to auth
	}
}

// ===================================
// CUSTOM AUTH HOOK (Auth Plugins)
// ===================================
// Authenticates requests to an endpoint that has auth plugins attached: an
// LLM's attached auth plugins, or the auth plugin list of a datasource, tool,
// router or custom-endpoint plugin. Credential extraction stays in the
// CredentialValidator; this decides only who the caller is.

// authPluginManager is what the auth hook needs from the plugin manager.
type authPluginManager interface {
	GetAuthPlugins(endpointType string, endpointID uint) (attached int, loaded []*plugins.LoadedPlugin, err error)
	CallAuth(lp *plugins.LoadedPlugin, req *interfaces.AuthRequest, pluginCtx *interfaces.PluginContext) (*interfaces.AuthResponse, error)
}

func createCustomAuthHook(serviceContainer *services.ServiceContainer, pluginManager *plugins.PluginManager, llms *hookLLMLookup) func(*http.Request, proxy.AuthTarget, string, string) (proxy.AuthResult, error) {
	return newPluginAuthenticator(serviceContainer, pluginManager, llms).authenticate
}

type pluginAuthenticator struct {
	edgeID        string
	edgeNamespace string
	plugins       authPluginManager
	llms          *hookLLMLookup
}

func newPluginAuthenticator(sc *services.ServiceContainer, pm authPluginManager, llms *hookLLMLookup) *pluginAuthenticator {
	a := &pluginAuthenticator{plugins: pm, llms: llms}
	if sc != nil {
		a.edgeID, a.edgeNamespace = sc.EdgeID, sc.EdgeNamespace
	}
	return a
}

// authenticate asks the endpoint's auth plugins, in order, until one
// authenticates the credential. An endpoint without auth plugins is not
// handled (app keys apply). A plugin that errors, refuses, or names no valid
// app is passed over; when none accepts, the request is rejected, and when
// none could even be asked, the error makes the gateway answer 503 rather
// than fall back to app keys.
func (a *pluginAuthenticator) authenticate(r *http.Request, target proxy.AuthTarget, credential, credType string) (proxy.AuthResult, error) {
	endpointType := target.Kind
	if endpointType == proxy.AuthTargetLLM {
		endpointType = plugins.AuthEndpointLLM
	}
	attached, loaded, err := a.plugins.GetAuthPlugins(endpointType, target.ID)
	if err != nil {
		return proxy.AuthResult{}, err
	}
	if attached == 0 {
		return proxy.AuthResult{Outcome: proxy.AuthNotHandled}, nil
	}
	if len(loaded) == 0 {
		return proxy.AuthResult{}, fmt.Errorf("none of the %d auth plugins on %s %q could be loaded", attached, target.Kind, target.Slug)
	}

	requestID, _ := r.Context().Value("request_id").(string)
	if requestID == "" {
		requestID = generateRequestID()
	}
	metadata := make(map[string]interface{})
	if a.edgeID != "" {
		metadata["edge_id"] = a.edgeID
	}
	if a.edgeNamespace != "" {
		metadata["edge_namespace"] = a.edgeNamespace
	}
	metadata["endpoint_type"] = target.Kind
	metadata["endpoint_slug"] = target.Slug
	pluginCtx := &interfaces.PluginContext{
		RequestID:    requestID,
		Metadata:     metadata,
		TraceContext: make(map[string]string),
	}
	if target.Kind == proxy.AuthTargetLLM {
		pluginCtx.LLMID = target.ID
		pluginCtx.LLMSlug = target.Slug
		if a.llms != nil {
			if v, err := a.llms.get(target.Slug); err == nil {
				if llm, ok := v.(*database.LLM); ok {
					database.AddGovernedMetadataToContext(metadata, llm)
				}
			}
		}
	}

	headers := make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	bodyBytes, _ := readBodyWithoutConsuming(r)
	authReq := &interfaces.AuthRequest{
		Credential: credential,
		AuthType:   credType,
		Request: &interfaces.PluginRequest{
			Method:     r.Method,
			Path:       r.URL.Path,
			Headers:    headers,
			Body:       bodyBytes,
			RemoteAddr: r.RemoteAddr,
			Context:    pluginCtx,
		},
	}

	answered := false
	reason := ""
	for _, lp := range loaded {
		resp, err := a.plugins.CallAuth(lp, authReq, pluginCtx)
		if err != nil {
			log.Warn().Err(err).Uint("plugin_id", lp.ID).Str("plugin_name", lp.Name).Msg("Auth plugin failed; trying the next")
			continue
		}
		answered = true
		if resp == nil || !resp.Authenticated {
			if resp != nil && resp.ErrorMessage != "" {
				reason = resp.ErrorMessage
			} else {
				reason = "rejected by auth plugin " + lp.Name
			}
			continue
		}
		// An authenticated answer must name an app. It used to fall back to
		// app 1 when it did not, letting any plugin bug act as that app.
		appID, err := strconv.ParseUint(resp.AppID, 10, 32)
		if err != nil || appID == 0 {
			log.Warn().Uint("plugin_id", lp.ID).Str("plugin_name", lp.Name).Str("app_id", resp.AppID).
				Msg("Auth plugin authenticated a request without a valid app id; treating it as a rejection")
			reason = "auth plugin " + lp.Name + " named no valid app"
			continue
		}
		return proxy.AuthResult{
			Outcome:    proxy.AuthAccepted,
			AppID:      uint(appID),
			Subject:    resp.UserID,
			Claims:     resp.Claims,
			PluginID:   lp.ID,
			PluginName: lp.Name,
		}, nil
	}
	if !answered {
		return proxy.AuthResult{}, fmt.Errorf("no auth plugin on %s %q answered", target.Kind, target.Slug)
	}
	return proxy.AuthResult{Outcome: proxy.AuthRejected, Reason: reason}, nil
}

// ===================================
// POST-AUTH HOOK
// ===================================
// Executes AFTER successful authentication, HAS access to authenticated user/app data

func createPostAuthHook(serviceContainer *services.ServiceContainer, pluginManager *plugins.PluginManager, llms *hookLLMLookup) func(http.ResponseWriter, *http.Request, uint) bool {
	return func(w http.ResponseWriter, r *http.Request, appID uint) bool {
		// Only for LLM requests
		if !strings.HasPrefix(r.URL.Path, "/llm/") {
			return false
		}

		llmSlug := extractLLMSlugFromPath(r.URL.Path)
		if llmSlug == "" {
			return false
		}

		llmInterface, err := llms.get(llmSlug)
		if err != nil {
			return false
		}

		var llmID uint
		var vendor string
		var dbLLM *database.LLM
		if llm, ok := llmInterface.(*database.LLM); ok {
			llmID = llm.ID
			vendor = llm.Vendor
			dbLLM = llm
		} else {
			return false
		}

		// Check for post-auth plugins
		pluginList, err := pluginManager.GetPluginsForLLM(llmID, "post_auth")
		if err != nil || isEmptySlice(pluginList) {
			return false // No plugins, continue
		}

		// Get canonical request ID from context (set by requestIDMiddleware)
		// This MUST exist - if it doesn't, the middleware chain is broken
		requestID := ""
		if reqID := r.Context().Value("request_id"); reqID != nil {
			requestID = reqID.(string)
		}
		if requestID == "" {
			// CRITICAL: Request ID middleware didn't run
			log.Error().Msg("Request ID not found in context - requestIDMiddleware not configured")
			respondWithError(w, http.StatusInternalServerError, "Internal server error", nil)
			return true
		}

		// Create plugin context (NOW with authenticated app_id)
		// Include edge identity in metadata for plugin context
		postAuthMetadata := make(map[string]interface{})
		if serviceContainer.EdgeID != "" {
			postAuthMetadata["edge_id"] = serviceContainer.EdgeID
		}
		if serviceContainer.EdgeNamespace != "" {
			postAuthMetadata["edge_namespace"] = serviceContainer.EdgeNamespace
		}
		database.AddGovernedMetadataToContext(postAuthMetadata, dbLLM)

		pluginCtx := &interfaces.PluginContext{
			RequestID:    requestID, // Use canonical request ID from context
			LLMID:        llmID,
			LLMSlug:      llmSlug,
			Vendor:       vendor,
			AppID:        appID, // AUTHENTICATED app_id available!
			UserID:       uint(0),
			Metadata:     postAuthMetadata,
			TraceContext: make(map[string]string),
		}

		// Create enriched request
		headers := make(map[string]string)
		for key, values := range r.Header {
			if len(values) > 0 {
				headers[key] = values[0]
			}
		}

		bodyBytes, _ := readBodyWithoutConsuming(r)

		// Who authenticated the request, and how (proxy.AuthIdentity). The
		// subject is set when an auth plugin said who the call is for.
		subject := ""
		authClaims := make(map[string]string)
		if ident := proxy.AuthIdentityFromContext(r.Context()); ident != nil {
			subject = ident.Subject
			for k, v := range ident.Claims {
				authClaims[k] = v
			}
			authClaims["auth_method"] = ident.Method
			if ident.PluginID != 0 {
				authClaims["auth_plugin_id"] = strconv.FormatUint(uint64(ident.PluginID), 10)
			}
		}

		// Create enriched request matching interfaces.EnrichedRequest structure
		enrichedReq := &interfaces.EnrichedRequest{
			PluginRequest: &interfaces.PluginRequest{
				Method:     r.Method,
				Path:       r.URL.Path,
				Headers:    headers,
				Body:       bodyBytes,
				RemoteAddr: r.RemoteAddr,
				Context:    pluginCtx,
			},
			UserID:        subject,
			AppID:         strconv.FormatUint(uint64(appID), 10), // String as per interface
			AuthClaims:    authClaims,
			Authenticated: true,
		}

		// Execute post-auth plugin chain
		result, err := pluginManager.ExecutePluginChain(llmID, "post_auth", enrichedReq, pluginCtx)
		if err != nil {
			log.Error().Err(err).Msg("Post-auth plugin chain failed")
			respondWithError(w, http.StatusInternalServerError, "Plugin execution failed", nil)
			return true
		}

		// Check if plugin blocked
		if pluginResp, ok := result.(*interfaces.PluginResponse); ok {
			if pluginResp.Block {
				writeBlockedPluginResponse(w, pluginResp)
				return true // Block request
			}

			// Apply modifications if the plugin modified the request
			if pluginResp.Modified {
				// Apply header modifications
				for key, value := range pluginResp.Headers {
					r.Header.Set(key, value)
				}
				// Apply body modifications
				if len(pluginResp.Body) > 0 {
					r.Body = io.NopCloser(bytes.NewReader(pluginResp.Body))
					r.ContentLength = int64(len(pluginResp.Body))
				}
			}

			// Apply context updates (e.g., upstream_override for DLB plugin)
			if len(pluginResp.ContextUpdates) > 0 {
				ctx := r.Context()
				for key, value := range pluginResp.ContextUpdates {
					ctx = context.WithValue(ctx, key, value)
					log.Debug().
						Str("key", key).
						Str("value", value).
						Msg("Applied plugin context update to request")
				}
				*r = *r.WithContext(ctx)
			}
		} else if modifiedEnrichedReq, ok := result.(*interfaces.EnrichedRequest); ok {
			// Handle EnrichedRequest from post-auth chain (new behavior for chained plugins)
			// Apply modifications from the plugin chain
			if modifiedEnrichedReq.PluginRequest != nil {
				// Apply header modifications
				for key, value := range modifiedEnrichedReq.PluginRequest.Headers {
					r.Header.Set(key, value)
				}
				// Apply body modifications
				if len(modifiedEnrichedReq.PluginRequest.Body) > 0 {
					r.Body = io.NopCloser(bytes.NewReader(modifiedEnrichedReq.PluginRequest.Body))
					r.ContentLength = int64(len(modifiedEnrichedReq.PluginRequest.Body))

					log.Debug().
						Int("modified_body_len", len(modifiedEnrichedReq.PluginRequest.Body)).
						Msg("✅ Applied post-auth plugin chain modifications to request")
				}
			}
		}

		// Apply context updates from plugin context metadata (populated by plugin chain)
		if pluginCtx != nil && pluginCtx.Metadata != nil {
			ctx := r.Context()
			for key, value := range pluginCtx.Metadata {
				if strVal, ok := value.(string); ok {
					ctx = context.WithValue(ctx, key, strVal)
					log.Debug().
						Str("key", key).
						Str("value", strVal).
						Msg("Applied plugin metadata to request context")
				}
			}
			*r = *r.WithContext(ctx)
		}

		return false // Continue to proxy
	}
}

// ===================================
// HELPER FUNCTIONS
// ===================================

// hookLLMLookup resolves the LLM that the three auth hooks look up on every
// /llm/ request. Each hook needs it only for the LLM's ID, vendor and
// governed metadata, and usually only to find that the LLM has no plugins,
// yet each lookup preloaded every app linked to the LLM: about a dozen
// queries per request between the three hooks. Entries are invalidated by
// any configuration write (database.GenCache).
//
// The cached LLM is stored without its Apps and Filters relations, which the
// hooks do not use, and every caller gets its own deep copy.
type hookLLMLookup struct {
	gateway services.GatewayServiceInterface
	cache   *database.GenCache[string, *database.LLM]
}

func newHookLLMLookup(sc *services.ServiceContainer) *hookLLMLookup {
	if err := database.EnsureConfigGenerationCallbacks(sc.DB); err != nil {
		log.Warn().Err(err).Msg("Could not register config generation callbacks; auth hook LLM lookups fall back to a short TTL")
	}
	return &hookLLMLookup{gateway: sc.GatewayService, cache: database.NewGenCache[string, *database.LLM]()}
}

// get returns the LLM as the hooks expect it from GetLLMBySlug: an
// interface{} holding a *database.LLM.
func (l *hookLLMLookup) get(slug string) (interface{}, error) {
	llm, err := l.cache.Load(slug, func() (*database.LLM, error) {
		v, err := l.gateway.GetLLMBySlug(slug)
		if err != nil {
			return nil, err
		}
		found, ok := v.(*database.LLM)
		if !ok {
			return nil, fmt.Errorf("unexpected LLM type %T for %q", v, slug)
		}
		trimmed := *found
		trimmed.Apps, trimmed.Filters = nil, nil
		return &trimmed, nil
	})
	if err != nil {
		return nil, err
	}
	return database.DeepCopy(llm), nil
}

func extractLLMSlugFromPath(path string) string {
	// /llm/{mode}/{slug}/...
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "llm" {
		return parts[2]
	}
	return ""
}

func isEmptySlice(v interface{}) bool {
	if v == nil {
		return true
	}
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice {
		return rv.Len() == 0
	}
	return false
}

func generateRequestID() string {
	return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

func readBodyWithoutConsuming(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return bodyBytes, nil
}

func respondWithError(w http.ResponseWriter, statusCode int, message string, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	resp := map[string]string{"error": message}
	if err != nil {
		resp["details"] = err.Error()
	}
	json.NewEncoder(w).Encode(resp)
}

// pluginFramingHeaders are headers net/http derives from the body it writes
// (or that only concern one connection). A plugin's value for them is not
// trusted: a plugin replaying a stored response, such as a cache, may carry
// another response's Content-Length, and a wrong one empties the body.
var pluginFramingHeaders = map[string]bool{
	"Content-Length":    true,
	"Transfer-Encoding": true,
	"Connection":        true,
	"Keep-Alive":        true,
	"Trailer":           true,
	"Upgrade":           true,
}

// writeBlockedPluginResponse writes the response a pre- or post-auth plugin
// answered a request with instead of letting it through.
func writeBlockedPluginResponse(w http.ResponseWriter, pluginResp *interfaces.PluginResponse) {
	statusCode := pluginResp.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusForbidden
	}

	// Set headers from plugin, except the framing net/http sets from the body
	for key, value := range pluginResp.Headers {
		if pluginFramingHeaders[http.CanonicalHeaderKey(key)] {
			continue
		}
		w.Header().Set(key, value)
	}

	w.WriteHeader(statusCode)

	if len(pluginResp.Body) > 0 {
		w.Write(pluginResp.Body)
	}
}

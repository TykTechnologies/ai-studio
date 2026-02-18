package proxy

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/rs/zerolog/log"
)

type CredentialExtractor func(r *http.Request) (string, error)

type PostAuthCallback func(w http.ResponseWriter, r *http.Request, appID uint) bool

type CredentialValidator struct {
	service    services.ServiceInterface
	p          *Proxy
	validators map[string]CredentialExtractor
	authHooks  *AuthHooks // Hooks for authentication lifecycle
}

func NewCredentialValidator(service services.ServiceInterface, proxy *Proxy) *CredentialValidator {
	return &CredentialValidator{
		service:    service,
		p:          proxy,
		validators: make(map[string]CredentialExtractor),
	}
}

// SetAuthHooks sets authentication lifecycle hooks
func (cv *CredentialValidator) SetAuthHooks(hooks *AuthHooks) {
	cv.authHooks = hooks
}

// SetPostAuthCallback is deprecated, use SetAuthHooks instead
// Kept for backward compatibility
func (cv *CredentialValidator) SetPostAuthCallback(callback PostAuthCallback) {
	cv.authHooks = &AuthHooks{
		PostAuth: callback,
	}
}

func (cv *CredentialValidator) RegisterValidator(vendor string, validator CredentialExtractor) {
	cv.validators[strings.ToLower(vendor)] = validator
}

func (cv *CredentialValidator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if authentication was already done by a microgateway plugin
		if pluginAuth := r.Context().Value("plugin_authenticated"); pluginAuth != nil {
			if authenticated, ok := pluginAuth.(bool); ok && authenticated {
				// Request already authenticated by microgateway plugin - skip credential validation
				// The plugin has already set the app context with correct AppID
				next.ServeHTTP(w, r)
				return
			}
		}

		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) < 2 { // Adjusted for paths like "/.well-known/..."
			// Allow .well-known paths without auth for now, or handle them separately if needed
			if pathParts[1] == ".well-known" {
				next.ServeHTTP(w, r)
				return
			}
			respondWithError(w, http.StatusBadRequest, "invalid request path", nil, false)
			return
		}

		// === HOOK POINT: PRE-AUTH ===
		// Execute pre-auth hooks BEFORE any authentication logic (for LLM proxy requests)
		if cv.authHooks != nil && cv.authHooks.PreAuth != nil {
			if blocked := cv.authHooks.PreAuth(w, r); blocked {
				return // Pre-auth hook blocked the request
			}
		}

		// --- Bearer Token Authentication (includes OAuth for MCP servers) ---
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")

			// First try OAuth access token lookup using interface method
			accessToken, err := cv.service.GetValidAccessTokenByToken(tokenString)
			if err == nil {
				// Valid OAuth access token
				user, err := cv.service.GetUserByID(accessToken.UserID)
				if err != nil {
					respondWithError(w, http.StatusInternalServerError, "Could not retrieve user for token", err, false)
					return
				}

				oauthClient, err := cv.service.GetOAuthClient(accessToken.ClientID)
				if err != nil {
					respondWithError(w, http.StatusInternalServerError, "Could not retrieve client for token", err, false)
					return
				}

				ctx := context.WithValue(r.Context(), "user", user)
				ctx = context.WithValue(ctx, "oauthClient", oauthClient)
				ctx = context.WithValue(ctx, "scope", accessToken.Scope)

				// OAuth flow does NOT trigger auth hooks - MCP server authentication path
				// This is separate from LLM proxy request authentication
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// OAuth token lookup failed - continue to check app secret or API key below
		}

		// If Bearer token but not OAuth, try app secret lookup
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")

			// Check if custom auth (auth plugin) should handle validation
			if cv.authHooks != nil && cv.authHooks.CustomAuth != nil {
				appID, authenticated, authErr := cv.authHooks.CustomAuth(tokenString, r)
				if authErr != nil {
					respondWithError(w, http.StatusInternalServerError, "Authentication error", authErr, false)
					return
				}

				if authenticated {
					// Auth plugin successfully validated
					app, err := cv.service.GetAppByID(appID)
					if err != nil {
						respondWithError(w, http.StatusInternalServerError, "Failed to retrieve app", err, false)
						return
					}

					ctx := context.WithValue(r.Context(), "app", app)
					// Update request with context BEFORE calling hook so hook modifications persist
					r = r.WithContext(ctx)

					// === HOOK POINT: POST-AUTH (Custom Auth Plugin) ===
					if cv.authHooks != nil && cv.authHooks.PostAuth != nil {
						if blocked := cv.authHooks.PostAuth(w, r, appID); blocked {
							return // Post-auth hook blocked the request
						}
					}

					next.ServeHTTP(w, r)
					return
				}
				// Auth plugin said not authenticated, fall through to standard validation
			}

			// Standard Bearer token validation (app secret)
			cred, err := cv.service.GetCredentialBySecret(tokenString)
			if err == nil && cred.Active {
				app, err := cv.service.GetAppByCredentialID(cred.ID)
				if err == nil {
					// Valid app secret - add app to context like API key flow
					ctx := context.WithValue(r.Context(), "app", app)

					// For tool requests, validate the app has access to the tool
					pathParts := strings.Split(r.URL.Path, "/")
					if len(pathParts) >= 3 && pathParts[1] == "tools" {
						toolSlug := pathParts[2]
						tool, err := cv.service.GetToolBySlug(toolSlug)
						if err != nil {
							// Return 401 for security - don't leak whether tool exists
							respondWithError(w, http.StatusUnauthorized, "invalid credential", nil, true)
							return
						}

						// Check if app has access to this tool
						hasAccess := false
						for _, t := range app.Tools {
							if t.ID == tool.ID {
								hasAccess = true
								break
							}
						}

						if !hasAccess {
							// Return 401 for security - don't leak tool access info
							respondWithError(w, http.StatusUnauthorized, "invalid credential", nil, true)
							return
						}

						ctx = context.WithValue(ctx, "tool", tool)
						ctx = context.WithValue(ctx, "toolSlug", toolSlug)

						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}

					// Not a tool request - continue with LLM request
					// Update request with context BEFORE calling hook so hook modifications persist
					r = r.WithContext(ctx)

					// === HOOK POINT: POST-AUTH (Bearer Token with App Secret) ===
					if cv.authHooks != nil && cv.authHooks.PostAuth != nil {
						log.Debug().Uint("app_id", app.ID).Msg("Bearer token auth: Calling post-auth hook")
						if blocked := cv.authHooks.PostAuth(w, r, app.ID); blocked {
							log.Debug().Msg("Bearer token auth: Post-auth hook blocked the request")
							return // Post-auth hook blocked the request
						}
						log.Debug().Msg("Bearer token auth: Post-auth hook completed")
					}

					next.ServeHTTP(w, r)
					return
				}
			}

			// Both OAuth token and app secret lookups failed
			respondWithError(w, http.StatusUnauthorized, "Invalid or expired bearer token", nil, true)
			return
		}

		// --- API Key Authentication (Fallback) ---
		// (Existing logic from original middleware)
		if len(pathParts) < 3 && pathParts[1] != ".well-known" {
			respondWithError(w, http.StatusBadRequest, "invalid request path for API key auth", nil, false) // false for wwwAuth
			return
		}

		var llmSlug, dsSlug, routeID, toolSlug string
		if len(pathParts) >= 2 {
			switch pathParts[1] {
			case "llm":
				if len(pathParts) > 3 {
					llmSlug = pathParts[3]
				} else if len(pathParts) > 2 {
					llmSlug = pathParts[2]
				}
			case "datasource":
				if len(pathParts) > 2 {
					dsSlug = pathParts[2]
				}
			case "ai":
				if len(pathParts) > 2 {
					routeID = pathParts[2]
				}
			case "tools":
				if len(pathParts) > 2 {
					toolSlug = pathParts[2]
				}
			case ".well-known":
				next.ServeHTTP(w, r)
				return
			default:
				respondWithError(w, http.StatusBadRequest, "invalid request path", nil, false) // false for wwwAuth
				return
			}
		}

		if llmSlug == "" && dsSlug == "" && routeID == "" && toolSlug == "" {
			respondWithError(w, http.StatusUnauthorized, "Missing or invalid authentication method.", nil, true) // true for wwwAuth
			return
		}

		var apiKey string
		var err error // Keep original err for extractor

		if dsSlug != "" {
			apiKey = r.Header.Get("Authorization")
			if apiKey == "" {
				respondWithError(w, http.StatusUnauthorized, "Missing Authorization header for datasource", nil, true) // true for wwwAuth
				return
			}
		} else if llmSlug != "" {
			llm, ok := cv.p.GetLLM(llmSlug)
			if !ok {
				respondWithError(w, http.StatusNotFound, "[cred validator] LLM not found "+llmSlug, nil, false) // false for wwwAuth
				return
			}
			if !strings.HasPrefix(authHeader, "Bearer ") {
				extractor, ok := cv.validators[strings.ToLower(string(llm.Vendor))]
				if !ok {
					respondWithError(w, http.StatusBadRequest, "no validator for this llm vendor", nil, false) // false for wwwAuth
					return
				}
				apiKey, err = extractor(r)
				if err != nil {
					respondWithError(w, http.StatusUnauthorized, "invalid API key for llm pass through", err, true) // true for wwwAuth
					return
				}
			}
		} else if toolSlug != "" {
			apiKey = r.Header.Get("Authorization")
			if apiKey != "" {
				// No Bearer prefix for tool API keys typically
			} else {
				apiKey = r.URL.Query().Get("apiKey")
				if apiKey == "" {
					respondWithError(w, http.StatusUnauthorized, "missing Authorization header or apiKey query parameter for tool request", nil, true) // true for wwwAuth
					return
				}
			}
		} else if routeID != "" {
			if !strings.HasPrefix(authHeader, "Bearer ") && authHeader != "" {
				apiKey = authHeader
			} else if authHeader == "" {
				respondWithError(w, http.StatusUnauthorized, "missing Authorization header for 'ai' route", nil, true) // true for wwwAuth
				return
			}
		}

		if apiKey == "" {
			// This case is hit if it was a Bearer token attempt for an LLM, but it wasn't an OAuth token.
			// Or if other paths somehow didn't extract an apiKey.
			respondWithError(w, http.StatusUnauthorized, "Missing or invalid API key.", nil, true) // true for wwwAuth
			return
		}

		if toolSlug != "" {
			ctx := context.WithValue(r.Context(), "toolSlug", toolSlug)
			r = r.WithContext(ctx)
		}

		// === TRY CUSTOM AUTH (Auth Plugin) for API Key ===
		if cv.authHooks != nil && cv.authHooks.CustomAuth != nil {
			appID, authenticated, authErr := cv.authHooks.CustomAuth(apiKey, r)
			if authErr != nil {
				// Auth plugin error
				respondWithError(w, http.StatusUnauthorized, "Authentication failed", authErr, true)
				return
			}

			if authenticated {
				// Auth plugin successfully validated
				app, err := cv.service.GetAppByID(appID)
				if err != nil {
					respondWithError(w, http.StatusInternalServerError, "Failed to retrieve app", err, false)
					return
				}

				ctx := r.Context()
				ctx = context.WithValue(ctx, "app", app)
				r = r.WithContext(ctx)

				// === HOOK POINT: POST-AUTH (Custom Auth Plugin via API Key) ===
				if cv.authHooks != nil && cv.authHooks.PostAuth != nil {
					if blocked := cv.authHooks.PostAuth(w, r, appID); blocked {
						return // Post-auth hook blocked the request
					}
				}

				next.ServeHTTP(w, r)
				return
			}
			// Auth plugin returned false - this means authentication failed (no fallback)
			// The error should have been returned above
		}

		// === STANDARD API KEY VALIDATION (only if no auth plugin) ===
		validAPIKey, reqWithCtx := cv.CheckAPICredential(apiKey, dsSlug, llmSlug, routeID, toolSlug, r)
		if !validAPIKey {
			respondWithError(w, http.StatusUnauthorized, "Invalid API key or insufficient permissions.", nil, true) // true for wwwAuth
			return
		}
		r = reqWithCtx

		// === HOOK POINT: POST-AUTH (API Key Authentication) ===
		if cv.authHooks != nil && cv.authHooks.PostAuth != nil {
			if app := r.Context().Value("app"); app != nil {
				// Use interface to avoid circular import with models package
				if appWithID, ok := app.(interface{ GetID() uint }); ok {
					appID := appWithID.GetID()
					log.Debug().Uint("app_id", appID).Msg("Calling post-auth hook with authenticated app_id")
					if blocked := cv.authHooks.PostAuth(w, r, appID); blocked {
						log.Debug().Msg("Post-auth hook blocked the request")
						return // Post-auth hook blocked the request
					}
					log.Debug().Msg("Post-auth hook completed successfully")
				} else {
					log.Warn().Msg("App in context does not implement GetID() method - cannot extract app_id")
				}
			} else {
				log.Debug().Msg("No app in context for post-auth hook")
			}
		} else {
			log.Debug().Msg("No post-auth hook registered")
		}

		next.ServeHTTP(w, r)
	})
}

// Renamed from CheckCredential to CheckAPICredential to differentiate
func (cv *CredentialValidator) CheckAPICredential(apiKey, dsSlug, llmSlug, routeID, toolSlug string, r *http.Request) (bool, *http.Request) {
	cred, err := cv.service.GetCredentialBySecret(apiKey) // API Key is the 'secret'
	if err != nil {
		log.Debug().Err(err).Str("api_key_prefix", apiKey[:min(len(apiKey), 8)]).Msg("CheckAPICredential: GetCredentialBySecret failed")
		return false, r
	}
	if !cred.Active {
		log.Debug().Uint("cred_id", cred.ID).Msg("CheckAPICredential: Credential is inactive")
		// Log inactive credential usage for compliance tracking
		if app, appErr := cv.service.GetAppByCredentialID(cred.ID); appErr == nil {
			analytics.RecordProxyLog(&models.ProxyLog{
				AppID:        app.ID,
				UserID:       app.UserID,
				ResponseCode: http.StatusUnauthorized,
				TimeStamp:    time.Now(),
				Vendor:       "auth",
				ResponseBody: `{"error":"credential_inactive","detail":"API credential is inactive"}`,
			})
		}
		return false, r
	}

	log.Debug().
		Uint("cred_id", cred.ID).
		Int("cred_id_signed", int(cred.ID)).
		Str("key_id", cred.KeyID).
		Msg("CheckAPICredential: Credential found and active")

	app, err := cv.service.GetAppByCredentialID(cred.ID)
	if err != nil {
		log.Debug().Err(err).Uint("cred_id", cred.ID).Int("cred_id_signed", int(cred.ID)).Msg("CheckAPICredential: GetAppByCredentialID failed")
		return false, r
	}

	log.Debug().
		Uint("app_id", app.ID).
		Str("app_name", app.Name).
		Int("llm_count", len(app.LLMs)).
		Msg("CheckAPICredential: Retrieved app for credential")

	ctx := context.WithValue(r.Context(), "app", app)
	// Note: toolSlug might be already in r.Context() if set before calling this func
	// but setting it again here from param ensures it's the one CheckAPICredential is using.
	if toolSlug != "" {
		ctx = context.WithValue(ctx, "toolSlug", toolSlug)
	}
	r = r.WithContext(ctx)

	if dsSlug != "" {
		ds, ok := cv.p.GetDatasource(dsSlug)
		if !ok {
			return false, r
		}
		for _, d := range app.Datasources {
			if d.ID == ds.ID {
				return true, r
			}
		}
		return false, r
	}

	if llmSlug != "" {
		llm, ok := cv.p.GetLLM(llmSlug)
		if !ok {
			log.Debug().Str("llm_slug", llmSlug).Msg("CheckAPICredential: LLM not found in proxy cache")
			return false, r
		}
		log.Debug().
			Uint("llm_id", llm.ID).
			Str("llm_slug", llmSlug).
			Uint("app_id", app.ID).
			Int("app_llm_count", len(app.LLMs)).
			Msg("CheckAPICredential: Checking if app has access to LLM")

		for i, l := range app.LLMs {
			log.Debug().Int("index", i).Uint("app_llm_id", l.ID).Uint("required_llm_id", llm.ID).Msg("CheckAPICredential: Comparing LLM IDs")
			if l.ID == llm.ID {
				log.Debug().Msg("CheckAPICredential: App has access to LLM - validation PASSED")
				return true, r
			}
		}
		log.Debug().
			Uint("app_id", app.ID).
			Uint("required_llm_id", llm.ID).
			Str("llm_slug", llmSlug).
			Int("app_llms_count", len(app.LLMs)).
			Msg("CheckAPICredential: App does not have access to this LLM - validation FAILED")
		return false, r
	}

	if routeID != "" { // This was for /ai/{routeID}, assuming routeID is an LLM slug
		px, ok := cv.p.GetLLM(routeID)
		if !ok {
			return false, r
		}
		for _, llm := range app.LLMs {
			if llm.ID == px.ID {
				return true, r
			}
		}
		return false, r
	}

	if toolSlugContext := r.Context().Value("toolSlug"); toolSlugContext != nil {
		if ts, ok := toolSlugContext.(string); ok && ts != "" {
			tool, err := cv.service.GetToolBySlug(ts)
			if err != nil {
				return false, r
			}
			for _, t := range app.Tools {
				if t.ID == tool.ID {
					ctx := context.WithValue(r.Context(), "tool", tool) // Add full tool to context
					return true, r.WithContext(ctx)
				}
			}
			return false, r
		}
	}

	return false, r // Default to no access if no specific resource type matches
}

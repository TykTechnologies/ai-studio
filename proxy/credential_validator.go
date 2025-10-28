package proxy

import (
	"context"
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/rs/zerolog/log"
)

type CredentialExtractor func(r *http.Request) (string, error)

type CredentialValidator struct {
	service    services.ServiceInterface
	p          *Proxy
	validators map[string]CredentialExtractor
}

func NewCredentialValidator(service services.ServiceInterface, proxy *Proxy) *CredentialValidator {
	return &CredentialValidator{
		service:    service,
		p:          proxy,
		validators: make(map[string]CredentialExtractor),
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

		// --- Bearer Token Authentication ---
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

				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// If OAuth token lookup failed, try app secret lookup (for MCP OAuth)
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
					next.ServeHTTP(w, r.WithContext(ctx))
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

		validAPIKey, reqWithCtx := cv.CheckAPICredential(apiKey, dsSlug, llmSlug, routeID, toolSlug, r)
		if !validAPIKey {
			respondWithError(w, http.StatusUnauthorized, "Invalid API key or insufficient permissions.", nil, true) // true for wwwAuth
			return
		}
		r = reqWithCtx

		next.ServeHTTP(w, r)
	})
}

// Renamed from CheckCredential to CheckAPICredential to differentiate
func (cv *CredentialValidator) CheckAPICredential(apiKey, dsSlug, llmSlug, routeID, toolSlug string, r *http.Request) (bool, *http.Request) {
	cred, err := cv.service.GetCredentialBySecret(apiKey) // API Key is the 'secret'
	if err != nil {
		log.Info().Err(err).Str("api_key_prefix", apiKey[:min(len(apiKey), 8)]).Msg("CheckAPICredential: GetCredentialBySecret failed")
		return false, r
	}
	if !cred.Active {
		log.Info().Uint("cred_id", cred.ID).Msg("CheckAPICredential: Credential is inactive")
		return false, r
	}

	log.Info().
		Uint("cred_id", cred.ID).
		Int("cred_id_signed", int(cred.ID)).
		Str("key_id", cred.KeyID).
		Msg("CheckAPICredential: Credential found and active")

	app, err := cv.service.GetAppByCredentialID(cred.ID)
	if err != nil {
		log.Info().Err(err).Uint("cred_id", cred.ID).Int("cred_id_signed", int(cred.ID)).Msg("CheckAPICredential: GetAppByCredentialID failed")
		return false, r
	}

	log.Info().
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
			log.Info().Str("llm_slug", llmSlug).Msg("CheckAPICredential: LLM not found in proxy cache")
			return false, r
		}
		log.Info().
			Uint("llm_id", llm.ID).
			Str("llm_slug", llmSlug).
			Uint("app_id", app.ID).
			Int("app_llm_count", len(app.LLMs)).
			Msg("CheckAPICredential: Checking if app has access to LLM")

		for i, l := range app.LLMs {
			log.Info().Int("index", i).Uint("app_llm_id", l.ID).Uint("required_llm_id", llm.ID).Msg("CheckAPICredential: Comparing LLM IDs")
			if l.ID == llm.ID {
				log.Info().Msg("CheckAPICredential: App has access to LLM - validation PASSED")
				return true, r
			}
		}
		log.Info().
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

// func (cv *CredentialValidator) Middleware(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		vars := mux.Vars(r)
// 		llmSlug := vars["llmSlug"]
// 		dsSlug := vars["dsSlug"]

// 		if llmSlug == "" && dsSlug == "" {
// 			respondWithError(w, http.StatusBadRequest, "no LLM or datasource specified", nil)
// 			return
// 		}

// 		// it's a DS query
// 		if dsSlug != "" {
// 			ds, ok := cv.p.GetDatasource(dsSlug)

// 			if !ok {
// 				respondWithError(w, http.StatusNotFound, "datasource not found", nil)
// 				return
// 			}

// 			token := r.Header.Get("Authorization")
// 			if token == "" {
// 				respondWithError(w, http.StatusUnauthorized, "missing authorization header", nil)
// 				return
// 			}

// 			allow := cv.CheckCredential(token, ds, nil)
// 			if !allow {
// 				respondWithError(w, http.StatusUnauthorized, "invalid credential", nil)
// 				return
// 			}
// 		}

// 		// it's an LLM query
// 		if llmSlug != "" {
// 			llm, ok := cv.p.GetLLM(llmSlug)

// 			if !ok {
// 				respondWithError(w, http.StatusNotFound, "LLM not found", nil)
// 				return
// 			}

// 			extractor, ok := cv.validators[string(llm.Vendor)]
// 			if !ok {
// 				respondWithError(w, http.StatusUnauthorized, "no validator for this vendor", nil)
// 			}

// 			token, err := extractor(r)
// 			if err != nil {
// 				respondWithError(w, http.StatusUnauthorized, "invalid credential", nil)
// 			}

// 			allow := cv.CheckCredential(token, nil, llm)
// 			if !allow {
// 				respondWithError(w, http.StatusUnauthorized, "invalid credential", nil)
// 				return
// 			}

// 		}

// 		next.ServeHTTP(w, r)
// 	})
// }

// func (cv *CredentialValidator) CheckCredential(token string, ds *models.Datasource, llm *models.LLM) bool {
// 	cred, err := cv.service.GetCredentialBySecret(token)
// 	if err != nil {
// 		return false
// 	}

// 	if !cred.Active {
// 		return false
// 	}

// 	app, err := cv.service.GetAppByCredentialID(cred.ID)
// 	if err != nil {
// 		return false
// 	}

// 	if ds != nil {
// 		for _, d := range app.Datasources {
// 			if d.ID == ds.ID {
// 				return true
// 			}
// 		}

// 		return false
// 	}

// 	if llm != nil {
// 		for _, l := range app.LLMs {
// 			if l.ID == llm.ID {
// 				return true
// 			}
// 		}

// 		return false
// 	}

// 	return false
// }

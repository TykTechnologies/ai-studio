package proxy

import (
	"context"
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/services"
)

type CredentialExtractor func(r *http.Request) (string, error)

type CredentialValidator struct {
	service    *services.Service // Changed to concrete type
	p          *Proxy
	validators map[string]CredentialExtractor
	// No need for explicit accessTokenService, use cv.service.AccessTokenService
}

func NewCredentialValidator(service *services.Service, proxy *Proxy) *CredentialValidator { // Changed to concrete type
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

			accessTokenService := services.NewAccessTokenService(cv.service.GetDB())
			accessToken, err := accessTokenService.GetValidAccessTokenByToken(tokenString)
			if err != nil {
				respondWithError(w, http.StatusUnauthorized, "Invalid or expired access token", err, true)
				return
			}

			// Use GetUserByID directly from the main service if available on the interface,
			// or ensure UserService is correctly instantiated if it's a separate component.
			// Based on services/user_service.go, GetUserByID is a method on *Service.
			// cv.service is ServiceInterface. ServiceInterface has GetUserByID.
			user, err := cv.service.GetUserByID(accessToken.UserID)
			if err != nil {
				respondWithError(w, http.StatusInternalServerError, "Could not retrieve user for token", err, false)
				return
			}

			oauthClientService := services.NewOAuthClientService(cv.service.GetDB())
			oauthClient, err := oauthClientService.GetClient(accessToken.ClientID)
			if err != nil {
				respondWithError(w, http.StatusInternalServerError, "Could not retrieve client for token", err, false)
				return
			}

			// At this point, user is authenticated via Bearer token.
			// We need to decide what to put in context.
			// The existing API key flow puts an *models.App in context.
			// For OAuth, we have a *models.User and *models.OAuthClient.
			// For simplicity, we can place the User in context.
			// If proxy logic specifically needs an *App-like structure, we might need an adapter
			// or modify proxy logic to handle *User or *OAuthClient.

			// For now, let's add user and oauthClient to context.
			// Proxy handlers might need adjustment if they expect *models.App.
			ctx := context.WithValue(r.Context(), "user", user)
			ctx = context.WithValue(ctx, "oauthClient", oauthClient)
			// Add scope to context if needed: ctx = context.WithValue(ctx, "scope", accessToken.Scope)

			// TODO: Check if the user/client has access to the specific resource (llmSlug, toolSlug, etc.)
			// This part is analogous to CheckCredential's permission checking for API keys.
			// For now, if token is valid, we allow access. Granular resource access control is a further step.
			// This might involve checking user's group permissions or if the OAuthClient is tied to specific resources.
			// For now, we'll assume valid token = access to requested resource if path matches a known type.

			next.ServeHTTP(w, r.WithContext(ctx))
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
				if len(pathParts) > 2 { dsSlug = pathParts[2] }
			case "ai":
				if len(pathParts) > 2 { routeID = pathParts[2] }
			case "tools":
				if len(pathParts) > 2 { toolSlug = pathParts[2] }
			case ".well-known":
				next.ServeHTTP(w,r)
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
	if err != nil || !cred.Active {
		return false, r
	}

	app, err := cv.service.GetAppByCredentialID(cred.ID)
	if err != nil {
		return false, r
	}

	ctx := context.WithValue(r.Context(), "app", app)
	// Note: toolSlug might be already in r.Context() if set before calling this func
	// but setting it again here from param ensures it's the one CheckAPICredential is using.
	if toolSlug != "" {
		ctx = context.WithValue(ctx, "toolSlug", toolSlug)
	}
	r = r.WithContext(ctx)


	if dsSlug != "" {
		ds, ok := cv.p.GetDatasource(dsSlug)
		if !ok { return false, r }
		for _, d := range app.Datasources {
			if d.ID == ds.ID { return true, r }
		}
		return false, r
	}

	if llmSlug != "" {
		llm, ok := cv.p.GetLLM(llmSlug)
		if !ok { return false, r }
		for _, l := range app.LLMs {
			if l.ID == llm.ID { return true, r }
		}
		return false, r
	}

	if routeID != "" { // This was for /ai/{routeID}, assuming routeID is an LLM slug
		px, ok := cv.p.GetLLM(routeID)
		if !ok { return false, r }
		for _, llm := range app.LLMs {
			if llm.ID == px.ID { return true, r }
		}
		return false, r
	}

	if toolSlugContext := r.Context().Value("toolSlug"); toolSlugContext != nil {
		if ts, ok := toolSlugContext.(string); ok && ts != "" {
			tool, err := cv.service.GetToolBySlug(ts)
			if err != nil { return false, r }
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

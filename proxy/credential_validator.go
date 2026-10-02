package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/oauthscope"
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

// toolSlugFromPath returns the slug named by a /tools/{slug}/... path, or "" when
// the path is not a tool path. It covers the REST endpoint and all three MCP
// transports (/mcp, /mcp/sse, /mcp/message) alike.
//
// The middleware wraps the router, so mux route vars are not populated yet and
// the path has to be split by hand. fixDoubleSlash runs ahead of auth, so
// "//tools/x" cannot dodge this.
func toolSlugFromPath(path string) string {
	pathParts := strings.Split(path, "/")
	if len(pathParts) >= 3 && pathParts[1] == "tools" {
		return pathParts[2]
	}
	return ""
}

// appHasTool reports whether the app is entitled to the tool.
func appHasTool(app *models.App, toolID uint) bool {
	if app == nil {
		return false
	}
	for _, t := range app.Tools {
		if t.ID == toolID {
			return true
		}
	}
	return false
}

// authorizeToolAccess resolves the tool named by a tool path and checks the app
// is entitled to it, returning a context carrying the tool for the handlers.
//
// This is the single implementation of the tool ACL. Every authentication branch
// calls it, because the branches had drifted: the OAuth branch performed no check
// at all, which let any valid token reach any tool's MCP server.
//
// Failures answer 401 "invalid credential" whether the tool is missing or merely
// not granted - the caller must not learn which.
func (cv *CredentialValidator) authorizeToolAccess(w http.ResponseWriter, r *http.Request, app *models.App, toolSlug string) (context.Context, bool) {
	tool, err := cv.service.GetToolBySlug(toolSlug)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "invalid credential", nil, true)
		return nil, false
	}

	if !appHasTool(app, tool.ID) {
		log.Debug().
			Uint("app_id", app.GetID()).
			Uint("tool_id", tool.ID).
			Str("tool_slug", toolSlug).
			Msg("Tool authorization denied: app is not entitled to this tool")
		respondWithError(w, http.StatusUnauthorized, "invalid credential", nil, true)
		return nil, false
	}

	ctx := context.WithValue(r.Context(), "tool", tool)
	ctx = context.WithValue(ctx, "toolSlug", toolSlug)
	return ctx, true
}

// Kinds of resource a request path can name, besides a tool.
const (
	targetLLM        = "llm"        // /llm/{rest|stream|call}/{slug}/...
	targetDatasource = "datasource" // /datasource/{slug}[/...]
	targetRoute      = "route"      // /ai/{slug}/... and /anthropic/{slug}/... (the unified /v1 rewrites to /ai/ before auth)
)

// gatewayTarget is the LLM, datasource or bridge route a request path names.
// A zero value means the path names none of them (tool paths, /v1/models, and
// paths the router does not serve), and there is nothing to authorise here.
type gatewayTarget struct {
	kind string
	slug string
}

// targetFromPath splits the path by hand for the same reason toolSlugFromPath
// does: the middleware wraps the router. It parses exactly as the API-key branch
// always has, so both see the same slug. A recognised prefix with no slug keeps
// its kind with an empty slug, which resolves to nothing and is refused.
func targetFromPath(path string) gatewayTarget {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return gatewayTarget{}
	}
	var t gatewayTarget
	switch parts[1] {
	case "llm":
		t.kind = targetLLM
		if len(parts) > 3 {
			t.slug = parts[3]
		} else if len(parts) > 2 {
			t.slug = parts[2]
		}
	case "datasource":
		t.kind = targetDatasource
	case "ai", "anthropic":
		t.kind = targetRoute
	default:
		return gatewayTarget{}
	}
	if t.kind != targetLLM && len(parts) > 2 {
		t.slug = parts[2]
	}
	return t
}

// appHoldsLLM reports whether the app was granted the LLM directly.
func appHoldsLLM(app *models.App, llmID uint) bool {
	if app == nil {
		return false
	}
	for _, l := range app.LLMs {
		if l.ID == llmID {
			return true
		}
	}
	return false
}

// appHoldsDatasource reports whether the app was granted the datasource.
func appHoldsDatasource(app *models.App, dsID uint) bool {
	if app == nil {
		return false
	}
	for _, d := range app.Datasources {
		if d.ID == dsID {
			return true
		}
	}
	return false
}

// targetAllowed is the single implementation of the LLM, datasource and route
// ACL, as authorizeToolAccess is for tools. The API-key branch used to be the
// only one that applied it: the bearer app-secret, custom-auth and
// plugin-authenticated branches authenticated the caller and then let it name
// any LLM or datasource on the gateway.
//
//   - An LLM path passes when the app holds the LLM, inherits it through a
//     router it holds (routerGrantsAccess), or inherits it as a failover rung
//     of an LLM it holds or reached through a router (failoverGrantsAccess).
//     Both inheritances trust the loopback marker only with this process's
//     token.
//   - A route (/ai/, /anthropic/, and so the unified /v1) passes when the app
//     holds the LLM it names. The outer hop always names the primary, so
//     failover inheritance is not needed there; Bedrock is served from this
//     hop directly, so this is its only check. A route that names a router
//     rather than an LLM passes here: the router grant is checked where the
//     router is resolved (resolveRoute), and the LLM it picks is checked on
//     the inner hop (or, for Bedrock, is one the router can reach).
//   - A datasource passes when the app holds it.
//
// Unknown slugs are refused. A path that names none of these passes: tools
// have their own ACL, and anything else is for the router to serve or 404.
func (cv *CredentialValidator) targetAllowed(r *http.Request, app *models.App, t gatewayTarget) bool {
	switch t.kind {
	case "":
		return true
	case targetLLM:
		llm, ok := cv.p.GetLLM(t.slug)
		if !ok {
			return false
		}
		return cv.p.appAllowedLLM(r, app, llm) || cv.p.failoverGrantsAccess(r, app, llm)
	case targetRoute:
		llm, ok := cv.p.GetLLM(t.slug)
		if !ok {
			_, isRouter := cv.p.lookupRouter(t.slug)
			return isRouter
		}
		return appHoldsLLM(app, llm.ID)
	case targetDatasource:
		ds, ok := cv.p.GetDatasource(t.slug)
		if !ok {
			return false
		}
		return appHoldsDatasource(app, ds.ID)
	}
	return false
}

// authorizeTarget applies targetAllowed to the request path for an app that has
// already been authenticated, answering 403 when the app may not use what the
// path names. Every authenticated branch other than the API-key one (which goes
// through CheckAPICredential) calls it before the post-auth hook and next.
//
// Unknown and ungranted get the same status and the same words - the wording
// the /ai/ translator already used for an unknown vendor - so a caller cannot
// use the difference to list the slugs configured on the gateway.
func (cv *CredentialValidator) authorizeTarget(w http.ResponseWriter, r *http.Request, app *models.App) bool {
	t := targetFromPath(r.URL.Path)
	if cv.targetAllowed(r, app, t) {
		return true
	}
	log.Debug().
		Uint("app_id", app.GetID()).
		Str("target_kind", t.kind).
		Str("target_slug", t.slug).
		Msg("Authorization denied: app is not granted the requested resource")
	respondWithError(w, http.StatusForbidden, targetDeniedMessage(t), nil, false)
	return false
}

// targetDeniedMessage is the 403 message for an app that is not granted t.
func targetDeniedMessage(t gatewayTarget) string {
	noun := map[string]string{targetLLM: "LLM", targetRoute: "vendor", targetDatasource: "datasource"}[t.kind]
	return fmt.Sprintf("%s '%s' not found or not supported by your access rights", noun, t.slug)
}

// credentialCheckRetryAfter is the Retry-After, in seconds, on a 503 for a
// credential that could not be checked.
const credentialCheckRetryAfter = "5"

// respondCredentialCheckUnavailable answers a request whose credential could
// not be checked (services.ErrCredentialCheckUnavailable) with a retryable 503
// in the OpenAI error shape, like the gateway's overload refusal. A 401 here
// would tell the client its key is wrong; SDKs do not retry those.
func respondCredentialCheckUnavailable(w http.ResponseWriter, err error) {
	log.Warn().Err(err).Msg("Credential could not be checked; answering 503")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", credentialCheckRetryAfter)
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"message": "The gateway cannot check credentials right now; retry shortly.",
			"type":    oaiErrorType(http.StatusServiceUnavailable),
			"code":    "credential_check_unavailable",
		},
	})
}

// respondCredentialLookupError answers a GetCredentialBySecret error that is
// not a plain rejection: a check that could not run (503) or an inactive App
// (403). It reports false, having written nothing, for a plain rejection, which
// each branch answers in its own words.
func respondCredentialLookupError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, services.ErrCredentialCheckUnavailable):
		respondCredentialCheckUnavailable(w, err)
		return true
	case errors.Is(err, services.ErrAppInactive):
		respondWithError(w, http.StatusForbidden, services.AppInactiveMessage, nil, true)
		return true
	}
	return false
}

func (cv *CredentialValidator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) < 2 {
			respondWithError(w, http.StatusBadRequest, "invalid request path", nil, false)
			return
		}

		// Discovery metadata is public by definition - it is how a client learns
		// where to authenticate - so it is served before any credential handling.
		// It was previously exempted further down, which meant a client that did
		// present a credential had its metadata request run through authentication.
		if pathParts[1] == ".well-known" {
			next.ServeHTTP(w, r)
			return
		}

		// === HOOK POINT: PRE-AUTH ===
		// Execute pre-auth hooks BEFORE any authentication logic (for LLM proxy requests)
		if cv.authHooks != nil && cv.authHooks.PreAuth != nil {
			if blocked := cv.authHooks.PreAuth(w, r); blocked {
				return // Pre-auth hook blocked the request
			}
		}

		// The inner hop of the /ai/ loopback, for a caller an auth plugin
		// authenticated on the outer hop.
		if cv.handoffAuth(w, r, next) {
			return
		}

		// --- Bearer Token Authentication (includes OAuth for MCP servers) ---
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")

			// First try OAuth access token lookup using interface method
			accessToken, err := cv.service.GetValidAccessTokenByToken(tokenString)
			if err == nil {
				// A valid OAuth access token authenticates a user - it does not, on its
				// own, authorise anything. Authorisation comes from the app the user
				// selected at consent, which the token carries as AppID, and from the
				// app's tool grants. This branch used to stop at "the token exists and
				// has not expired", which let any token reach any tool's MCP server.
				toolSlug := toolSlugFromPath(r.URL.Path)
				if toolSlug == "" {
					// OAuth access tokens are issued for MCP. They authorise the tool
					// endpoints and their MCP transports, nothing else: LLM, datasource
					// and /ai/ traffic authenticates with an app secret or API key.
					log.Debug().Str("path", r.URL.Path).Msg("OAuth token presented on a non-tool path")
					respondWithError(w, http.StatusUnauthorized, "Invalid or expired bearer token", nil, true)
					return
				}

				if !oauthscope.Grants(accessToken.Scope, oauthscope.MCP) {
					log.Debug().
						Str("scope", accessToken.Scope).
						Str("required", oauthscope.MCP).
						Msg("OAuth token rejected: scope does not grant MCP access")
					respondWithError(w, http.StatusUnauthorized, "insufficient scope for this resource", nil, true)
					return
				}

				// Fail closed: a token with no app binding was never authorised against
				// anything, so it grants nothing.
				if accessToken.AppID == nil {
					log.Warn().
						Uint("token_id", accessToken.ID).
						Uint("user_id", accessToken.UserID).
						Msg("OAuth token rejected: no app binding")
					respondWithError(w, http.StatusUnauthorized, "invalid credential", nil, true)
					return
				}

				user, err := cv.service.GetUserByID(accessToken.UserID)
				if err != nil {
					respondWithError(w, http.StatusInternalServerError, "Could not retrieve user for token", err, false)
					return
				}
				if user.Disabled {
					log.Warn().
						Uint("token_id", accessToken.ID).
						Uint("user_id", user.ID).
						Msg("OAuth token rejected: user account is disabled")
					respondWithError(w, http.StatusForbidden, "user account is disabled", nil, true)
					return
				}

				oauthClient, err := cv.service.GetOAuthClient(accessToken.ClientID)
				if err != nil {
					respondWithError(w, http.StatusInternalServerError, "Could not retrieve client for token", err, false)
					return
				}

				app, err := cv.service.GetAppByID(*accessToken.AppID)
				if err != nil {
					log.Debug().Err(err).Uint("app_id", *accessToken.AppID).Msg("OAuth token references an app that could not be loaded")
					respondWithError(w, http.StatusUnauthorized, "invalid credential", nil, true)
					return
				}
				if !app.IsActive {
					respondWithError(w, http.StatusForbidden, "app is inactive", nil, true)
					return
				}

				// Re-check the ownership consent established. The consent screen and
				// the token endpoint both verify it, but this is the point where the
				// binding is actually trusted, and the whole reason this branch exists
				// is that a token must not be believed about its own authority. App
				// ownership is a single scalar (models.App.UserID), and the edge syncs
				// it, so the check is the same on both planes.
				if app.UserID != accessToken.UserID {
					log.Warn().
						Uint("token_id", accessToken.ID).
						Uint("token_user_id", accessToken.UserID).
						Uint("app_id", app.ID).
						Uint("app_user_id", app.UserID).
						Msg("OAuth token rejected: bound app is not owned by the token's user")
					respondWithError(w, http.StatusUnauthorized, "invalid credential", nil, true)
					return
				}

				// The same tool ACL every other branch applies.
				ctx, ok := cv.authorizeToolAccess(w, r, app, toolSlug)
				if !ok {
					return
				}

				ctx = context.WithValue(ctx, "user", user)
				ctx = context.WithValue(ctx, "oauthClient", oauthClient)
				ctx = context.WithValue(ctx, "scope", accessToken.Scope)
				// The app is what budget, analytics, filters and the plugin hooks read.
				ctx = context.WithValue(ctx, "app", app)
				ctx = WithAuthIdentity(ctx, &AuthIdentity{AppID: app.ID, Method: AuthMethodOAuth, OAuthUserID: user.ID})

				// Tool requests do not fire the post-auth hook, matching the app-secret
				// branch below, which returns before reaching it.
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// OAuth token lookup failed - continue to check app secret or API key below
		}

		// If Bearer token but not OAuth, try app secret lookup
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")

			// An endpoint with auth plugins is authenticated by them alone.
			if cv.pluginAuth(w, r, next, tokenString, CredTypeBearer) {
				return
			}

			// Standard Bearer token validation (app secret)
			cred, err := cv.service.GetCredentialBySecret(tokenString)
			if err == nil && cred.Active {
				app, err := cv.service.GetAppByCredentialID(cred.ID)
				if err == nil && !app.IsActive {
					respondWithError(w, http.StatusForbidden, "app is inactive", nil, true)
					return
				}
				if err == nil {
					// Valid app secret - add app to context like API key flow
					ctx := context.WithValue(r.Context(), "app", app)
					ctx = WithAuthIdentity(ctx, &AuthIdentity{AppID: app.ID, Method: AuthMethodAppKey})

					// For tool requests, validate the app has access to the tool
					if toolSlug := toolSlugFromPath(r.URL.Path); toolSlug != "" {
						toolCtx, ok := cv.authorizeToolAccess(w, r, app, toolSlug)
						if !ok {
							return
						}
						ctx = context.WithValue(toolCtx, "app", app)

						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}

					// Not a tool request - continue with LLM request. The app must hold
					// the LLM, datasource or route the path names; this is the same
					// check the API-key branch makes in CheckAPICredential. Both hops of
					// the /ai/ bridge land here, since the loopback forwards the caller's
					// bearer secret, and failover rungs pass on the inherited-access marker.
					if !cv.authorizeTarget(w, r, app) {
						return
					}

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

			// The app secret could not be checked, or belongs to an inactive
			// App: neither is "invalid token".
			if err != nil && respondCredentialLookupError(w, err) {
				return
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
			case "ai", "anthropic":
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
				// The Anthropic Messages bridge (Claude Code) presents the app key via the
				// x-api-key header (ANTHROPIC_API_KEY). Bearer tokens are handled earlier.
				if pathParts[1] == "anthropic" {
					if k, xerr := AnthropicValidator(r); xerr == nil {
						apiKey = k
					}
				}
				if apiKey == "" {
					respondWithError(w, http.StatusUnauthorized, "missing credentials for route", nil, true) // true for wwwAuth
					return
				}
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

		// === AUTH PLUGINS: an endpoint with any is authenticated by them alone ===
		if cv.pluginAuth(w, r, next, apiKey, CredTypeAPIKey) {
			return
		}

		// === STANDARD API KEY VALIDATION (only if no auth plugin) ===
		denial, reqWithCtx := cv.checkAPICredential(apiKey, dsSlug, llmSlug, routeID, toolSlug, r)
		if denial != nil {
			if denial.err != nil && respondCredentialLookupError(w, denial.err) {
				return
			}
			respondWithError(w, denial.status, denial.message, nil, denial.status == http.StatusUnauthorized) // wwwAuth on 401 only
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
	denial, r := cv.checkAPICredential(apiKey, dsSlug, llmSlug, routeID, toolSlug, r)
	return denial == nil, r
}

// apiKeyDenial is why the API-key branch refused a request, as the response
// to give. err, when set, is the credential lookup error, which may call for a
// 503 or a 403 instead (respondCredentialLookupError).
type apiKeyDenial struct {
	status  int
	message string
	err     error
}

// apiKeyInvalid is the answer for a key that does not authenticate.
var apiKeyInvalid = &apiKeyDenial{status: http.StatusUnauthorized, message: "Invalid API key or insufficient permissions."}

// apiKeyTargetDenied is the answer for a valid key whose App is not granted t:
// a 403, the same as every other authentication branch gives (authorizeTarget).
// This branch used to answer 401, telling the caller to fix a key that works.
func apiKeyTargetDenied(t gatewayTarget) *apiKeyDenial {
	return &apiKeyDenial{status: http.StatusForbidden, message: targetDeniedMessage(t)}
}

// checkAPICredential is CheckAPICredential reporting why a request is refused;
// a nil denial means it is allowed.
func (cv *CredentialValidator) checkAPICredential(apiKey, dsSlug, llmSlug, routeID, toolSlug string, r *http.Request) (*apiKeyDenial, *http.Request) {
	cred, err := cv.service.GetCredentialBySecret(apiKey) // API Key is the 'secret'
	if err != nil {
		log.Debug().Err(err).Str("api_key_prefix", apiKey[:min(len(apiKey), 8)]).Msg("CheckAPICredential: GetCredentialBySecret failed")
		return &apiKeyDenial{status: apiKeyInvalid.status, message: apiKeyInvalid.message, err: err}, r
	}
	if !cred.Active {
		log.Debug().Uint("cred_id", cred.ID).Msg("CheckAPICredential: Credential is inactive")
		// Log inactive credential usage for compliance tracking
		if app, appErr := cv.service.GetAppByCredentialID(cred.ID); appErr == nil {
			analytics.RecordProxyLog(r.Context(), &models.ProxyLog{
				AppID:        app.ID,
				UserID:       app.UserID,
				ResponseCode: http.StatusUnauthorized,
				TimeStamp:    time.Now(),
				Vendor:       "auth",
				ResponseBody: `{"error":"credential_inactive","detail":"API credential is inactive"}`,
			})
		}
		return apiKeyInvalid, r
	}

	log.Debug().
		Uint("cred_id", cred.ID).
		Int("cred_id_signed", int(cred.ID)).
		Str("key_id", cred.KeyID).
		Msg("CheckAPICredential: Credential found and active")

	app, err := cv.service.GetAppByCredentialID(cred.ID)
	if err != nil {
		log.Debug().Err(err).Uint("cred_id", cred.ID).Int("cred_id_signed", int(cred.ID)).Msg("CheckAPICredential: GetAppByCredentialID failed")
		return &apiKeyDenial{status: apiKeyInvalid.status, message: apiKeyInvalid.message, err: err}, r
	}

	if !app.IsActive {
		// The app's live switch (apps:publish) is off. The microgateway already
		// refuses these; the embedded gateway has to agree or "deactivate" only
		// takes effect at the edge.
		log.Debug().Uint("app_id", app.ID).Msg("CheckAPICredential: App is inactive")
		analytics.RecordProxyLog(r.Context(), &models.ProxyLog{
			AppID:        app.ID,
			UserID:       app.UserID,
			ResponseCode: http.StatusForbidden,
			TimeStamp:    time.Now(),
			Vendor:       "auth",
			ResponseBody: `{"error":"app_inactive","detail":"app is inactive"}`,
		})
		return &apiKeyDenial{status: http.StatusForbidden, message: services.AppInactiveMessage}, r
	}

	log.Debug().
		Uint("app_id", app.ID).
		Str("app_name", app.Name).
		Int("llm_count", len(app.LLMs)).
		Msg("CheckAPICredential: Retrieved app for credential")

	ctx := context.WithValue(r.Context(), "app", app)
	ctx = WithAuthIdentity(ctx, &AuthIdentity{AppID: app.ID, Method: AuthMethodAppKey})
	// Note: toolSlug might be already in r.Context() if set before calling this func
	// but setting it again here from param ensures it's the one CheckAPICredential is using.
	if toolSlug != "" {
		ctx = context.WithValue(ctx, "toolSlug", toolSlug)
	}
	r = r.WithContext(ctx)

	// The resource half of the check is shared with every other authentication
	// branch (see targetAllowed), so the rules cannot drift between them again.
	// A key that authenticates but is not granted the target is a 403.
	check := func(t gatewayTarget) (*apiKeyDenial, *http.Request) {
		if cv.targetAllowed(r, app, t) {
			return nil, r
		}
		return apiKeyTargetDenied(t), r
	}

	if dsSlug != "" {
		return check(gatewayTarget{kind: targetDatasource, slug: dsSlug})
	}

	if llmSlug != "" {
		// A failover rung is allowed through the inherited-access marker.
		allowed := cv.targetAllowed(r, app, gatewayTarget{kind: targetLLM, slug: llmSlug})
		log.Debug().
			Uint("app_id", app.ID).
			Str("llm_slug", llmSlug).
			Int("app_llm_count", len(app.LLMs)).
			Bool("allowed", allowed).
			Msg("CheckAPICredential: LLM access check")
		if !allowed {
			return apiKeyTargetDenied(gatewayTarget{kind: targetLLM, slug: llmSlug}), r
		}
		return nil, r
	}

	if routeID != "" { // /ai/{routeID} and /anthropic/{routeID}: routeID is an LLM slug
		return check(gatewayTarget{kind: targetRoute, slug: routeID})
	}

	if toolSlugContext := r.Context().Value("toolSlug"); toolSlugContext != nil {
		if ts, ok := toolSlugContext.(string); ok && ts != "" {
			// Tools answer 401 whether missing or not granted, as in
			// authorizeToolAccess: the caller must not learn which.
			tool, err := cv.service.GetToolBySlug(ts)
			if err != nil {
				return apiKeyInvalid, r
			}
			if appHasTool(app, tool.ID) {
				ctx := context.WithValue(r.Context(), "tool", tool) // Add full tool to context
				return nil, r.WithContext(ctx)
			}
			return apiKeyInvalid, r
		}
	}

	return apiKeyInvalid, r // Default to no access if no specific resource type matches
}

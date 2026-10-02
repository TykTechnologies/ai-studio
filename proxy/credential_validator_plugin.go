package proxy

import (
	"context"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/rs/zerolog/log"
)

// authTarget resolves the endpoint a request addresses, for CustomAuth. It
// reports false for a path that names none, or names one that does not
// exist; app keys then apply and the request is refused or routed as before.
func (cv *CredentialValidator) authTarget(r *http.Request) (AuthTarget, bool) {
	if slug := toolSlugFromPath(r.URL.Path); slug != "" {
		tool, err := cv.service.GetToolBySlug(slug)
		if err != nil || tool == nil {
			return AuthTarget{}, false
		}
		return AuthTarget{Kind: AuthTargetTool, ID: tool.ID, Slug: slug}, true
	}

	t := targetFromPath(r.URL.Path)
	switch t.kind {
	case targetLLM:
		if llm, ok := cv.p.GetLLM(t.slug); ok {
			return AuthTarget{Kind: AuthTargetLLM, ID: llm.ID, Slug: t.slug}, true
		}
	case targetDatasource:
		if ds, ok := cv.p.GetDatasource(t.slug); ok {
			return AuthTarget{Kind: AuthTargetDatasource, ID: ds.ID, Slug: t.slug}, true
		}
	case targetRoute:
		// /ai/{slug}, /anthropic/{slug} and the unified /v1 (rewritten to
		// /ai/): an LLM's list, or the router's.
		if llm, ok := cv.p.GetLLM(t.slug); ok {
			return AuthTarget{Kind: AuthTargetLLM, ID: llm.ID, Slug: t.slug}, true
		}
		if ref, ok := cv.p.lookupRouter(t.slug); ok {
			kind := AuthTargetModelRouter
			if ref.Kind == RouterKindSemantic {
				kind = AuthTargetSemanticRouter
			}
			return AuthTarget{Kind: kind, ID: ref.ID, Slug: t.slug}, true
		}
	}
	return AuthTarget{}, false
}

// pluginAuth asks the endpoint's auth plugins about the credential. It
// reports whether it has answered the request (accepted and served it, or
// refused it); false means the endpoint has no auth plugins and app keys
// apply.
func (cv *CredentialValidator) pluginAuth(w http.ResponseWriter, r *http.Request, next http.Handler, credential, credType string) bool {
	if cv.authHooks == nil || cv.authHooks.CustomAuth == nil {
		return false
	}
	target, ok := cv.authTarget(r)
	if !ok {
		return false
	}

	result, err := cv.authHooks.CustomAuth(r, target, credential, credType)
	if err != nil {
		log.Warn().Err(err).
			Str("target_kind", target.Kind).
			Str("target_slug", target.Slug).
			Msg("Auth plugins could not be asked; refusing the request")
		respondWithError(w, http.StatusServiceUnavailable, "authentication is unavailable, retry shortly", nil, false)
		return true
	}

	switch result.Outcome {
	case AuthNotHandled:
		return false
	case AuthAccepted:
	default:
		log.Debug().
			Str("target_kind", target.Kind).
			Str("target_slug", target.Slug).
			Str("reason", result.Reason).
			Msg("Auth plugins refused the credential")
		respondWithError(w, http.StatusUnauthorized, "invalid credential", nil, true)
		return true
	}

	// An auth plugin authenticates; it does not authorise. The app it named
	// must exist, be active and hold what the path addresses.
	app, err := cv.service.GetAppByID(result.AppID)
	if err != nil {
		log.Warn().Err(err).
			Uint("app_id", result.AppID).
			Uint("plugin_id", result.PluginID).
			Msg("Auth plugin named an app that could not be loaded")
		respondWithError(w, http.StatusUnauthorized, "invalid credential", nil, true)
		return true
	}
	if !app.IsActive {
		respondWithError(w, http.StatusForbidden, "app is inactive", nil, true)
		return true
	}

	cv.completeAuth(w, r, next, app, &AuthIdentity{
		AppID:      app.ID,
		Method:     AuthMethodPlugin,
		PluginID:   result.PluginID,
		PluginName: result.PluginName,
		Subject:    result.Subject,
		Claims:     result.Claims,
	})
	return true
}

// handoffAuth authenticates the inner hop of the /ai/ loopback as the outer
// hop's identity (see authHandoffHeaders). The app must still hold the LLM
// the inner hop calls. It reports false when the request carries no trusted
// hand-off.
func (cv *CredentialValidator) handoffAuth(w http.ResponseWriter, r *http.Request, next http.Handler) bool {
	if !IsInternalHop(r) {
		return false
	}
	id, ok := cv.p.parseAuthHandoff(r)
	if !ok {
		return false
	}
	app, err := cv.service.GetAppByID(id.AppID)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "invalid credential", nil, true)
		return true
	}
	if !app.IsActive {
		respondWithError(w, http.StatusForbidden, "app is inactive", nil, true)
		return true
	}
	cv.completeAuth(w, r, next, app, id)
	return true
}

// completeAuth finishes a request whose credential named app: the app must
// hold the tool, LLM, datasource or route the path addresses, then the
// identity goes on the context, the post-auth hook runs (not for tools,
// as on every branch) and the request is served.
func (cv *CredentialValidator) completeAuth(w http.ResponseWriter, r *http.Request, next http.Handler, app *models.App, id *AuthIdentity) {
	if toolSlug := toolSlugFromPath(r.URL.Path); toolSlug != "" {
		ctx, ok := cv.authorizeToolAccess(w, r, app, toolSlug)
		if !ok {
			return
		}
		ctx = context.WithValue(ctx, "app", app)
		next.ServeHTTP(w, r.WithContext(WithAuthIdentity(ctx, id)))
		return
	}

	if !cv.authorizeTarget(w, r, app) {
		return
	}

	ctx := context.WithValue(r.Context(), "app", app)
	r = r.WithContext(WithAuthIdentity(ctx, id))

	if cv.authHooks != nil && cv.authHooks.PostAuth != nil {
		if blocked := cv.authHooks.PostAuth(w, r, app.ID); blocked {
			return
		}
	}
	next.ServeHTTP(w, r)
}

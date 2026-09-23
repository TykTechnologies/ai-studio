package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/rs/zerolog/log"
)

// Routers.
//
// A router is a named target on the /ai/ chain that is not an LLM: the caller
// sends {"model": "<router>/<model>"} to the unified ingress (or calls
// /ai/<router>/v1/... directly) and the router picks the LLM and model that
// serve the request. The ingress rewrites without resolving anything, so the
// router is resolved here, after authentication, with the authenticated App
// in hand: an anonymous caller costs nothing and learns nothing.
//
// Routers are a data-plane feature of the edge gateway. The proxy only knows
// the interface; the host installs an implementation with SetRouteResolver
// (the microgateway does, AI Studio's embedded gateway does not, so a router
// slug there is just an unknown vendor).
//
// Access follows the LLM model. An App is granted a router the way it is
// granted an LLM, and the grant lets it reach every LLM the router can pick,
// but only through the router: the bridge marks its loopback requests with the
// router it resolved, and the inner /llm/call/ hop honours that marker only
// with this process's token, only when the App holds the router, and only for
// an LLM the router can actually reach (routerGrantsAccess).

// RouterKind names the kind of router a slug resolves to.
type RouterKind string

const (
	// RouterKindModel is the Enterprise Model Router: pools matched on the
	// requested model name, vendors picked by round robin or weight.
	RouterKindModel RouterKind = "model_router"
)

// RouterRef identifies a router.
type RouterRef struct {
	Kind RouterKind
	ID   uint
	Slug string
}

// RouteRequest is what a resolver is asked to route.
type RouteRequest struct {
	Router RouterRef
	App    *models.App
	// Model is the model the caller asked for, with the router prefix
	// already stripped by the ingress.
	Model string
	// Body is the OpenAI-shaped request body as the caller sent it.
	Body []byte
	// Header is the caller's request header (session affinity and the like).
	Header http.Header
	// Allow restricts the LLMs the resolver may pick. Nil allows every LLM
	// the router can reach.
	Allow func(llmID uint) bool
}

// RouteDecision is a resolver's answer: the LLM and model that serve the
// request, and why.
type RouteDecision struct {
	Router RouterRef
	LLMID  uint
	Model  string
	// Pool is the Model Router pool that matched.
	Pool string
	// Route is the route that was chosen, for routers that have named routes.
	Route string
	// Reason says why this target was chosen, from a small fixed set
	// ("model_pattern", ...), for analytics and the X-Tyk-Route-Reason header.
	Reason string
}

// RouteResolver resolves router slugs. Implementations must be safe for
// concurrent use.
type RouteResolver interface {
	// Lookup reports whether slug names a router.
	Lookup(slug string) (RouterRef, bool)
	// Resolve picks the LLM and model for a request. Errors are
	// ErrRouteNoMatch, ErrRouteNoCandidates or ErrRouteUnavailable, possibly
	// wrapped.
	Resolve(ctx context.Context, req RouteRequest) (*RouteDecision, error)
	// Reaches reports whether the router can send a request to the LLM.
	Reaches(ref RouterRef, llmID uint) bool
	// Models lists the model names the router advertises (GET /v1/models).
	Models(ref RouterRef) []string
}

var (
	// ErrRouteNoMatch: the request names nothing the router routes (400).
	ErrRouteNoMatch = errors.New("no route matches the requested model")
	// ErrRouteNoCandidates: the router would route the request, but to
	// nothing this App may use (403).
	ErrRouteNoCandidates = errors.New("the router has no target this app may use")
	// ErrRouteUnavailable: every target the router could pick is inactive or
	// unloaded (503).
	ErrRouteUnavailable = errors.New("the router has no available target")
)

// SetRouteResolver installs the router implementation. Called once by the
// host before the proxy serves traffic; nil disables routers.
func (p *Proxy) SetRouteResolver(r RouteResolver) {
	p.mu.Lock()
	p.routeResolver = r
	p.mu.Unlock()
}

func (p *Proxy) resolver() RouteResolver {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.routeResolver
}

// lookupRouter reports whether slug names a router on this gateway.
func (p *Proxy) lookupRouter(slug string) (RouterRef, bool) {
	res := p.resolver()
	if res == nil || slug == "" {
		return RouterRef{}, false
	}
	return res.Lookup(slug)
}

// appHoldsRouter reports whether the App was granted the router.
func appHoldsRouter(app *models.App, ref RouterRef) bool {
	if app == nil {
		return false
	}
	switch ref.Kind {
	case RouterKindModel:
		for _, r := range app.ModelRouters {
			if r.ID == ref.ID {
				return true
			}
		}
	}
	return false
}

// routeError maps a resolver error to the status the caller gets.
func routeErrorStatus(err error) int {
	switch {
	case errors.Is(err, ErrRouteNoMatch):
		return http.StatusBadRequest
	case errors.Is(err, ErrRouteNoCandidates):
		return http.StatusForbidden
	case errors.Is(err, ErrRouteUnavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadGateway
	}
}

// legacyRouterWarned remembers the (app, router) pairs the deprecation
// warning was logged for, so it is logged once per process, not per request.
var legacyRouterWarned sync.Map

// resolveRoute routes a bridge request addressed to a router. It enforces
// the grant: an App must hold the router, except that a Model Router still
// serves an App without a grant from the LLMs the App holds directly (the
// behaviour before routers were grantable; deprecated).
func (p *Proxy) resolveRoute(r *http.Request, ref RouterRef, model string, body []byte) (*RouteDecision, int, error) {
	res := p.resolver()
	if res == nil {
		return nil, http.StatusNotFound, ErrRouteNoMatch
	}
	app, err := p.getAppFromContext(r, nil)
	if err != nil {
		return nil, http.StatusUnauthorized, err
	}
	req := RouteRequest{Router: ref, App: app, Model: model, Body: body, Header: r.Header}
	if !appHoldsRouter(app, ref) {
		if ref.Kind != RouterKindModel {
			return nil, http.StatusForbidden, ErrRouteNoCandidates
		}
		// Deprecated: before routers were grantable, an App reached a Model
		// Router by holding the LLMs behind it. Such an App is still served,
		// from its own LLMs only; the marker the loopback carries grants it
		// nothing, because it does not hold the router.
		if _, seen := legacyRouterWarned.LoadOrStore(fmt.Sprintf("%d:%d", app.ID, ref.ID), true); !seen {
			log.Warn().Uint("app_id", app.ID).Str("router", ref.Slug).
				Msg("App called a Model Router it has not been granted; routing to its own LLMs only (deprecated; grant the router to the app)")
		}
		req.Allow = func(llmID uint) bool { return appHoldsLLM(app, llmID) }
	}
	d, err := res.Resolve(r.Context(), req)
	if err != nil {
		return nil, routeErrorStatus(err), err
	}
	d.Router = ref
	return d, http.StatusOK, nil
}

// routeBridgeRequest resolves a bridge request addressed to a router and
// re-targets it: it returns the chosen LLM, the request carrying the decision
// on its context, and the body with the chosen model. *model is rewritten in
// place. On failure it has answered the client and ok is false.
func (p *Proxy) routeBridgeRequest(w http.ResponseWriter, r *http.Request, ref RouterRef, model *string, body []byte) (*models.LLM, *http.Request, []byte, bool) {
	d, status, err := p.resolveRoute(r, ref, *model, body)
	if err != nil {
		msg := fmt.Sprintf("router '%s' cannot route model '%s': %v", ref.Slug, *model, err)
		if status == http.StatusUnauthorized {
			msg = "authentication required"
		}
		respondWithOAIError(w, status, msg, nil, false)
		return nil, r, body, false
	}
	conf, ok := p.GetLLMByID(d.LLMID)
	if !ok {
		respondWithOAIError(w, http.StatusServiceUnavailable,
			fmt.Sprintf("router '%s' chose an LLM that is not loaded on this gateway", ref.Slug), nil, false)
		return nil, r, body, false
	}
	log.Debug().
		Str("router", ref.Slug).
		Str("kind", string(ref.Kind)).
		Str("requested_model", *model).
		Str("llm", conf.Name).
		Str("model", d.Model).
		Str("pool", d.Pool).
		Str("route", d.Route).
		Str("reason", d.Reason).
		Msg("router resolved request")
	*model = d.Model
	setRouteHeaders(w, d)
	return conf, r.WithContext(withRouteDecision(r.Context(), d)), withBodyModel(body, d.Model), true
}

// withBodyModel returns body with its "model" field set to model, every other
// field forwarded verbatim. A body that is not a JSON object is returned as is.
func withBodyModel(body []byte, model string) []byte {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return body
	}
	m, err := json.Marshal(model)
	if err != nil {
		return body
	}
	fields["model"] = m
	out, err := json.Marshal(fields)
	if err != nil {
		return body
	}
	return out
}

// Loopback router marker. Carried next to the failover marker and trusted
// only with the same per-process token.
const (
	hdrRouterOrigin = "X-Tyk-Router-Origin" // router slug
	hdrRouterKind   = "X-Tyk-Router-Kind"
	hdrRouterPool   = "X-Tyk-Router-Pool"
	hdrRouterRoute  = "X-Tyk-Router-Route"
	hdrRouterReason = "X-Tyk-Router-Reason"
)

// Client-facing response headers naming the routing decision.
const (
	hdrRouter      = "X-Tyk-Router"
	hdrRoute       = "X-Tyk-Route"
	hdrRouteReason = "X-Tyk-Route-Reason"
)

// routerCORSExposeHeaders lists the routing headers for
// Access-Control-Expose-Headers.
const routerCORSExposeHeaders = hdrRouter + ", " + hdrRoute + ", " + hdrRouteReason

type routeDecisionKey struct{}

func withRouteDecision(ctx context.Context, d *RouteDecision) context.Context {
	return context.WithValue(ctx, routeDecisionKey{}, d)
}

func routeDecisionFromContext(ctx context.Context) (*RouteDecision, bool) {
	d, ok := ctx.Value(routeDecisionKey{}).(*RouteDecision)
	return d, ok && d != nil
}

// routerHeaders adds the router marker for the decision on ctx to h. The
// marker names the router for analytics on every call, and grants access
// only when the App holds the router (routerGrantsAccess), so an ungranted
// App's call carries it harmlessly.
func (p *Proxy) routerHeaders(ctx context.Context, h http.Header) http.Header {
	d, ok := routeDecisionFromContext(ctx)
	if !ok {
		return h
	}
	if h == nil {
		h = http.Header{}
	}
	h.Set(hdrRouterOrigin, d.Router.Slug)
	h.Set(hdrRouterKind, string(d.Router.Kind))
	if d.Pool != "" {
		h.Set(hdrRouterPool, d.Pool)
	}
	if d.Route != "" {
		h.Set(hdrRouterRoute, d.Route)
	}
	if d.Reason != "" {
		h.Set(hdrRouterReason, d.Reason)
	}
	h.Set(hdrFailoverToken, p.failoverToken)
	return h
}

// loopbackHeaders is every marker a loopback request for attempt a carries.
func (p *Proxy) loopbackHeaders(ctx context.Context, a llmAttempt) http.Header {
	return p.routerHeaders(ctx, p.failoverHeaders(a))
}

// routerMarker is what the inner hop learns from a trusted router marker.
type routerMarker struct {
	Ref    RouterRef
	Pool   string
	Route  string
	Reason string
}

type routerMarkerKey struct{}

func withRouterMarker(ctx context.Context, m routerMarker) context.Context {
	return context.WithValue(ctx, routerMarkerKey{}, m)
}

func routerMarkerFromContext(ctx context.Context) (routerMarker, bool) {
	m, ok := ctx.Value(routerMarkerKey{}).(routerMarker)
	return m, ok
}

// parseRouterMarker reads a trusted router marker off the inner-hop request.
// Like the failover marker, anything untrusted is ignored rather than refused.
func (p *Proxy) parseRouterMarker(r *http.Request) (routerMarker, bool) {
	if !p.tokenMatches(r) {
		return routerMarker{}, false
	}
	ref, ok := p.lookupRouter(r.Header.Get(hdrRouterOrigin))
	if !ok || string(ref.Kind) != r.Header.Get(hdrRouterKind) {
		return routerMarker{}, false
	}
	return routerMarker{
		Ref:    ref,
		Pool:   r.Header.Get(hdrRouterPool),
		Route:  r.Header.Get(hdrRouterRoute),
		Reason: r.Header.Get(hdrRouterReason),
	}, true
}

// routerGrantsAccess is the router-inherited access rule for the inner hop:
// the request carries a trusted marker naming a router, the App holds that
// router, and the router can reach the LLM.
func (p *Proxy) routerGrantsAccess(r *http.Request, app *models.App, target *models.LLM) bool {
	if app == nil || target == nil {
		return false
	}
	m, ok := p.parseRouterMarker(r)
	if !ok || !appHoldsRouter(app, m.Ref) {
		return false
	}
	res := p.resolver()
	return res != nil && res.Reaches(m.Ref, target.ID)
}

// appAllowedLLM is the inner hop's full LLM access rule short of failover: a
// direct grant, or one inherited through a router.
func (p *Proxy) appAllowedLLM(r *http.Request, app *models.App, llm *models.LLM) bool {
	return appHoldsLLM(app, llm.ID) || p.routerGrantsAccess(r, app, llm)
}

// stripRouterHeaders removes the router marker before a request leaves for
// the vendor.
func stripRouterHeaders(h http.Header) {
	h.Del(hdrRouterOrigin)
	h.Del(hdrRouterKind)
	h.Del(hdrRouterPool)
	h.Del(hdrRouterRoute)
	h.Del(hdrRouterReason)
}

// setRouteHeaders tells the client which router served the request and why.
func setRouteHeaders(w http.ResponseWriter, d *RouteDecision) {
	if d == nil {
		return
	}
	h := w.Header()
	h.Set(hdrRouter, d.Router.Slug)
	route := d.Route
	if route == "" {
		route = d.Pool
	}
	if route != "" {
		h.Set(hdrRoute, route)
	}
	if d.Reason != "" {
		h.Set(hdrRouteReason, d.Reason)
	}
}

// applyRouterMarker stamps a ProxyLog with the routing decision on ctx: the
// inner hop's trusted marker, or the bridge's own decision on the paths that
// record analytics without a loopback (Bedrock).
func applyRouterMarker(l *models.ProxyLog, ctx context.Context) {
	if m, ok := routerMarkerFromContext(ctx); ok {
		l.RouterKind = string(m.Ref.Kind)
		l.RouterSlug = m.Ref.Slug
		l.RouterPool = m.Pool
		l.Route = m.Route
		l.RouteReason = m.Reason
		return
	}
	if d, ok := routeDecisionFromContext(ctx); ok {
		l.RouterKind = string(d.Router.Kind)
		l.RouterSlug = d.Router.Slug
		l.RouterPool = d.Pool
		l.Route = d.Route
		l.RouteReason = d.Reason
	}
}

package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/TykTechnologies/midsommar/v2/proxy"
)

// RouterResolver serves both kinds of router to the AI Gateway's /ai/ chain:
// Model Routers (a mapping from the requested model to a pool of vendors) and
// Semantic Routers (a route picked by classifying the prompt). Slugs are
// unique across LLMs and both kinds of router (the hub enforces it), so a slug
// names at most one of them.
//
// A Semantic Router route either names an LLM and model directly, or hands a
// model alias to a Model Router, which then picks the vendor as it would for
// a direct call. Holding the Semantic Router is enough for the hand-off: the
// App needs no grant of the Model Router.
type RouterResolver struct {
	modelRouters *ModelRouterService
	model        *ModelRouterResolver
	semantic     *SemanticRouterService
}

// NewRouterResolver wraps the router services as a proxy.RouteResolver.
// Either may be nil.
func NewRouterResolver(modelRouters *ModelRouterService, semantic *SemanticRouterService) *RouterResolver {
	r := &RouterResolver{modelRouters: modelRouters, semantic: semantic}
	if modelRouters != nil {
		r.model = NewModelRouterResolver(modelRouters)
	}
	return r
}

var _ proxy.RouteResolver = (*RouterResolver)(nil)

// Lookup reports whether slug names a loaded router of either kind.
func (r *RouterResolver) Lookup(slug string) (proxy.RouterRef, bool) {
	if r.model != nil {
		if ref, ok := r.model.Lookup(slug); ok {
			return ref, true
		}
	}
	if r.semantic != nil {
		if c, ok := r.semantic.GetRouter(slug); ok {
			return proxy.RouterRef{Kind: proxy.RouterKindSemantic, ID: c.ID, Slug: c.Slug}, true
		}
	}
	return proxy.RouterRef{}, false
}

// Resolve routes the request with the router it names.
func (r *RouterResolver) Resolve(ctx context.Context, req proxy.RouteRequest) (*proxy.RouteDecision, error) {
	switch req.Router.Kind {
	case proxy.RouterKindModel:
		if r.model == nil {
			return nil, proxy.ErrRouteNoMatch
		}
		return r.model.Resolve(ctx, req)
	case proxy.RouterKindSemantic:
		return r.resolveSemantic(ctx, req)
	}
	return nil, proxy.ErrRouteNoMatch
}

func (r *RouterResolver) semanticRouter(ref proxy.RouterRef) (*compiledSemanticRouter, bool) {
	if r.semantic == nil || ref.Kind != proxy.RouterKindSemantic {
		return nil, false
	}
	c, ok := r.semantic.GetRouter(ref.Slug)
	if !ok || c.ID != ref.ID {
		return nil, false
	}
	return c, true
}

// affinityKey names the caller's session for the router's affinity: the App
// and the session header, so two Apps never share a pin.
func affinityKey(c *compiledSemanticRouter, req proxy.RouteRequest) string {
	a := c.Config.Settings.Affinity
	if !a.Enabled || req.App == nil || req.Header == nil {
		return ""
	}
	header := a.Header
	if header == "" {
		header = sr.DefaultAffinityHeader
	}
	session := strings.TrimSpace(req.Header.Get(header))
	if session == "" {
		return ""
	}
	return fmt.Sprintf("app:%d|%s", req.App.ID, session)
}

func (r *RouterResolver) resolveSemantic(ctx context.Context, req proxy.RouteRequest) (*proxy.RouteDecision, error) {
	c, ok := r.semanticRouter(req.Router)
	if !ok {
		return nil, fmt.Errorf("%w: semantic router %q is not loaded", proxy.ErrRouteNoMatch, req.Router.Slug)
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = sr.ReservedAutoModel
	}
	if model != sr.ReservedAutoModel {
		if _, exists := c.Config.Route(model); !exists || !c.Config.Settings.AllowExplicitRoute {
			return nil, fmt.Errorf("%w: send %q (or a route name, where the router allows it)",
				proxy.ErrRouteNoMatch, req.Router.Slug+"/"+sr.ReservedAutoModel)
		}
	}

	d := r.semantic.Classify(ctx, c, sr.Request{
		Model:       model,
		Messages:    sr.MessagesFromOpenAIBody(req.Body),
		AffinityKey: affinityKey(c, req),
	})
	route, ok := c.Config.Route(d.Route)
	if !ok {
		return nil, fmt.Errorf("%w: route %q is not configured", proxy.ErrRouteUnavailable, d.Route)
	}
	out := &proxy.RouteDecision{
		Route:       route.Name,
		Reason:      d.Reason,
		Score:       d.Score,
		ShadowRoute: d.ShadowRoute,
	}

	switch route.Target.Type {
	case sr.TargetLLM:
		if req.Allow != nil && !req.Allow(route.Target.LLMID) {
			return nil, proxy.ErrRouteNoCandidates
		}
		out.LLMID = route.Target.LLMID
		out.Model = route.Target.Model
		return out, nil

	case sr.TargetModelRouter:
		if r.modelRouters == nil {
			return nil, fmt.Errorf("%w: route %q hands off to a Model Router, which this gateway does not serve", proxy.ErrRouteUnavailable, route.Name)
		}
		mr, ok := r.modelRouters.GetRouterByID(route.Target.ModelRouterID)
		if !ok {
			return nil, fmt.Errorf("%w: route %q hands off to Model Router %d, which is not loaded", proxy.ErrRouteUnavailable, route.Name, route.Target.ModelRouterID)
		}
		sel, err := r.modelRouters.SelectVendorFor(mr.Router.Slug, route.Target.Model, req.Allow)
		switch {
		case err == nil:
		case errors.Is(err, ErrNoPermittedVendors):
			return nil, fmt.Errorf("%w: %v", proxy.ErrRouteNoCandidates, err)
		case errors.Is(err, ErrNoMatchingPool), errors.Is(err, ErrNoActiveVendors), errors.Is(err, ErrRouterNotFound):
			// A configuration the hub accepted but this gateway cannot serve
			// (the Model Router changed since): nothing the caller did.
			return nil, fmt.Errorf("%w: route %q: %v", proxy.ErrRouteUnavailable, route.Name, err)
		default:
			return nil, err
		}
		out.LLMID = sel.Vendor.LLMID
		out.Model = sel.TargetModel
		out.Pool = sel.Pool.Name
		out.Selection = selectionAlgorithm(sel.Pool.SelectionAlgorithm)
		return out, nil
	}
	return nil, fmt.Errorf("%w: route %q has an unknown target", proxy.ErrRouteUnavailable, route.Name)
}

// Reaches reports whether the router can send a request to the LLM. For a
// Semantic Router: an LLM a route targets directly, or a vendor of the pool a
// hand-off alias matches in its Model Router.
func (r *RouterResolver) Reaches(ref proxy.RouterRef, llmID uint) bool {
	switch ref.Kind {
	case proxy.RouterKindModel:
		return r.model != nil && r.model.Reaches(ref, llmID)
	case proxy.RouterKindSemantic:
		c, ok := r.semanticRouter(ref)
		if !ok {
			return false
		}
		for _, route := range c.Config.Routes {
			switch route.Target.Type {
			case sr.TargetLLM:
				if route.Target.LLMID == llmID {
					return true
				}
			case sr.TargetModelRouter:
				if r.modelRouters != nil && r.modelRouters.ReachesModel(route.Target.ModelRouterID, route.Target.Model, llmID) {
					return true
				}
			}
		}
	}
	return false
}

// Models lists the model names the router advertises, without its slug.
func (r *RouterResolver) Models(ref proxy.RouterRef) []string {
	switch ref.Kind {
	case proxy.RouterKindModel:
		if r.model == nil {
			return nil
		}
		return r.model.Models(ref)
	case proxy.RouterKindSemantic:
		c, ok := r.semanticRouter(ref)
		if !ok {
			return nil
		}
		var out []string
		for _, m := range sr.ModelsFor(c.Slug, c.Config) {
			out = append(out, strings.TrimPrefix(m, c.Slug+"/"))
		}
		return out
	}
	return nil
}

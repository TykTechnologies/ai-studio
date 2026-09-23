package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/proxy"
)

// ModelRouterResolver serves Model Routers to the AI Gateway's /ai/ chain.
// The gateway resolves a router slug after authentication (the unified
// ingress sends {"model": "{router}/{model}"} there, and the legacy
// /router/{slug}/ endpoints are an alias of it), with the App in hand, so
// selection can be limited to what the App may use and every decision is
// attributed to the App that caused it.
type ModelRouterResolver struct {
	routers *ModelRouterService
}

// NewModelRouterResolver wraps the router service as a proxy.RouteResolver.
func NewModelRouterResolver(routers *ModelRouterService) *ModelRouterResolver {
	return &ModelRouterResolver{routers: routers}
}

var _ proxy.RouteResolver = (*ModelRouterResolver)(nil)

// Lookup reports whether slug names a loaded Model Router.
func (m *ModelRouterResolver) Lookup(slug string) (proxy.RouterRef, bool) {
	r, ok := m.routers.GetRouter(slug)
	if !ok {
		return proxy.RouterRef{}, false
	}
	return proxy.RouterRef{Kind: proxy.RouterKindModel, ID: r.Router.ID, Slug: r.Router.Slug}, true
}

// Resolve matches the requested model to a pool and picks a vendor in it.
func (m *ModelRouterResolver) Resolve(_ context.Context, req proxy.RouteRequest) (*proxy.RouteDecision, error) {
	if req.Router.Kind != proxy.RouterKindModel {
		return nil, proxy.ErrRouteNoMatch
	}
	sel, err := m.routers.SelectVendorFor(req.Router.Slug, req.Model, req.Allow)
	switch {
	case err == nil:
	case errors.Is(err, ErrNoMatchingPool), errors.Is(err, ErrRouterNotFound):
		return nil, fmt.Errorf("%w: %v", proxy.ErrRouteNoMatch, err)
	case errors.Is(err, ErrNoPermittedVendors):
		return nil, fmt.Errorf("%w: %v", proxy.ErrRouteNoCandidates, err)
	case errors.Is(err, ErrNoActiveVendors):
		return nil, fmt.Errorf("%w: %v", proxy.ErrRouteUnavailable, err)
	default:
		return nil, err
	}
	return &proxy.RouteDecision{
		LLMID:  sel.Vendor.LLMID,
		Model:  sel.TargetModel,
		Pool:   sel.Pool.Name,
		Reason: "model_pattern",
	}, nil
}

// Reaches reports whether the router can pick the LLM.
func (m *ModelRouterResolver) Reaches(ref proxy.RouterRef, llmID uint) bool {
	return ref.Kind == proxy.RouterKindModel && m.routers.Reaches(ref.Slug, ref.ID, llmID)
}

// Models lists the model names the router advertises.
func (m *ModelRouterResolver) Models(ref proxy.RouterRef) []string {
	if ref.Kind != proxy.RouterKindModel {
		return nil
	}
	return m.routers.AdvertisedModels(ref.Slug)
}

package services

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/models"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeSemanticRouter stands in for the Enterprise engine: "prove" picks the
// complex route, "hello" the chit-chat route, anything else the default.
type fakeSemanticRouter struct {
	cfg sr.Config

	mu   sync.Mutex
	last sr.Request
}

func (f *fakeSemanticRouter) Config() sr.Config { return f.cfg }

func (f *fakeSemanticRouter) Classify(_ context.Context, req sr.Request) sr.Decision {
	f.mu.Lock()
	f.last = req
	f.mu.Unlock()
	if req.Model != sr.ReservedAutoModel {
		return sr.Decision{Route: req.Model, Reason: sr.ReasonExplicit}
	}
	text := ""
	if n := len(req.Messages); n > 0 {
		text = req.Messages[n-1].Content
	}
	switch {
	case strings.Contains(text, "prove"):
		return sr.Decision{Route: "complex", Reason: sr.ReasonEmbedding, Score: 0.91}
	case strings.Contains(text, "hello"):
		return sr.Decision{Route: "chat", Reason: sr.ReasonKeyword}
	case strings.Contains(text, "shadow"):
		return sr.Decision{Route: f.cfg.Settings.DefaultRoute, Reason: sr.ReasonEmbedding, ShadowRoute: "complex"}
	}
	return sr.Decision{Route: f.cfg.Settings.DefaultRoute, Reason: sr.ReasonDefault}
}

// semanticFixture: Model Router "prod" (id 1, see routedService) and
// Semantic Router "smart" (id 5): complex -> LLM 20 "opus", chat -> prod
// with alias gpt-4o (pool over LLMs 10 and 11), simple (default) -> prod
// with claude-best (LLM 12, mapped to claude-opus-4).
func semanticFixture(allowExplicit bool) (*RouterResolver, *fakeSemanticRouter) {
	cfg := sr.Config{RouterID: 5, Slug: "smart",
		Settings: sr.Settings{DefaultRoute: "simple", AllowExplicitRoute: allowExplicit,
			Affinity: sr.AffinitySettings{Enabled: true}},
		Routes: []sr.Route{
			{Name: "complex", Target: sr.Target{Type: sr.TargetLLM, LLMID: 20, Model: "opus"}},
			{Name: "chat", Target: sr.Target{Type: sr.TargetModelRouter, ModelRouterID: 1, Model: "gpt-4o"}},
			{Name: "simple", Target: sr.Target{Type: sr.TargetModelRouter, ModelRouterID: 1, Model: "claude-best"}},
		},
	}
	fake := &fakeSemanticRouter{cfg: cfg}
	semantic := NewSemanticRouterService(nil)
	semantic.routers["smart"] = &compiledSemanticRouter{ID: 5, Slug: "smart", Config: cfg, Router: fake}
	return NewRouterResolver(routedService(), semantic), fake
}

func chatBody(text string) []byte {
	return []byte(`{"model":"auto","messages":[{"role":"user","content":"` + text + `"}]}`)
}

func TestRouterResolver_Lookup(t *testing.T) {
	res, _ := semanticFixture(false)
	ref, ok := res.Lookup("smart")
	require.True(t, ok)
	assert.Equal(t, proxy.RouterRef{Kind: proxy.RouterKindSemantic, ID: 5, Slug: "smart"}, ref)
	ref, ok = res.Lookup("prod")
	require.True(t, ok)
	assert.Equal(t, proxy.RouterKindModel, ref.Kind)
	_, ok = res.Lookup("nope")
	assert.False(t, ok)
}

func TestRouterResolver_SemanticRoutes(t *testing.T) {
	res, fake := semanticFixture(false)
	smart := proxy.RouterRef{Kind: proxy.RouterKindSemantic, ID: 5, Slug: "smart"}
	app := &models.App{ID: 42}
	hdr := http.Header{}
	hdr.Set(sr.DefaultAffinityHeader, "sess-1")

	d, err := res.Resolve(context.Background(), proxy.RouteRequest{Router: smart, App: app, Model: "auto",
		Body: chatBody("please prove it"), Header: hdr})
	require.NoError(t, err)
	assert.Equal(t, uint(20), d.LLMID)
	assert.Equal(t, "opus", d.Model)
	assert.Equal(t, "complex", d.Route)
	assert.Equal(t, sr.ReasonEmbedding, d.Reason)
	assert.Equal(t, 0.91, d.Score)
	assert.Equal(t, "app:42|sess-1", fake.last.AffinityKey, "affinity is per App and session")
	assert.Equal(t, "please prove it", fake.last.Messages[0].Content)

	// A hand-off: the Model Router picks the vendor and maps the model.
	d, err = res.Resolve(context.Background(), proxy.RouteRequest{Router: smart, App: app, Model: "auto", Body: chatBody("anything")})
	require.NoError(t, err)
	assert.Equal(t, uint(12), d.LLMID)
	assert.Equal(t, "claude-opus-4", d.Model)
	assert.Equal(t, "simple", d.Route)
	assert.Equal(t, "claude", d.Pool)
	assert.Equal(t, "round_robin", d.Selection)
	assert.Empty(t, fake.last.AffinityKey, "no session header, no affinity")

	d, err = res.Resolve(context.Background(), proxy.RouteRequest{Router: smart, App: app, Model: "auto", Body: chatBody("shadow")})
	require.NoError(t, err)
	assert.Equal(t, "simple", d.Route)
	assert.Equal(t, "complex", d.ShadowRoute)

	// An Allow restriction reaches through the hand-off.
	_, err = res.Resolve(context.Background(), proxy.RouteRequest{Router: smart, App: app, Model: "auto",
		Body: chatBody("hello"), Allow: func(id uint) bool { return id == 20 }})
	assert.ErrorIs(t, err, proxy.ErrRouteNoCandidates)
	_, err = res.Resolve(context.Background(), proxy.RouteRequest{Router: smart, App: app, Model: "auto",
		Body: chatBody("prove"), Allow: func(id uint) bool { return id == 10 }})
	assert.ErrorIs(t, err, proxy.ErrRouteNoCandidates)
}

func TestRouterResolver_ExplicitRoutes(t *testing.T) {
	smart := proxy.RouterRef{Kind: proxy.RouterKindSemantic, ID: 5, Slug: "smart"}

	res, _ := semanticFixture(false)
	_, err := res.Resolve(context.Background(), proxy.RouteRequest{Router: smart, Model: "complex", Body: chatBody("x")})
	assert.ErrorIs(t, err, proxy.ErrRouteNoMatch, "explicit routes are off")
	_, err = res.Resolve(context.Background(), proxy.RouteRequest{Router: smart, Model: "gpt-4o", Body: chatBody("x")})
	assert.ErrorIs(t, err, proxy.ErrRouteNoMatch)
	assert.Equal(t, []string{"auto"}, res.Models(smart))

	res, _ = semanticFixture(true)
	d, err := res.Resolve(context.Background(), proxy.RouteRequest{Router: smart, Model: "complex", Body: chatBody("x")})
	require.NoError(t, err)
	assert.Equal(t, "complex", d.Route)
	assert.Equal(t, sr.ReasonExplicit, d.Reason)
	_, err = res.Resolve(context.Background(), proxy.RouteRequest{Router: smart, Model: "nope", Body: chatBody("x")})
	assert.ErrorIs(t, err, proxy.ErrRouteNoMatch)
	assert.Equal(t, []string{"auto", "complex", "chat", "simple"}, res.Models(smart))
}

func TestRouterResolver_SemanticReaches(t *testing.T) {
	res, _ := semanticFixture(false)
	smart := proxy.RouterRef{Kind: proxy.RouterKindSemantic, ID: 5, Slug: "smart"}
	assert.True(t, res.Reaches(smart, 20), "a direct target")
	assert.True(t, res.Reaches(smart, 10), "a vendor of the pool gpt-4o matches")
	assert.True(t, res.Reaches(smart, 12), "a vendor of the pool claude-best matches")
	assert.False(t, res.Reaches(smart, 13), "an inactive vendor")
	assert.False(t, res.Reaches(smart, 99))
	assert.False(t, res.Reaches(proxy.RouterRef{Kind: proxy.RouterKindSemantic, ID: 6, Slug: "smart"}, 20),
		"a different router id under the same slug")
}

func TestRouterResolver_HandOffToMissingModelRouter(t *testing.T) {
	res, fake := semanticFixture(false)
	fake.cfg.Routes[2].Target.ModelRouterID = 99
	res.semantic.routers["smart"].Config = fake.cfg
	_, err := res.Resolve(context.Background(), proxy.RouteRequest{
		Router: proxy.RouterRef{Kind: proxy.RouterKindSemantic, ID: 5, Slug: "smart"}, Model: "auto", Body: chatBody("x")})
	assert.ErrorIs(t, err, proxy.ErrRouteUnavailable)
}

// Semantic Routers and their App grants arrive in the snapshot and land in
// the edge database; the next snapshot replaces them.
func TestEdgeSync_SemanticRoutersAndGrants(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	snapshot := createFullSnapshot("")
	snapshot.SemanticRouters = []*pb.SemanticRouterConfig{{
		Id: 5, Name: "Smart", Slug: "smart", IsActive: true, ConfigJson: `{"slug":"smart"}`,
		CreatedAt: timestamppb.Now(), UpdatedAt: timestamppb.Now(),
	}}
	snapshot.Apps[0].SemanticRouterIds = []uint32{5}

	sync := NewEdgeSyncService(db, "")
	require.NoError(t, sync.SyncConfiguration(snapshot))

	var app database.App
	require.NoError(t, db.Preload("SemanticRouters").First(&app, snapshot.Apps[0].Id).Error)
	require.Len(t, app.SemanticRouters, 1)
	assert.Equal(t, "smart", app.SemanticRouters[0].Slug)
	assert.Equal(t, `{"slug":"smart"}`, app.SemanticRouters[0].ConfigJSON)

	svc := NewSemanticRouterService(db)
	require.NoError(t, svc.LoadRouters(""))
	if !sr.Available() {
		assert.Zero(t, svc.GetRouterCount(), "without the Enterprise engine no router is served")
	}

	snapshot.SemanticRouters = nil
	snapshot.Apps[0].SemanticRouterIds = nil
	require.NoError(t, sync.SyncConfiguration(snapshot))
	var n int64
	require.NoError(t, db.Model(&database.AppSemanticRouter{}).Count(&n).Error)
	assert.Zero(t, n)
	require.NoError(t, db.Model(&database.SemanticRouter{}).Count(&n).Error)
	assert.Zero(t, n)
}

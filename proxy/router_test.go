package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/gosimple/slug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeResolver is a Model Router with fixed routes: requested model -> (LLM,
// model, pool). It honours RouteRequest.Allow like the real one must.
type fakeResolver struct {
	ref    RouterRef
	routes map[string]fakeRoute
	calls  atomic.Int32
}

type fakeRoute struct {
	llmID uint
	model string
	pool  string
}

func (f *fakeResolver) Lookup(s string) (RouterRef, bool) {
	if s == f.ref.Slug {
		return f.ref, true
	}
	return RouterRef{}, false
}

func (f *fakeResolver) Resolve(_ context.Context, req RouteRequest) (*RouteDecision, error) {
	f.calls.Add(1)
	rt, ok := f.routes[req.Model]
	if !ok {
		return nil, ErrRouteNoMatch
	}
	if req.Allow != nil && !req.Allow(rt.llmID) {
		return nil, ErrRouteNoCandidates
	}
	return &RouteDecision{LLMID: rt.llmID, Model: rt.model, Pool: rt.pool, Reason: "model_pattern"}, nil
}

func (f *fakeResolver) Reaches(ref RouterRef, llmID uint) bool {
	if ref != f.ref {
		return false
	}
	for _, rt := range f.routes {
		if rt.llmID == llmID {
			return true
		}
	}
	return false
}

func (f *fakeResolver) Models(ref RouterRef) []string {
	out := []string{}
	for m := range f.routes {
		out = append(out, m)
	}
	return out
}

// routerHarness is a live proxy with three OpenAI-shaped LLMs (fast, big,
// spare), a Model Router "smart" that reaches fast and big, and an app that
// holds the router and no LLM directly unless a test grants one.
type routerHarness struct {
	t                *testing.T
	db               *gorm.DB
	proxy            *Proxy
	port             int
	apiKey           string
	app              *models.App
	service          *services.Service
	fast, big, spare *models.LLM
	fastV, bigV      *fakeVendor
	spareV           *fakeVendor
	router           *models.ModelRouter
	resolver         *fakeResolver
}

type routerHarnessOpts struct {
	grantRouter bool
	directLLMs  func(h *routerHarness) []uint
	noResolver  bool
	mutate      func(h *routerHarness)
}

func newRouterHarness(t *testing.T, o routerHarnessOpts) *routerHarness {
	t.Helper()
	db, cancel := setupTest(t)
	t.Cleanup(func() { tearDownTest(db, cancel) })

	h := &routerHarness{t: t, db: db}
	h.fastV = newFakeVendor(t, serveOpenAIText("fast answer"))
	h.bigV = newFakeVendor(t, serveOpenAIText("big answer"))
	h.spareV = newFakeVendor(t, serveOpenAIText("spare answer"))

	h.service = services.NewService(db)
	budgetSvc := budget.NewService(db, services.NewTestNotificationService(db))
	user, err := h.service.CreateUser(services.UserDTO{Email: "router@example.com", Name: "router", Password: "password123"})
	require.NoError(t, err)

	mk := func(name string, v *fakeVendor) *models.LLM {
		l := &models.LLM{Name: name, Vendor: models.OPENAI, Active: true, APIEndpoint: v.server.URL, APIKey: name + "-key", DefaultModel: "gpt-4o"}
		require.NoError(t, db.Create(l).Error)
		return l
	}
	h.fast = mk("Fast", h.fastV)
	h.big = mk("Big", h.bigV)
	h.spare = mk("Spare", h.spareV)

	h.router = &models.ModelRouter{Name: "Smart", Slug: "smart", Active: true}
	require.NoError(t, db.Create(h.router).Error)
	h.resolver = &fakeResolver{
		ref: RouterRef{Kind: RouterKindModel, ID: h.router.ID, Slug: "smart"},
		routes: map[string]fakeRoute{
			"fast": {llmID: h.fast.ID, model: "gpt-4o-mini", pool: "fast-pool"},
			"big":  {llmID: h.big.ID, model: "gpt-4o", pool: "big-pool"},
		},
	}
	if o.mutate != nil {
		o.mutate(h)
	}

	var direct []uint
	if o.directLLMs != nil {
		direct = o.directLLMs(h)
	}
	app, err := h.service.CreateApp("Router App", "", user.ID, nil, direct, nil, nil, nil, nil)
	require.NoError(t, err)
	if o.grantRouter {
		require.NoError(t, db.Model(app).Association("ModelRouters").Append(h.router))
	}
	require.NoError(t, h.service.ActivateAppCredential(app.ID))
	app, err = h.service.GetAppByID(app.ID)
	require.NoError(t, err)
	h.app = app
	h.apiKey = app.Credential.Secret

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	h.port = ln.Addr().(*net.TCPAddr).Port
	h.proxy = NewProxy(h.service, &Config{Port: h.port}, budgetSvc)
	if !o.noResolver {
		h.proxy.SetRouteResolver(h.resolver)
	}
	require.NoError(t, h.proxy.loadResources())
	srv := &http.Server{Handler: h.proxy.createHandler()}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		h.proxy.waitForAnalyzers()
	})
	return h
}

func (h *routerHarness) do(method, path, body string, headers ...string) (*http.Response, []byte) {
	h.t.Helper()
	req, err := http.NewRequest(method, "http://127.0.0.1:"+strconv.Itoa(h.port)+path, bytes.NewBufferString(body))
	require.NoError(h.t, err)
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		if headers[i+1] == "" {
			req.Header.Del(headers[i])
			continue
		}
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(h.t, err)
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	return resp, buf.Bytes()
}

func chatBody(model string, stream bool) string {
	b, _ := json.Marshal(map[string]interface{}{
		"model":    model,
		"stream":   stream,
		"messages": []map[string]string{{"role": "user", "content": "hello"}},
	})
	return string(b)
}

func (h *routerHarness) proxyLogs() []models.ProxyLog {
	var logs []models.ProxyLog
	require.NoError(h.t, h.db.Where("app_id = ?", h.app.ID).Order("id asc").Find(&logs).Error)
	return logs
}

func TestRouter_GrantedAppIsServedThroughTheUnifiedIngress(t *testing.T) {
	h := newRouterHarness(t, routerHarnessOpts{grantRouter: true})

	resp, body := h.do(http.MethodPost, "/v1/chat/completions", chatBody("smart/big", false))
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

	var out ChatCompletionResponse
	require.NoError(t, json.Unmarshal(body, &out))
	assert.Equal(t, "big answer", out.Choices[0].Message.Content)
	assert.Equal(t, "gpt-4o", out.Model)

	assert.Equal(t, "smart", resp.Header.Get(hdrRouter))
	assert.Equal(t, "big-pool", resp.Header.Get(hdrRoute))
	assert.Equal(t, "model_pattern", resp.Header.Get(hdrRouteReason))
	assert.Equal(t, slug.Make(h.big.Name), resp.Header.Get(hdrServedLLM))

	calls := h.bigV.calls()
	require.Len(t, calls, 1)
	assert.Empty(t, h.fastV.calls())
	var sent map[string]interface{}
	require.NoError(t, json.Unmarshal(calls[0].Body, &sent))
	assert.Equal(t, "gpt-4o", sent["model"], "the vendor is asked for the router's model")
	for k := range calls[0].Headers {
		assert.False(t, strings.HasPrefix(http.CanonicalHeaderKey(k), "X-Tyk-Router"), "router marker %s leaked to the vendor", k)
		assert.NotEqual(t, http.CanonicalHeaderKey(hdrFailoverToken), http.CanonicalHeaderKey(k), "loopback token leaked to the vendor")
	}

	waitForProxyLog(t, h.db, h.app.ID, http.StatusOK)
	logs := h.proxyLogs()
	require.Len(t, logs, 1)
	assert.Equal(t, h.big.ID, logs[0].LLMID)
	assert.Equal(t, string(RouterKindModel), logs[0].RouterKind)
	assert.Equal(t, "smart", logs[0].RouterSlug)
	assert.Equal(t, "big-pool", logs[0].RouterPool)
	assert.Equal(t, "model_pattern", logs[0].RouteReason)
	assert.Equal(t, "big", logs[0].RouteSourceModel, "the model the caller asked the router for")
	assert.Equal(t, "gpt-4o", logs[0].RouteTargetModel, "the model the chosen LLM was asked for")
}

func TestRouter_Streaming(t *testing.T) {
	h := newRouterHarness(t, routerHarnessOpts{grantRouter: true})

	resp, body := h.do(http.MethodPost, "/v1/chat/completions", chatBody("smart/fast", true))
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	text, chunks := streamedText(t, body)
	assert.Equal(t, "fast answer", text)
	require.NotEmpty(t, chunks)
	assert.Equal(t, "gpt-4o-mini", chunks[0].Model)
	assert.Equal(t, "fast-pool", resp.Header.Get(hdrRoute))
	assert.Len(t, h.fastV.calls(), 1)
}

// A router grant reaches the router's LLMs only through the router: naming
// one of them directly is refused, on every entry point.
func TestRouter_GrantDoesNotGrantItsLLMsDirectly(t *testing.T) {
	h := newRouterHarness(t, routerHarnessOpts{grantRouter: true})

	resp, body := h.do(http.MethodPost, "/v1/chat/completions", chatBody("big/gpt-4o", false))
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "unified ingress: %s", body)
	resp, body = h.do(http.MethodPost, "/ai/big/v1/chat/completions", chatBody("gpt-4o", false))
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "/ai/ bridge: %s", body)
	resp, body = h.do(http.MethodPost, "/llm/call/big/v1/chat/completions", chatBody("gpt-4o", false))
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "/llm/ inner path: %s", body)
	assert.Empty(t, h.bigV.calls(), "nothing may reach Big except through the router")

	resp, body = h.do(http.MethodPost, "/v1/chat/completions", chatBody("smart/big", false))
	require.Equal(t, http.StatusOK, resp.StatusCode, "through the router: %s", body)
	assert.Len(t, h.bigV.calls(), 1)
}

func TestRouter_DirectAIPath(t *testing.T) {
	h := newRouterHarness(t, routerHarnessOpts{grantRouter: true})
	resp, body := h.do(http.MethodPost, "/ai/smart/v1/chat/completions", chatBody("fast", false))
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Len(t, h.fastV.calls(), 1)
}

func TestRouter_UnknownModelIsABadRequest(t *testing.T) {
	h := newRouterHarness(t, routerHarnessOpts{grantRouter: true})
	resp, body := h.do(http.MethodPost, "/v1/chat/completions", chatBody("smart/nope", false))
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", body)
	assert.Empty(t, h.fastV.calls())
	assert.Empty(t, h.bigV.calls())
}

func TestRouter_AnonymousCallerCostsNothing(t *testing.T) {
	h := newRouterHarness(t, routerHarnessOpts{grantRouter: true})
	resp, _ := h.do(http.MethodPost, "/v1/chat/completions", chatBody("smart/big", false), "Authorization", "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp, _ = h.do(http.MethodPost, "/v1/chat/completions", chatBody("smart/big", false), "Authorization", "Bearer not-a-key")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Zero(t, h.resolver.calls.Load(), "the router must not run before authentication")
}

func TestRouter_UngrantedAppUsesOnlyItsOwnLLMs(t *testing.T) {
	// Deprecated compatibility: an App that holds no grant for a Model Router
	// but holds LLMs behind it is served from those LLMs only.
	h := newRouterHarness(t, routerHarnessOpts{
		directLLMs: func(h *routerHarness) []uint { return []uint{h.fast.ID} },
	})

	resp, body := h.do(http.MethodPost, "/v1/chat/completions", chatBody("smart/fast", false))
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

	resp, body = h.do(http.MethodPost, "/v1/chat/completions", chatBody("smart/big", false))
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "body: %s", body)
	assert.Empty(t, h.bigV.calls())
}

func TestRouter_UngrantedAppWithNoLLMsIsRefused(t *testing.T) {
	h := newRouterHarness(t, routerHarnessOpts{})
	resp, _ := h.do(http.MethodPost, "/v1/chat/completions", chatBody("smart/fast", false))
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Empty(t, h.fastV.calls())
}

func TestRouter_WithoutAResolverARouterIsAnUnknownVendor(t *testing.T) {
	h := newRouterHarness(t, routerHarnessOpts{grantRouter: true, noResolver: true})
	resp, _ := h.do(http.MethodPost, "/v1/chat/completions", chatBody("smart/big", false))
	// Unknown and ungranted routes get the same 403, so a caller cannot list
	// the slugs a gateway serves; either way nothing is routed.
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Empty(t, h.bigV.calls())
}

func TestRouter_ModelsListsGrantedRouters(t *testing.T) {
	h := newRouterHarness(t, routerHarnessOpts{grantRouter: true})
	resp, body := h.do(http.MethodGet, "/v1/models", "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	var list unifiedModelList
	require.NoError(t, json.Unmarshal(body, &list))
	ids := []string{}
	for _, m := range list.Data {
		ids = append(ids, m.ID)
	}
	assert.Contains(t, ids, "smart/fast")
	assert.Contains(t, ids, "smart/big")
}

func TestRouter_ModelsOmitsUngrantedRouters(t *testing.T) {
	h := newRouterHarness(t, routerHarnessOpts{directLLMs: func(h *routerHarness) []uint { return []uint{h.fast.ID} }})
	_, body := h.do(http.MethodGet, "/v1/models", "")
	assert.NotContains(t, string(body), "smart/")
}

func TestRouter_ChosenLLMFailsOverToALLMOutsideTheRouter(t *testing.T) {
	// Big fails over to Spare, which the router cannot reach and the app does
	// not hold: access to Spare is inherited from Big, whose own access is
	// inherited from the router.
	h := newRouterHarness(t, routerHarnessOpts{grantRouter: true, mutate: func(h *routerHarness) {
		h.bigV.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { serveStatus(503)(w, r) })
		h.big.Failover = models.LLMFailover{Targets: []models.LLMFailoverTarget{{LLMID: h.spare.ID, Model: "gpt-4o"}}}
		require.NoError(h.t, h.db.Save(h.big).Error)
	}})

	resp, body := h.do(http.MethodPost, "/v1/chat/completions", chatBody("smart/big", false))
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, "true", resp.Header.Get(hdrFailover))
	assert.Equal(t, slug.Make(h.spare.Name), resp.Header.Get(hdrServedLLM))
	assert.Len(t, h.spareV.calls(), 1)
}

// --- unit: the inner hop's grant rule --------------------------------------

func TestRouterGrantsAccess(t *testing.T) {
	fast := &models.LLM{ID: 1, Name: "Fast"}
	big := &models.LLM{ID: 2, Name: "Big"}
	other := &models.LLM{ID: 3, Name: "Other"}
	res := &fakeResolver{
		ref:    RouterRef{Kind: RouterKindModel, ID: 7, Slug: "smart"},
		routes: map[string]fakeRoute{"fast": {llmID: 1}, "big": {llmID: 2}},
	}
	p := &Proxy{failoverToken: newFailoverToken()}
	p.SetRouteResolver(res)

	holder := &models.App{ModelRouters: []models.ModelRouter{{ID: 7}}}
	stranger := &models.App{}

	mk := func(origin, kind, token string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/llm/call/big/v1/chat/completions", nil)
		r.Header.Set(hdrRouterOrigin, origin)
		r.Header.Set(hdrRouterKind, kind)
		r.Header.Set(hdrFailoverToken, token)
		return r
	}
	good := mk("smart", string(RouterKindModel), p.failoverToken)

	assert.True(t, p.routerGrantsAccess(good, holder, big))
	assert.True(t, p.routerGrantsAccess(good, holder, fast))
	assert.False(t, p.routerGrantsAccess(good, holder, other), "an LLM the router cannot reach")
	assert.False(t, p.routerGrantsAccess(good, stranger, big), "an app that does not hold the router")
	assert.False(t, p.routerGrantsAccess(mk("smart", string(RouterKindModel), "guess"), holder, big), "a forged token")
	assert.False(t, p.routerGrantsAccess(mk("smart", string(RouterKindModel), ""), holder, big), "no token")
	assert.False(t, p.routerGrantsAccess(mk("other", string(RouterKindModel), p.failoverToken), holder, big), "an unknown router")
	assert.False(t, p.routerGrantsAccess(mk("smart", "semantic_router", p.failoverToken), holder, big), "a kind mismatch")
	assert.False(t, p.routerGrantsAccess(good, nil, big))

	p.SetRouteResolver(nil)
	assert.False(t, p.routerGrantsAccess(good, holder, big), "no resolver, no routers")
}

func TestFailoverGrantsAccess_OriginReachedThroughARouter(t *testing.T) {
	spare := &models.LLM{ID: 3, Name: "Spare"}
	big := &models.LLM{ID: 2, Name: "Big", Failover: models.LLMFailover{Targets: []models.LLMFailoverTarget{{LLMID: 3, Model: "m"}}}}
	p := &Proxy{
		llms:          map[string]*models.LLM{"big": big, "spare": spare},
		llmsByID:      map[uint]*models.LLM{2: big, 3: spare},
		failoverToken: newFailoverToken(),
	}
	p.SetRouteResolver(&fakeResolver{
		ref:    RouterRef{Kind: RouterKindModel, ID: 7, Slug: "smart"},
		routes: map[string]fakeRoute{"big": {llmID: 2}},
	})
	holder := &models.App{ModelRouters: []models.ModelRouter{{ID: 7}}}

	r := httptest.NewRequest(http.MethodPost, "/llm/call/spare/v1/chat/completions", nil)
	r.Header.Set(hdrFailoverOrigin, "big")
	r.Header.Set(hdrFailoverAttempt, "1")
	r.Header.Set(hdrFailoverToken, p.failoverToken)
	assert.False(t, p.failoverGrantsAccess(r, holder, spare), "without the router marker the origin is not the app's")

	r.Header.Set(hdrRouterOrigin, "smart")
	r.Header.Set(hdrRouterKind, string(RouterKindModel))
	assert.True(t, p.failoverGrantsAccess(r, holder, spare), "origin inherited through the router")
	assert.False(t, p.routerGrantsAccess(r, holder, spare), "the router alone does not reach the rung")
	assert.False(t, p.failoverGrantsAccess(r, &models.App{}, spare))
}

func TestWithBodyModel(t *testing.T) {
	out := withBodyModel([]byte(`{"model":"smart/x","max_tokens":12345678901234567890,"messages":[]}`), "gpt-4o")
	assert.JSONEq(t, `{"model":"gpt-4o","max_tokens":12345678901234567890,"messages":[]}`, string(out))
	assert.Equal(t, "not json", string(withBodyModel([]byte("not json"), "x")))
}

package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/tracing"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/proxy"
)

type endpointSink struct {
	mu     sync.Mutex
	events []*agenttrace.Event
}

func (s *endpointSink) Enqueue(ev *agenttrace.Event) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	return true
}

// With agent tracing on, a custom plugin endpoint request is one generic
// plugin_endpoint event: the plugin, the App it authenticated as, the status
// and the client's trace. The body is not looked at.
func TestPluginEndpoint_AgentTraceCapture(t *testing.T) {
	gin.SetMode(gin.TestMode)
	agenttrace.SetEnabled(true)
	sink := &endpointSink{}
	agenttrace.SetSink(sink)
	tracing.Use(nil, propagation.TraceContext{})
	t.Cleanup(func() { agenttrace.SetSink(nil); agenttrace.SetEnabled(false); tracing.Use(nil, nil) })
	endpointAppCache.set(7101, &pb.App{Id: 7101, Name: "active", IsActive: true})

	pluginAuth := func(*http.Request, proxy.AuthTarget, string, string) (proxy.AuthResult, error) {
		return proxy.AuthResult{Outcome: proxy.AuthAccepted, AppID: 7101}, nil
	}
	pm := &endpointPluginManager{route: &EndpointRouteInfo{PluginID: 33, PluginName: "my-ep", RequireAuth: true}}
	router := gin.New()
	router.Any("/plugins/*path", handlePluginEndpoint(&RouterConfig{PluginManager: pm, PluginAuth: pluginAuth}))

	req := httptest.NewRequest(http.MethodPost, "/plugins/my-ep/things", nil)
	req.Header.Set("Authorization", "Bearer jwt")
	req.Header.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	sink.mu.Lock()
	defer sink.mu.Unlock()
	require.Len(t, sink.events, 1)
	ev := sink.events[0]
	assert.Equal(t, agenttrace.KindPluginEndpoint, ev.Kind)
	assert.Equal(t, agenttrace.Resource{Type: agenttrace.ResourcePlugin, ID: "33", Name: "my-ep"}, ev.Resource)
	assert.Equal(t, uint(7101), ev.Identity.AppID)
	assert.Equal(t, http.StatusOK, ev.HTTPStatus)
	assert.Equal(t, agenttrace.StatusOK, ev.Status)
	assert.Equal(t, "POST", ev.Attributes["http.request.method"])
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", ev.TraceID.String())
	assert.Empty(t, ev.Parts, "the body is not captured")
}

// An App whose hub settings switch agent tracing off leaves no
// plugin_endpoint event (jyx.116).
func TestPluginEndpoint_AppTracingOff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	agenttrace.SetEnabled(true)
	sink := &endpointSink{}
	agenttrace.SetSink(sink)
	t.Cleanup(func() { agenttrace.SetSink(nil); agenttrace.SetEnabled(false) })
	endpointAppCache.set(7103, &pb.App{Id: 7103, Name: "untraced", IsActive: true}, true)

	pluginAuth := func(*http.Request, proxy.AuthTarget, string, string) (proxy.AuthResult, error) {
		return proxy.AuthResult{Outcome: proxy.AuthAccepted, AppID: 7103}, nil
	}
	pm := &endpointPluginManager{route: &EndpointRouteInfo{PluginID: 33, PluginName: "my-ep", RequireAuth: true}}
	router := gin.New()
	router.Any("/plugins/*path", handlePluginEndpoint(&RouterConfig{PluginManager: pm, PluginAuth: pluginAuth}))
	req := httptest.NewRequest(http.MethodGet, "/plugins/my-ep/things", nil)
	req.Header.Set("Authorization", "Bearer jwt")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	sink.mu.Lock()
	defer sink.mu.Unlock()
	assert.Empty(t, sink.events)
	assert.True(t, appTracingOff([]byte(`{"enabled":false}`)))
	assert.False(t, appTracingOff([]byte(`{"content_mode":"full"}`)))
	assert.False(t, appTracingOff(nil))
}

// A refused request is an errored plugin_endpoint event; with tracing off
// nothing is captured.
func TestPluginEndpoint_AgentTraceRefusedAndOff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := &endpointSink{}
	agenttrace.SetSink(sink)
	t.Cleanup(func() { agenttrace.SetSink(nil); agenttrace.SetEnabled(false) })
	pluginAuth := func(*http.Request, proxy.AuthTarget, string, string) (proxy.AuthResult, error) {
		return proxy.AuthResult{Outcome: proxy.AuthRejected}, nil
	}
	serve := func() {
		pm := &endpointPluginManager{route: &EndpointRouteInfo{PluginID: 33, PluginName: "my-ep", RequireAuth: true}}
		router := gin.New()
		router.Any("/plugins/*path", handlePluginEndpoint(&RouterConfig{PluginManager: pm, PluginAuth: pluginAuth}))
		req := httptest.NewRequest(http.MethodGet, "/plugins/my-ep/things", nil)
		req.Header.Set("Authorization", "Bearer bad")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnauthorized, rr.Code)
	}

	agenttrace.SetEnabled(false)
	serve()
	require.Empty(t, sink.events)

	agenttrace.SetEnabled(true)
	serve()
	require.Len(t, sink.events, 1)
	assert.Equal(t, agenttrace.StatusError, sink.events[0].Status)
	assert.Equal(t, http.StatusUnauthorized, sink.events[0].HTTPStatus)
	assert.Equal(t, "401", sink.events[0].ErrorType)
}

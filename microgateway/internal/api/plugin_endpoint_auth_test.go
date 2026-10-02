package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// endpointPluginManager serves one custom endpoint route and records the
// request the plugin would get.
type endpointPluginManager struct {
	route *EndpointRouteInfo
	got   *pb.EndpointRequest
}

func (m *endpointPluginManager) ExecutePluginChain(uint, string, interface{}, interface{}) (interface{}, error) {
	return nil, nil
}
func (m *endpointPluginManager) GetPluginsForLLM(uint, string) (interface{}, error) { return nil, nil }
func (m *endpointPluginManager) IsPluginLoaded(uint) bool                           { return true }
func (m *endpointPluginManager) RefreshLLMPluginMapping(uint) error                 { return nil }
func (m *endpointPluginManager) GetEndpointRoute(method, pluginName, subPath string) *EndpointRouteInfo {
	return m.route
}
func (m *endpointPluginManager) HandleEndpointRequest(_ context.Context, _ uint, req *pb.EndpointRequest) (*pb.EndpointResponse, error) {
	m.got = req
	return &pb.EndpointResponse{StatusCode: http.StatusOK, Body: []byte("ok")}, nil
}
func (m *endpointPluginManager) HandleEndpointRequestStream(context.Context, uint, *pb.EndpointRequest) (interface {
	Recv() (*pb.EndpointResponseChunk, error)
}, error) {
	return nil, nil
}

// A custom endpoint whose plugin has auth plugins attached is authenticated
// by them, with the subject and claims passed on to the endpoint plugin; the
// app they name must be active.
func TestPluginEndpoint_AuthPlugins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	endpointAppCache.set(7001, &pb.App{Id: 7001, Name: "active", IsActive: true})
	endpointAppCache.set(7002, &pb.App{Id: 7002, Name: "inactive", IsActive: false})

	var gotTarget proxy.AuthTarget
	pluginAuth := func(_ *http.Request, target proxy.AuthTarget, credential, _ string) (proxy.AuthResult, error) {
		gotTarget = target
		switch credential {
		case "jwt-active":
			return proxy.AuthResult{Outcome: proxy.AuthAccepted, AppID: 7001, Subject: "alice", Claims: map[string]string{"auth_actor": "bot"}}, nil
		case "jwt-inactive":
			return proxy.AuthResult{Outcome: proxy.AuthAccepted, AppID: 7002}, nil
		}
		return proxy.AuthResult{Outcome: proxy.AuthRejected}, nil
	}

	serve := func(pm *endpointPluginManager, token string) *httptest.ResponseRecorder {
		router := gin.New()
		router.Any("/plugins/*path", handlePluginEndpoint(&RouterConfig{PluginManager: pm, PluginAuth: pluginAuth}))
		req := httptest.NewRequest(http.MethodGet, "/plugins/my-ep/things", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}
	newPM := func() *endpointPluginManager {
		return &endpointPluginManager{route: &EndpointRouteInfo{PluginID: 33, PluginName: "my-ep", RequireAuth: true}}
	}

	t.Run("accepted", func(t *testing.T) {
		pm := newPM()
		rr := serve(pm, "jwt-active")
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		require.NotNil(t, pm.got)
		assert.True(t, pm.got.Authenticated)
		assert.EqualValues(t, 7001, pm.got.App.Id)
		assert.Equal(t, "alice", pm.got.Subject)
		assert.Equal(t, map[string]string{"auth_actor": "bot"}, pm.got.Claims)
		assert.Equal(t, proxy.AuthTarget{Kind: proxy.AuthTargetPlugin, ID: 33, Slug: "my-ep"}, gotTarget)
	})
	t.Run("rejected", func(t *testing.T) {
		pm := newPM()
		rr := serve(pm, "nope")
		require.Equal(t, http.StatusUnauthorized, rr.Code, rr.Body.String())
		assert.Nil(t, pm.got)
	})
	t.Run("inactive app", func(t *testing.T) {
		pm := newPM()
		rr := serve(pm, "jwt-inactive")
		require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
		assert.Nil(t, pm.got)
	})
}

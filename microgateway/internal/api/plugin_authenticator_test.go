package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAuthPluginManager answers GetAuthPlugins from a fixed list and CallAuth
// from a per-plugin function.
type fakeAuthPluginManager struct {
	attached map[string]int // endpointType -> attached count
	loaded   map[string][]*plugins.LoadedPlugin
	answers  map[uint]func(*interfaces.AuthRequest) (*interfaces.AuthResponse, error)
	asked    []uint
	gotType  string
	gotReq   *interfaces.AuthRequest
}

func (f *fakeAuthPluginManager) GetAuthPlugins(endpointType string, _ uint) (int, []*plugins.LoadedPlugin, error) {
	f.gotType = endpointType
	return f.attached[endpointType], f.loaded[endpointType], nil
}

func (f *fakeAuthPluginManager) CallAuth(lp *plugins.LoadedPlugin, req *interfaces.AuthRequest, _ *interfaces.PluginContext) (*interfaces.AuthResponse, error) {
	f.asked = append(f.asked, lp.ID)
	f.gotReq = req
	return f.answers[lp.ID](req)
}

func authenticated(appID string) func(*interfaces.AuthRequest) (*interfaces.AuthResponse, error) {
	return func(*interfaces.AuthRequest) (*interfaces.AuthResponse, error) {
		return &interfaces.AuthResponse{Authenticated: true, AppID: appID, UserID: "alice", Claims: map[string]string{"auth_actor": "bot"}}, nil
	}
}

func rejected(*interfaces.AuthRequest) (*interfaces.AuthResponse, error) {
	return &interfaces.AuthResponse{Authenticated: false, ErrorMessage: "bad token"}, nil
}

func failing(*interfaces.AuthRequest) (*interfaces.AuthResponse, error) {
	return nil, errors.New("plugin crashed")
}

func authRequest() *http.Request {
	return httptest.NewRequest(http.MethodPost, "/datasource/docs", nil)
}

var docsTarget = proxy.AuthTarget{Kind: proxy.AuthTargetDatasource, ID: 3, Slug: "docs"}

func TestPluginAuthenticator_NoPluginsIsNotHandled(t *testing.T) {
	a := newPluginAuthenticator(nil, &fakeAuthPluginManager{}, nil)
	res, err := a.authenticate(authRequest(), docsTarget, "tok", proxy.CredTypeBearer)
	require.NoError(t, err)
	assert.Equal(t, proxy.AuthNotHandled, res.Outcome)
}

// An authenticated answer without a usable app id is a rejection. The hook
// used to turn it into app 1.
func TestPluginAuthenticator_InvalidAppIDIsRejected(t *testing.T) {
	for _, appID := range []string{"", "abc", "0", "-3"} {
		t.Run("app id "+appID, func(t *testing.T) {
			pm := &fakeAuthPluginManager{
				attached: map[string]int{"datasource": 1},
				loaded:   map[string][]*plugins.LoadedPlugin{"datasource": {{ID: 1, Name: "idp"}}},
				answers:  map[uint]func(*interfaces.AuthRequest) (*interfaces.AuthResponse, error){1: authenticated(appID)},
			}
			res, err := newPluginAuthenticator(nil, pm, nil).authenticate(authRequest(), docsTarget, "tok", proxy.CredTypeBearer)
			require.NoError(t, err)
			assert.Equal(t, proxy.AuthRejected, res.Outcome)
			assert.Zero(t, res.AppID)
		})
	}
}

// Plugins are asked in order; a refusal or an error moves on to the next,
// and the first that authenticates wins.
func TestPluginAuthenticator_FirstAcceptWins(t *testing.T) {
	pm := &fakeAuthPluginManager{
		attached: map[string]int{"datasource": 3},
		loaded:   map[string][]*plugins.LoadedPlugin{"datasource": {{ID: 1, Name: "a"}, {ID: 2, Name: "b"}, {ID: 3, Name: "c"}}},
		answers: map[uint]func(*interfaces.AuthRequest) (*interfaces.AuthResponse, error){
			1: rejected,
			2: failing,
			3: authenticated("7"),
		},
	}
	res, err := newPluginAuthenticator(nil, pm, nil).authenticate(authRequest(), docsTarget, "tok", proxy.CredTypeAPIKey)
	require.NoError(t, err)
	assert.Equal(t, proxy.AuthResult{
		Outcome: proxy.AuthAccepted, AppID: 7, Subject: "alice",
		Claims: map[string]string{"auth_actor": "bot"}, PluginID: 3, PluginName: "c",
	}, res)
	assert.Equal(t, []uint{1, 2, 3}, pm.asked)
	assert.Equal(t, "tok", pm.gotReq.Credential)
	assert.Equal(t, proxy.CredTypeAPIKey, pm.gotReq.AuthType)
	assert.Equal(t, "docs", pm.gotReq.Request.Context.Metadata["endpoint_slug"])
}

func TestPluginAuthenticator_AllRefuse(t *testing.T) {
	pm := &fakeAuthPluginManager{
		attached: map[string]int{"datasource": 2},
		loaded:   map[string][]*plugins.LoadedPlugin{"datasource": {{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}},
		answers:  map[uint]func(*interfaces.AuthRequest) (*interfaces.AuthResponse, error){1: failing, 2: rejected},
	}
	res, err := newPluginAuthenticator(nil, pm, nil).authenticate(authRequest(), docsTarget, "tok", proxy.CredTypeBearer)
	require.NoError(t, err)
	assert.Equal(t, proxy.AuthRejected, res.Outcome)
	assert.Equal(t, "bad token", res.Reason)
}

// Attached plugins that cannot run fail closed: an error, which the gateway
// answers with 503, never a fall back to app keys.
func TestPluginAuthenticator_UnavailableIsAnError(t *testing.T) {
	t.Run("none loaded", func(t *testing.T) {
		pm := &fakeAuthPluginManager{attached: map[string]int{"datasource": 1}}
		_, err := newPluginAuthenticator(nil, pm, nil).authenticate(authRequest(), docsTarget, "tok", proxy.CredTypeBearer)
		require.Error(t, err)
	})
	t.Run("none answered", func(t *testing.T) {
		pm := &fakeAuthPluginManager{
			attached: map[string]int{"datasource": 1},
			loaded:   map[string][]*plugins.LoadedPlugin{"datasource": {{ID: 1, Name: "a"}}},
			answers:  map[uint]func(*interfaces.AuthRequest) (*interfaces.AuthResponse, error){1: failing},
		}
		_, err := newPluginAuthenticator(nil, pm, nil).authenticate(authRequest(), docsTarget, "tok", proxy.CredTypeBearer)
		require.Error(t, err)
	})
}

// An LLM target asks the LLM's attached plugins.
func TestPluginAuthenticator_LLMTarget(t *testing.T) {
	pm := &fakeAuthPluginManager{}
	_, err := newPluginAuthenticator(nil, pm, nil).authenticate(authRequest(), proxy.AuthTarget{Kind: proxy.AuthTargetLLM, ID: 5, Slug: "gpt"}, "tok", proxy.CredTypeBearer)
	require.NoError(t, err)
	assert.Equal(t, plugins.AuthEndpointLLM, pm.gotType)
}

package api

import (
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The loopback markers a request carries on the gateway's own inner hop.
// With the hop and failover tokens, a plugin could dial the loopback and
// authenticate as any App through the auth hand-off (midsommar-clean-55x).
var loopbackMarkers = []string{
	"X-Tyk-Internal-Hop",
	"X-Tyk-Failover-Token", "X-Tyk-Failover-Origin", "X-Tyk-Failover-Attempt",
	"X-Tyk-Router-Origin", "X-Tyk-Router-Kind",
	"X-Tyk-Auth-App", "X-Tyk-Auth-Method", "X-Tyk-Auth-Plugin", "X-Tyk-Auth-Subject", "X-Tyk-Auth-Claims",
}

func innerHopRequest() *http.Request {
	r := authRequest()
	for _, h := range loopbackMarkers {
		r.Header.Set(h, "secret-"+h)
	}
	r.Header.Set("X-Request-Source", "client")
	return r
}

func TestPluginHeaders_LeaveOutLoopbackMarkers(t *testing.T) {
	r := innerHopRequest()
	got := pluginHeaders(r)
	for _, h := range loopbackMarkers {
		assert.NotContains(t, got, h, "%s reached a plugin", h)
	}
	assert.Equal(t, "client", got["X-Request-Source"], "other headers still reach plugins")
	assert.Equal(t, "secret-X-Tyk-Internal-Hop", r.Header.Get("X-Tyk-Internal-Hop"), "the request itself keeps its markers")
}

// An auth plugin asked about an inner-hop request never sees the markers.
func TestPluginAuthenticator_NoLoopbackMarkers(t *testing.T) {
	pm := &fakeAuthPluginManager{
		attached: map[string]int{"datasource": 1},
		loaded:   map[string][]*plugins.LoadedPlugin{"datasource": {{ID: 1, Name: "a"}}},
		answers:  map[uint]func(*interfaces.AuthRequest) (*interfaces.AuthResponse, error){1: authenticated("7")},
	}
	_, err := newPluginAuthenticator(nil, pm, nil).authenticate(innerHopRequest(), docsTarget, "tok", proxy.CredTypeAPIKey)
	require.NoError(t, err)
	require.NotNil(t, pm.gotReq)
	for _, h := range loopbackMarkers {
		assert.NotContains(t, pm.gotReq.Request.Headers, h, "%s reached the auth plugin", h)
	}
	assert.Equal(t, "client", pm.gotReq.Request.Headers["X-Request-Source"])
}

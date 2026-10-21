package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/TykTechnologies/midsommar/v2/pkg/pluginlimits"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// tooLarge is what the plugin manager returns when a plugin on an older SDK
// refuses a request over its 4 MiB gRPC limit.
func tooLarge(plugin, hook string) error {
	refusal := status.Error(codes.ResourceExhausted, "grpc: received message larger than max (5242930 vs. 4194304)")
	return fmt.Errorf("plugin %s execution failed: %w", plugin, pluginlimits.AsTooLarge(refusal, plugin, hook, 5242930))
}

// A pre- or post-auth chain that failed because a plugin refused the size is
// a 413 that does not name the plugin; any other failure stays a 500.
func TestRespondPluginChainError(t *testing.T) {
	rec := httptest.NewRecorder()
	respondPluginChainError(rec, tooLarge("old-cache", "post_auth"))
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "request too large for the gateway's plugins (5242930 bytes)", body["error"])

	rec = httptest.NewRecorder()
	respondPluginChainError(rec, errors.New("plugin x execution failed: connection refused"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// When no auth plugin answered and one refused the size, the authenticator
// says so, so the proxy answers 413 rather than "retry shortly".
func TestPluginAuthenticator_TooLargeIsReported(t *testing.T) {
	pm := &fakeAuthPluginManager{
		attached: map[string]int{"datasource": 2},
		loaded:   map[string][]*plugins.LoadedPlugin{"datasource": {{ID: 1, Name: "old-idp"}, {ID: 2, Name: "down"}}},
		answers: map[uint]func(*interfaces.AuthRequest) (*interfaces.AuthResponse, error){
			1: func(*interfaces.AuthRequest) (*interfaces.AuthResponse, error) {
				return nil, tooLarge("old-idp", "auth")
			},
			2: failing,
		},
	}
	_, err := newPluginAuthenticator(nil, pm, nil).authenticate(authRequest(), docsTarget, "tok", proxy.CredTypeBearer)
	tl, ok := pluginlimits.AsTooLargeError(err)
	require.True(t, ok, "err = %v", err)
	assert.Equal(t, "old-idp", tl.Plugin)

	// A plugin that does answer still decides.
	pm.answers[2] = authenticated("5")
	res, err := newPluginAuthenticator(nil, pm, nil).authenticate(authRequest(), docsTarget, "tok", proxy.CredTypeBearer)
	require.NoError(t, err)
	assert.Equal(t, proxy.AuthAccepted, res.Outcome)
}

// The body read for the plugins stops at the plugin limit: a larger body is
// ErrRequestTooLarge (413 from the hooks, nothing forwarded), not read whole.
func TestReadBodyWithoutConsumingIsCapped(t *testing.T) {
	t.Setenv(pluginlimits.EnvMaxMsgBytes, "1024")

	r := httptest.NewRequest(http.MethodPost, "/llm/rest/x/v1/messages", strings.NewReader(strings.Repeat("a", 1024)))
	body, err := readBodyWithoutConsuming(r)
	require.NoError(t, err)
	assert.Len(t, body, 1024)
	again, _ := io.ReadAll(r.Body)
	assert.Len(t, again, 1024, "the body is put back for the proxy")

	r = httptest.NewRequest(http.MethodPost, "/llm/rest/x/v1/messages", strings.NewReader(strings.Repeat("a", 1025)))
	_, err = readBodyWithoutConsuming(r)
	require.ErrorIs(t, err, pluginlimits.ErrRequestTooLarge)

	rec := httptest.NewRecorder()
	respondPluginChainError(rec, err)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Contains(t, rec.Body.String(), "request too large for the gateway's plugins")
}

// An auth plugin is never sent a body over the limit: the authenticator
// refuses before asking any plugin.
func TestPluginAuthenticator_OversizeBodyAsksNoPlugin(t *testing.T) {
	t.Setenv(pluginlimits.EnvMaxMsgBytes, "64")
	pm := &fakeAuthPluginManager{
		attached: map[string]int{"datasource": 1},
		loaded:   map[string][]*plugins.LoadedPlugin{"datasource": {{ID: 1, Name: "idp"}}},
		answers:  map[uint]func(*interfaces.AuthRequest) (*interfaces.AuthResponse, error){1: authenticated("5")},
	}
	r := httptest.NewRequest(http.MethodPost, "/datasource/docs", strings.NewReader(strings.Repeat("a", 65)))
	_, err := newPluginAuthenticator(nil, pm, nil).authenticate(r, docsTarget, "tok", proxy.CredTypeBearer)
	require.ErrorIs(t, err, pluginlimits.ErrRequestTooLarge)
	assert.Empty(t, pm.asked)
}

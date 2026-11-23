package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingChain is a plugin manager whose plugin list for the LLM cannot be
// produced. It records whether the chain was run anyway.
type failingChain struct {
	err      error
	executed bool
}

func (c *failingChain) GetPluginsForLLM(uint, interfaces.HookType) ([]*plugins.LoadedPlugin, error) {
	return nil, c.err
}

func (c *failingChain) ExecutePluginChain(uint, interfaces.HookType, interface{}, *interfaces.PluginContext) (interface{}, error) {
	c.executed = true
	return nil, c.err
}

const secretLoadDetail = "signature verification failed for oci://registry.internal/fw@sha256:abc at /var/lib/plugins/fw"

func loadFailure() error {
	return fmt.Errorf("failed to load plugin 7: %w", &plugins.PluginLoadError{
		PluginID: 7, PluginName: "llm-firewall-2", Err: errors.New(secretLoadDetail),
	})
}

func hookRequest(path string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"m"}`))
	return r.WithContext(context.WithValue(r.Context(), "request_id", "req-1"))
}

func requirePluginLoadRefusal(t *testing.T, rec *httptest.ResponseRecorder, slug string) {
	t.Helper()
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, strconv.Itoa(int(plugins.PluginLoadRetryInterval/time.Second)), rec.Header().Get("Retry-After"))
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	assert.Equal(t, PluginLoadFailedCode, body.Error.Code)
	assert.Equal(t, "plugin_load_failed", body.Error.Code, "the code is part of the client contract")
	assert.Equal(t, "api_error", body.Error.Type)
	assert.Contains(t, body.Error.Message, `"`+slug+`"`, "the message names the LLM")
	assert.Contains(t, body.Error.Message, "plugin")

	// Which plugin, and why, stays in the edge's log.
	for _, leak := range []string{"llm-firewall-2", "signature", "oci://", "/var/lib", "sha256"} {
		assert.NotContains(t, rec.Body.String(), leak)
	}
}

// A pre_auth plugin list that cannot be produced refuses the request. It was
// read as "no plugins" and the request went on to the vendor.
func TestPreAuthHook_PluginLoadFailureRefusesTheRequest(t *testing.T) {
	for name, err := range map[string]error{
		"plugin not loaded":  loadFailure(),
		"list lookup failed": errors.New("failed to get plugins for LLM: database is locked"),
	} {
		t.Run(name, func(t *testing.T) {
			chain := &failingChain{err: err}
			llms := &hookLLMLookup{gateway: slugGateway{llm: &database.LLM{Slug: "claude", Vendor: "anthropic"}}}
			hook := createPreAuthHook(&services.ServiceContainer{}, chain, llms)

			rec := httptest.NewRecorder()
			stop := hook(rec, hookRequest("/llm/rest/claude/v1/messages"))

			require.True(t, stop, "the request must not continue to the vendor")
			requirePluginLoadRefusal(t, rec, "claude")
			assert.NotContains(t, rec.Body.String(), "database is locked")
			assert.False(t, chain.executed)
		})
	}
}

// The same for post_auth, where the firewall, cache and rate limiter run.
func TestPostAuthHook_PluginLoadFailureRefusesTheRequest(t *testing.T) {
	chain := &failingChain{err: loadFailure()}
	llms := &hookLLMLookup{gateway: slugGateway{llm: &database.LLM{Slug: "claude", Vendor: "anthropic"}}}
	hook := createPostAuthHook(&services.ServiceContainer{}, chain, llms)

	rec := httptest.NewRecorder()
	stop := hook(rec, hookRequest("/llm/stream/claude/v1/messages"), 3)

	require.True(t, stop, "the request must not continue to the vendor")
	requirePluginLoadRefusal(t, rec, "claude")
	assert.False(t, chain.executed)
}

// An AWS SDK calling a Bedrock LLM gets the refusal as an AWS error.
func TestPostAuthHook_PluginLoadFailureOnBedrockRuntimeCall(t *testing.T) {
	chain := &failingChain{err: loadFailure()}
	llms := &hookLLMLookup{gateway: slugGateway{llm: &database.LLM{Slug: "bed", Vendor: "bedrock"}}}
	hook := createPostAuthHook(&services.ServiceContainer{}, chain, llms)

	rec := httptest.NewRecorder()
	stop := hook(rec, hookRequest("/llm/rest/bed/model/anthropic.claude-3-haiku-20240307-v1:0/converse"), 3)

	require.True(t, stop)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "ServiceUnavailableException", rec.Header().Get("X-Amzn-ErrorType"))
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	assert.Contains(t, body["message"], `"bed"`)
	assert.NotContains(t, rec.Body.String(), "llm-firewall-2")
}

// An LLM whose plugins all load, or that has none, is unaffected.
func TestAuthHooks_NoPluginsStillContinues(t *testing.T) {
	llms := &hookLLMLookup{gateway: slugGateway{llm: &database.LLM{Slug: "claude", Vendor: "anthropic"}}}
	none := &failingChain{}

	rec := httptest.NewRecorder()
	assert.False(t, createPreAuthHook(&services.ServiceContainer{}, none, llms)(rec, hookRequest("/llm/rest/claude/v1/messages")))
	assert.False(t, createPostAuthHook(&services.ServiceContainer{}, none, llms)(rec, hookRequest("/llm/rest/claude/v1/messages"), 3))
	assert.Equal(t, http.StatusOK, rec.Code, "nothing was written")
	assert.Empty(t, rec.Body.String())
}

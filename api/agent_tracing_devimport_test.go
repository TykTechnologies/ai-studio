package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appconfig "github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/agenttracing"
)

// importingTracing is a tracing service that implements the seed import.
type importingTracing struct {
	recordingTracing
	doc []byte
}

func (i *importingTracing) ImportDocument(_ context.Context, doc []byte) (string, error) {
	i.doc = doc
	if string(doc) == `"bad"` {
		return "", agenttracing.BadRequest("not a v1 document")
	}
	return testSessionID, nil
}

func withDevMode(t *testing.T, on bool) {
	t.Helper()
	prev := appconfig.Get("")
	conf := *prev
	conf.DevMode = on
	appconfig.Set(&conf)
	t.Cleanup(func() { appconfig.Set(prev) })
}

const devImportPath = "/api/v1/agent-tracing/dev/import"

// The e2e seed import (jyx.35) exists only in DEVMODE: without it the
// route is not registered at all.
func TestAgentTracingDevImport_AbsentWithoutDevMode(t *testing.T) {
	withDevMode(t, false)
	svc := &importingTracing{}
	api := withTracing(t, svc)
	_, ok := api.routePerms["POST "+devImportPath]
	assert.False(t, ok)
	// The path falls through to the console's catch-all; nothing imports.
	w := performRequest(api.router, "POST", devImportPath, map[string]any{"version": 1})
	assert.NotEqual(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Nil(t, svc.doc)
}

// In DEVMODE it needs traces:write, passes the body through, answers 201
// with the session id, and maps a bad document to 400.
func TestAgentTracingDevImport_DevMode(t *testing.T) {
	withDevMode(t, true)
	svc := &importingTracing{}
	api := withTracing(t, svc)
	e, ok := api.routePerms["POST "+devImportPath]
	require.True(t, ok)
	assert.Equal(t, authz.Write("traces"), e.perm)

	w := performRequest(api.router, "POST", devImportPath, map[string]any{"version": 1})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), testSessionID)
	assert.JSONEq(t, `{"version": 1}`, string(svc.doc))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))

	w = performRequest(api.router, "POST", devImportPath, "bad")
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// A service without the importer (Community, or tracing off) answers as
// the other tracing routes do.
func TestAgentTracingDevImport_CommunityAnswers402(t *testing.T) {
	skipWithEnterpriseTracing(t)
	withDevMode(t, true)
	api, _ := setupTestAPI(t)
	w := performRequest(api.router, "POST", devImportPath, map[string]any{"version": 1})
	assert.Equal(t, http.StatusPaymentRequired, w.Code, w.Body.String())
}

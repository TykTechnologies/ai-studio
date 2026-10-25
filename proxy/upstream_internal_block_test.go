package proxy

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/gosimple/slug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
)

// With internal-range blocking on (the default since 2.3, sf5), an LLM
// upstream that resolves to an internal address is refused at dial time,
// for a hostname as much as an IP literal: "localhost" resolves to
// 127.0.0.1. LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS exempts a named host.
func TestUpstreamInternalAddressBlockedByDefault(t *testing.T) {
	netguardtest.BlockInternal(t)
	t.Setenv(netguard.EnvAllowInternal, "")
	t.Setenv(netguard.EnvAllowedHosts, "")
	t.Setenv(netguard.EnvAllowedInternalHosts, "")

	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)
	service := services.NewService(db)
	budgetService := budget.NewService(db, services.NewTestNotificationService(db))

	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"completion":"ok"}`))
	}))
	defer upstream.Close()
	u, err := url.Parse(upstream.URL)
	require.NoError(t, err)

	cred := &models.Credential{Secret: "internal-block-cred", Active: true}
	require.NoError(t, db.Create(cred).Error)
	app := &models.App{Name: "Internal block app", CredentialID: cred.ID}
	require.NoError(t, db.Create(app).Error)

	call := func(name, endpoint string) int {
		t.Helper()
		llm := &models.LLM{Name: name, APIKey: "sk-fake", APIEndpoint: endpoint, Vendor: models.MOCK_VENDOR, Active: true, DefaultModel: "m"}
		require.NoError(t, db.Create(llm).Error)
		require.NoError(t, app.AddLLM(db, llm))
		p := NewProxy(service, &Config{Port: 8080}, budgetService)
		require.NoError(t, p.loadResources())
		req := httptest.NewRequest("POST", "/llm/rest/"+slug.Make(name)+"/v1/chat/completions",
			bytes.NewReader([]byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", cred.Secret)
		req = req.WithContext(context.WithValue(req.Context(), "app", app))
		w := httptest.NewRecorder()
		p.createHandler().ServeHTTP(w, req)
		return w.Code
	}

	assert.NotEqual(t, http.StatusOK, call("Loopback literal", upstream.URL))
	assert.NotEqual(t, http.StatusOK, call("Loopback name", "http://localhost:"+u.Port()))
	assert.Zero(t, hits.Load(), "the internal upstream was reached")

	t.Setenv(netguard.EnvAllowedInternalHosts, "localhost")
	assert.Equal(t, http.StatusOK, call("Allow-listed name", "http://localhost:"+u.Port()))
	assert.Equal(t, int32(1), hits.Load())
	t.Setenv(netguard.EnvAllowedInternalHosts, "")
	// The env exemption is static in a running gateway; drop the connection
	// it let the shared pool keep.
	upstreamGuardedTransport.(interface{ CloseIdleConnections() }).CloseIdleConnections()

	// The LLM's own "Allow internal host" switch (w4e) exempts its host for
	// that LLM only, IP literal or name; another LLM on the same host stays
	// blocked.
	callLLM := func(llm *models.LLM) int {
		t.Helper()
		require.NoError(t, db.Create(llm).Error)
		require.NoError(t, app.AddLLM(db, llm))
		p := NewProxy(service, &Config{Port: 8080}, budgetService)
		require.NoError(t, p.loadResources())
		req := httptest.NewRequest("POST", "/llm/rest/"+slug.Make(llm.Name)+"/v1/chat/completions",
			bytes.NewReader([]byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", cred.Secret)
		req = req.WithContext(context.WithValue(req.Context(), "app", app))
		w := httptest.NewRecorder()
		p.createHandler().ServeHTTP(w, req)
		return w.Code
	}
	assert.Equal(t, http.StatusOK, callLLM(&models.LLM{Name: "Switch literal", APIKey: "sk-fake", APIEndpoint: upstream.URL,
		Vendor: models.MOCK_VENDOR, Active: true, DefaultModel: "m", AllowInternalHost: true}))
	assert.Equal(t, http.StatusOK, callLLM(&models.LLM{Name: "Switch name", APIKey: "sk-fake", APIEndpoint: "http://localhost:" + u.Port(),
		Vendor: models.MOCK_VENDOR, Active: true, DefaultModel: "m", AllowInternalHost: true}))
	assert.NotEqual(t, http.StatusOK, call("No switch, same host", "http://localhost:"+u.Port()))
	assert.Equal(t, int32(3), hits.Load())
}

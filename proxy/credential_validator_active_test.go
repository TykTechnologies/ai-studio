package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// The app's live switch (apps:publish) must be enforced by the embedded
// gateway exactly as the microgateway enforces it. Before this test the
// credential validator only looked at the credential's own Active flag, so an
// app "deactivated" through the publish routes kept serving traffic on Studio.
func TestInactiveAppIsRefusedByEmbeddedGateway(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)
	cred, err := f.service.GetCredentialByID(f.appA.CredentialID)
	require.NoError(t, err)

	// Active app: the app-secret bearer path reaches the tool.
	rr := httptest.NewRecorder()
	f.handler.ServeHTTP(rr, restToolRequest(t, f.toolA.Slug, cred.Secret))
	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	_, err = f.service.SetAppActive(f.appA.ID, false, f.userA.ID)
	require.NoError(t, err)

	// Bearer (app secret) path.
	rr = httptest.NewRecorder()
	f.handler.ServeHTTP(rr, restToolRequest(t, f.toolA.Slug, cred.Secret))
	require.Equal(t, http.StatusForbidden, rr.Code, "body: %s", rr.Body.String())
	require.Contains(t, rr.Body.String(), "app is inactive")

	// API key path (CheckAPICredential).
	req := restToolRequest(t, f.toolA.Slug, "")
	req.Header.Set("Authorization", cred.Secret)
	rr = httptest.NewRecorder()
	f.handler.ServeHTTP(rr, req)
	require.NotEqual(t, http.StatusOK, rr.Code, "inactive app must not be served via API key; body: %s", rr.Body.String())

	// Switching it back on restores service.
	_, err = f.service.SetAppActive(f.appA.ID, true, f.userA.ID)
	require.NoError(t, err)
	rr = httptest.NewRecorder()
	f.handler.ServeHTTP(rr, restToolRequest(t, f.toolA.Slug, cred.Secret))
	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
}

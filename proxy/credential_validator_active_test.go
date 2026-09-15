package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/services"
)

// setAppActiveRetrying flips the app's live switch, retrying the SQLite
// "database table is locked" error the shared-cache test database raises
// while the analytics goroutine for the previous request is still writing.
// The same contention is handled the same way in unregisterTestTool.
func setAppActiveRetrying(t *testing.T, service *services.Service, appID uint, active bool, userID uint) {
	t.Helper()
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		_, err = service.SetAppActive(appID, active, userID)
		if err == nil {
			return
		}
		if !strings.Contains(err.Error(), "locked") {
			break
		}
		t.Logf("Database locked during SetAppActive, retrying... (attempt %d/5)", attempt+1)
		time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
	}
	require.NoError(t, err)
}

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

	setAppActiveRetrying(t, f.service, f.appA.ID, false, f.userA.ID)

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
	setAppActiveRetrying(t, f.service, f.appA.ID, true, f.userA.ID)
	rr = httptest.NewRecorder()
	f.handler.ServeHTTP(rr, restToolRequest(t, f.toolA.Slug, cred.Secret))
	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
}

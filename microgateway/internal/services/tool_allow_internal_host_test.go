package services

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// w4e (51q): an edge refuses a tool whose API is on an internal address
// unless the hub's "Allow internal host" switch is on for that tool.
func TestEdgeCallToolOperation_InternalHostNeedsTheSwitch(t *testing.T) {
	netguardtest.BlockInternal(t)
	db := setupToolsSyncTestDB(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)
	spec := fmt.Sprintf(`{"openapi":"3.0.0","info":{"title":"t","version":"1"},"servers":[{"url":"%s"}],`+
		`"paths":{"/x":{"post":{"operationId":"sendData","responses":{"200":{"description":"OK"}}}}}}`, upstream.URL)
	tool := &database.Tool{Name: "internal", ToolType: "REST", OASSpec: base64.StdEncoding.EncodeToString([]byte(spec)), Active: true}
	require.NoError(t, db.Create(tool).Error)

	repo := database.NewRepository(db)
	adapter := NewGatewayServiceAdapter(
		NewDatabaseGatewayService(db, repo),
		NewManagementService(db, repo, &noopCryptoService{}),
		nil, &noopCryptoService{},
		NewFilterService(db, repo), NewPluginService(db, repo),
		nil, db,
	)
	_, err := adapter.CallToolOperation(tool.ID, "sendData", nil, map[string]interface{}{}, nil)
	assert.ErrorContains(t, err, "internal address")

	require.NoError(t, db.Model(tool).Update("allow_internal_host", true).Error)
	_, err = adapter.CallToolOperation(tool.ID, "sendData", nil, map[string]interface{}{}, nil)
	assert.NoError(t, err)
}

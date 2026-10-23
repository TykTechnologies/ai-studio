package services

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// w4e (51q): a tool whose API is on an internal address is refused unless
// an administrator switched "Allow internal host" on for that tool.
func TestCallToolOperation_InternalHostNeedsTheSwitch(t *testing.T) {
	netguardtest.BlockInternal(t)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)
	spec := fmt.Sprintf(`{"openapi":"3.0.0","info":{"title":"t","version":"1"},"servers":[{"url":"%s"}],`+
		`"paths":{"/x":{"post":{"operationId":"sendData","responses":{"200":{"description":"OK"}}}}}}`, upstream.URL)

	svc := NewService(db)
	tool, err := svc.CreateTool("Internal", "d", models.ToolTypeREST, base64.StdEncoding.EncodeToString([]byte(spec)), 0, "", "")
	require.NoError(t, err)
	_, err = svc.CallToolOperation(tool.ID, "sendData", nil, map[string]interface{}{}, nil)
	assert.ErrorContains(t, err, "internal address")

	require.NoError(t, db.Model(tool).Update("allow_internal_host", true).Error)
	_, err = svc.CallToolOperation(tool.ID, "sendData", nil, map[string]interface{}{}, nil)
	assert.NoError(t, err)
}

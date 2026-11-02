package api

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// shutdownCountingClient records the Shutdown a reload sends the running
// plugin process.
type shutdownCountingClient struct {
	mockPortalPluginClient
	shutdowns atomic.Int32
}

func (m *shutdownCountingClient) Shutdown(ctx context.Context, in *pb.ShutdownRequest, opts ...grpc.CallOption) (*pb.ShutdownResponse, error) {
	m.shutdowns.Add(1)
	return &pb.ShutdownResponse{}, nil
}

// Saving a loaded Studio plugin's configuration from the edit form (which
// does not send load_immediately) reloads it, so the running plugin gets the
// new configuration (53e.30). Edits that leave the configuration alone do not
// restart it.
func TestPluginAPI_ConfigChangeReloadsLoadedStudioPlugin(t *testing.T) {
	a, db := setupTestAPI(t)
	router := internalHostRouter(a, true)
	mgr := services.NewAIStudioPluginManager(db, nil)
	a.service.AIStudioPluginManager = mgr

	plugin := &models.Plugin{Name: "price-sync", Command: "file:///nonexistent/price-sync",
		HookType: models.HookTypeStudioUI, HookTypes: []string{models.HookTypeStudioUI}, IsActive: true,
		Config: map[string]interface{}{"prices_url": "https://www.llm-prices.com/current-v1.json"}}
	require.NoError(t, db.Create(plugin).Error)
	path := fmt.Sprintf("/plugins/%d", plugin.ID)

	running := &shutdownCountingClient{}
	mgr.InjectTestPlugin(plugin.ID, plugin.Name, running)
	t.Cleanup(func() { mgr.RemoveTestPlugin(plugin.ID) })

	// Name only, and the same configuration: the running plugin stays.
	w := performRequest(router, "PATCH", path, map[string]any{"name": "price-sync-renamed", "load_immediately": false})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = performRequest(router, "PATCH", path, map[string]any{"load_immediately": false,
		"config": map[string]any{"prices_url": "https://www.llm-prices.com/current-v1.json"}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, int32(0), running.shutdowns.Load())
	assert.True(t, mgr.IsPluginLoaded(plugin.ID))

	// A new configuration: the old process is shut down for a fresh load
	// (which here fails, the command does not exist).
	w = performRequest(router, "PATCH", path, map[string]any{"load_immediately": false,
		"config": map[string]any{"prices_url": "https://prices.example.com/v1.json"}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, int32(1), running.shutdowns.Load(), "the running plugin was unloaded to apply the new configuration")
	assert.False(t, mgr.IsPluginLoaded(plugin.ID), "the old instance is gone")
}

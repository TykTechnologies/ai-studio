package plugins

import (
	"os"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/pluginenv"
	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/microgateway_management"
)

// gatedBroker is a management server (the plugins' service broker) whose
// licensing service is set only when the test says so.
type gatedBroker struct {
	mgmtpb.UnimplementedMicrogatewayManagementServiceServer
	configured atomic.Bool
}

func (b *gatedBroker) LicensingConfigured() bool { return b.configured.Load() }

// No plugin process is started before the service broker can answer
// GetLicenseInfo: an Enterprise plugin asks as soon as its session is ready,
// and one that was started first was told "community" and exited. Whatever
// path tries (pre-warm, reconcile, a request) is refused until main has
// wired licensing.
func TestLoadPlugin_RefusedUntilLicensingIsConfigured(t *testing.T) {
	self, err := os.Executable()
	require.NoError(t, err)
	t.Setenv(msgSizeHelperEnv, "gatewayplugin_sdk")
	t.Setenv("PLUGIN_RUNTIME", "gateway")
	t.Setenv(pluginenv.EnvPassthrough, msgSizeHelperEnv+",PLUGIN_RUNTIME")
	t.Setenv("MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS", "1")

	p := PluginData{ID: 1, Name: "ent-plugin", HookType: "post_auth", Command: self, IsActive: true}
	svc := &mockPluginService{
		allActiveGwPlugins: []PluginData{p},
		getPluginByID:      map[uint]PluginData{1: p},
	}
	pm := NewPluginManager(svc)
	broker := &gatedBroker{}
	pm.SetManagementServer(broker)
	t.Cleanup(func() { _ = pm.UnloadPlugin(1) })

	_, err = pm.LoadPlugin(1)
	require.ErrorIs(t, err, ErrLicensingNotConfigured)
	assert.False(t, pm.IsPluginLoaded(1))
	assert.Equal(t, PluginStatusFailed, pm.GetPluginHealth()[1].Status)

	// The pre-warm main runs at start-up is refused the same way.
	require.NoError(t, pm.PrewarmAllPlugins(t.Context()))
	assert.False(t, pm.IsPluginLoaded(1), "the pre-warm started no plugin")

	broker.configured.Store(true)
	_, err = pm.LoadPlugin(1)
	require.NoError(t, err)
	assert.True(t, pm.IsPluginLoaded(1))
}

// A manager without a service broker gives plugins no license API; there is
// nothing to wait for.
func TestCheckLicensingConfigured_NoBroker(t *testing.T) {
	assert.NoError(t, NewPluginManager(&mockPluginService{}).checkLicensingConfigured())
}

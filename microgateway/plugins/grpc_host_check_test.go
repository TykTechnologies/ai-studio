package plugins

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ipOnlyValidator accepts any hostname but refuses private IP literals: a
// stand-in for a check that passed on the name (as the hub's did, from its
// own network) while the name resolves to an internal address here.
type ipOnlyValidator struct{ checked []string }

func (v *ipOnlyValidator) ValidateGRPCHost(host string) error {
	v.checked = append(v.checked, host)
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback()) {
		return fmt.Errorf("internal address %s", host)
	}
	return nil
}

// A grpc:// plugin the hub pushes with an internal IP is refused at load
// time by the gateway's own internal-network rule.
func TestReconcile_HubPushedGRPCPluginToInternalIPRefused(t *testing.T) {
	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "")
	pushed := PluginData{ID: 7, Name: "internal-grpc", HookType: "post_auth", Command: "grpc://10.0.0.5:50051", IsActive: true}
	svc := &mockPluginService{
		allActiveGwPlugins: []PluginData{pushed},
		getPluginByID:      map[uint]PluginData{7: pushed},
	}
	pm := NewPluginManager(svc)

	require.NoError(t, pm.ReconcilePlugins(context.Background()))
	assert.False(t, pm.IsPluginLoaded(7))
	health := pm.GetPluginHealth()[7]
	require.NotNil(t, health)
	assert.Equal(t, PluginStatusFailed, health.Status)
	assert.Contains(t, health.ErrorMessage, "refused")
}

// A manager without a validator still applies the rule.
func TestCreatePluginClient_GRPCInternalIPRefusedByDefault(t *testing.T) {
	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "")
	pm := NewPluginManager(&mockPluginService{})
	for _, command := range []string{"grpc://10.0.0.5:50051", "grpc://127.0.0.1:50051", "grpc://localhost:50051"} {
		client, err := pm.createPluginClient(command)
		assert.Nil(t, client, command)
		require.Error(t, err, command)
		assert.Contains(t, err.Error(), "refused", command)
	}

	// A host named in LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS is let through.
	t.Setenv("LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS", "10.0.0.5")
	client, err := pm.createPluginClient("grpc://10.0.0.5:50051")
	require.NoError(t, err)
	client.Kill()
	_, err = pm.createPluginClient("grpc://10.0.0.6:50051")
	require.Error(t, err)
	t.Setenv("LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS", "")

	// The development bypass lets them through.
	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
	client, err = pm.createPluginClient("grpc://10.0.0.5:50051")
	require.NoError(t, err)
	client.Kill()
}

// A hostname is checked again by the address it resolves to on the gateway:
// a name that passes (the hub saw a public address) but resolves to an
// internal one here is refused.
func TestCreatePluginClient_GRPCHostnameResolvingToInternalRefused(t *testing.T) {
	orig := resolveTCPAddr
	resolveTCPAddr = func(network, address string) (*net.TCPAddr, error) {
		host, _, _ := net.SplitHostPort(address)
		switch host {
		case "internal.example.com":
			return &net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 50051}, nil
		case "public.example.com":
			return &net.TCPAddr{IP: net.ParseIP("93.184.216.34"), Port: 50051}, nil
		}
		return nil, fmt.Errorf("no such host %q", host)
	}
	t.Cleanup(func() { resolveTCPAddr = orig })

	pm := NewPluginManager(&mockPluginService{})
	validator := &ipOnlyValidator{}
	pm.SetGRPCHostValidator(validator)

	client, err := pm.createPluginClient("grpc://internal.example.com:50051")
	assert.Nil(t, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolves to a refused address")
	assert.Equal(t, []string{"internal.example.com", "10.0.0.5"}, validator.checked)

	client, err = pm.createPluginClient("grpc://public.example.com:50051")
	require.NoError(t, err)
	client.Kill()
	assert.True(t, strings.HasPrefix(strings.Join(validator.checked, ","), "internal.example.com,10.0.0.5,public.example.com,93.184.216.34"))
}

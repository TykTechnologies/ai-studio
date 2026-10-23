package ociplugins

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins/ocitest"
)

func TestAuthConfigContextRoundTrip(t *testing.T) {
	assert.Equal(t, "", AuthConfigFrom(context.Background()))
	assert.Equal(t, "cloudsmith", AuthConfigFrom(WithAuthConfig(context.Background(), "cloudsmith")))
}

// authRecordingVerifier records the auth config name each call carries.
type authRecordingVerifier struct {
	stubVerifier
	authConfigs []string
}

func (v *authRecordingVerifier) VerifySignature(ctx context.Context, _ *OCIReference, _ string) error {
	v.authConfigs = append(v.authConfigs, AuthConfigFrom(ctx))
	return nil
}

func TestFetchPlugin_PassesAuthConfigToVerifier(t *testing.T) {
	config := DefaultOCIConfig()
	config.CacheDir = t.TempDir()
	config.GCInterval = 0
	config.RequireSignature = true
	client, err := NewOCIPluginClient(config)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })
	verifier := &authRecordingVerifier{}
	client.SetSecurityService(verifier)

	reg := ocitest.NewRegistry(t)
	reg.PushPlugin("plugins/demo", "1.0.0", "linux/amd64", []byte("binary"))
	ref := &OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Tag: "1.0.0"}
	_, err = client.FetchPlugin(context.Background(), ref,
		&OCIPluginParams{Architecture: "linux/amd64", AuthConfig: "private-registry"})
	require.NoError(t, err)

	assert.Equal(t, []string{"private-registry"}, verifier.authConfigs)
}

// NewRemoteRepository's repositories go through the netguard client for
// every auth mode, so an internal registry is refused while internal
// blocking is on and reachable once ALLOW_INTERNAL_NETWORK_ACCESS is set.
func TestNewRemoteRepository_GuardsInternalRegistries(t *testing.T) {
	reg := ocitest.NewRegistry(t) // listens on 127.0.0.1
	manifest := reg.PushPlugin("plugins/demo", "1.0.0", "linux/amd64", []byte("binary"))
	t.Setenv("TEST_REGISTRY_SECRET", "secret")

	auths := map[string]RegistryAuth{
		"anonymous":   {},
		"username":    {Username: "user", PasswordEnv: "TEST_REGISTRY_SECRET"},
		"token":       {TokenEnv: "TEST_REGISTRY_SECRET"},
		"entitlement": {EntitlementEnv: "TEST_REGISTRY_SECRET"},
	}
	for name, regAuth := range auths {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultOCIConfig()
			cfg.RegistryAuth = map[string]RegistryAuth{}
			authName := ""
			if name != "anonymous" {
				cfg.RegistryAuth[name] = regAuth
				authName = name
			}
			ref := &OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Tag: "1.0.0"}

			// Internal hosts allowed while building, refused when dialling.
			repo, err := NewRemoteRepository(cfg, ref, authName)
			require.NoError(t, err)
			assert.True(t, repo.PlainHTTP, "a 127.0.0.1 registry is reached over plain HTTP")
			t.Setenv(netguard.EnvAllowInternal, "")
			registryHTTPClient.CloseIdleConnections() // the guard runs when a connection is dialled
			_, err = repo.Resolve(context.Background(), "1.0.0")
			require.Error(t, err, "an internal registry must be refused unless internal hosts are allowed")

			t.Setenv(netguard.EnvAllowInternal, "true")
			repo, err = NewRemoteRepository(cfg, ref, authName)
			require.NoError(t, err)
			desc, err := repo.Resolve(context.Background(), "1.0.0")
			require.NoError(t, err)
			assert.Equal(t, manifest.Digest, desc.Digest)
		})
	}
}

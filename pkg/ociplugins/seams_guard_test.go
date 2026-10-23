package ociplugins

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins/ocitest"
)

// An internal registry is refused before any request reaches it, for every
// auth mode: by name when the repository is built, and by the dial guard when
// the name passed but the address dialled is internal (a rebinding answer;
// simulated here by allowing internal hosts only while building the
// repository) (midsommar-clean-c1g.3, c1g.32).
func TestNewRemoteRepository_InternalRegistryGetsNoRequest(t *testing.T) {
	t.Setenv("TEST_REGISTRY_SECRET", "secret")
	auths := map[string]RegistryAuth{
		"anonymous":   {},
		"username":    {Username: "user", PasswordEnv: "TEST_REGISTRY_SECRET"},
		"token":       {TokenEnv: "TEST_REGISTRY_SECRET"},
		"entitlement": {EntitlementEnv: "TEST_REGISTRY_SECRET"},
		// Named on the reference but not configured: anonymous, still guarded.
		"missing": {},
	}
	for name, regAuth := range auths {
		t.Run(name, func(t *testing.T) {
			reg := ocitest.NewRegistry(t)
			reg.PushPlugin("plugins/demo", "1.0.0", "linux/amd64", []byte("binary"))

			cfg := DefaultOCIConfig()
			cfg.RegistryAuth = map[string]RegistryAuth{}
			authName := ""
			switch name {
			case "anonymous":
			case "missing":
				authName = "not-configured"
			default:
				cfg.RegistryAuth[name] = regAuth
				authName = name
			}
			ref := &OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Tag: "1.0.0"}

			t.Setenv(netguard.EnvAllowInternal, "")
			_, err := NewRemoteRepository(cfg, ref, authName)
			require.ErrorIs(t, err, netguard.ErrInternalPluginHost)
			assert.Contains(t, err.Error(), netguard.EnvAllowInternal, "the error names the setting")

			t.Setenv(netguard.EnvAllowInternal, "true")
			repo, err := NewRemoteRepository(cfg, ref, authName)
			require.NoError(t, err)
			t.Setenv(netguard.EnvAllowInternal, "")
			_, err = repo.Resolve(context.Background(), "1.0.0")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "internal address", "the refusal must come from the dial guard")
			for _, method := range []string{"HEAD", "GET"} {
				assert.Zero(t, reg.Requests(method, "manifests"), "%s manifests reached the registry", method)
			}
		})
	}
}

// Building a repository with registry auth configured writes nothing to
// stdout; it used to print the configured auth key names.
func TestNewRemoteRepository_WritesNothingToStdout(t *testing.T) {
	t.Setenv("TEST_REGISTRY_SECRET", "secret")
	cfg := DefaultOCIConfig()
	cfg.RegistryAuth = map[string]RegistryAuth{
		"registry.example.com": {Username: "user", PasswordEnv: "TEST_REGISTRY_SECRET"},
		"secret-key-name":      {TokenEnv: "TEST_REGISTRY_SECRET"},
	}

	r, w, err := os.Pipe()
	require.NoError(t, err)
	stdout := os.Stdout
	os.Stdout = w
	func() {
		defer func() { os.Stdout = stdout }()
		for _, authName := range []string{"", "secret-key-name", "absent"} {
			for _, registry := range []string{"registry.example.com", "other.example.com"} {
				ref := &OCIReference{Registry: registry, Repository: "plugins/demo", Tag: "1.0.0"}
				_, err := NewRemoteRepository(cfg, ref, authName)
				require.NoError(t, err)
			}
		}
	}()
	require.NoError(t, w.Close())
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Empty(t, string(out))
}

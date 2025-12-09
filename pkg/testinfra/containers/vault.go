package containers

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// VaultContainer wraps a testcontainers Vault instance.
type VaultContainer struct {
	testcontainers.Container
	host      string
	port      string
	rootToken string
}

// VaultConfig holds configuration for creating a Vault container.
type VaultConfig struct {
	// RootToken sets the root token for dev mode. Defaults to "test-token".
	RootToken string
	// Version specifies the Vault image tag. Defaults to "latest".
	Version string
}

// DefaultVaultConfig returns default Vault container configuration.
func DefaultVaultConfig() *VaultConfig {
	return &VaultConfig{
		RootToken: "test-token",
		Version:   "latest",
	}
}

// NewVaultContainer creates and starts a new Vault container in dev mode.
func NewVaultContainer(ctx context.Context, cfg *VaultConfig) (*VaultContainer, error) {
	if cfg == nil {
		cfg = DefaultVaultConfig()
	}
	if cfg.RootToken == "" {
		cfg.RootToken = "test-token"
	}
	if cfg.Version == "" {
		cfg.Version = "latest"
	}

	req := testcontainers.ContainerRequest{
		Image:        fmt.Sprintf("hashicorp/vault:%s", cfg.Version),
		ExposedPorts: []string{"8200/tcp"},
		Env: map[string]string{
			"VAULT_DEV_ROOT_TOKEN_ID":  cfg.RootToken,
			"VAULT_DEV_LISTEN_ADDRESS": "0.0.0.0:8200",
		},
		WaitingFor: wait.ForAll(
			wait.ForHTTP("/v1/sys/health").WithPort("8200/tcp").WithStartupTimeout(60*time.Second),
			wait.ForListeningPort("8200/tcp"),
		),
		// Dev mode runs with IPC_LOCK capability requirement relaxed
		CapAdd: []string{"IPC_LOCK"},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start Vault container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "8200")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get mapped port: %w", err)
	}

	return &VaultContainer{
		Container: container,
		host:      host,
		port:      port.Port(),
		rootToken: cfg.RootToken,
	}, nil
}

// Host returns the container's host address.
func (v *VaultContainer) Host() string {
	return v.host
}

// Port returns the mapped port for Vault.
func (v *VaultContainer) Port() string {
	return v.port
}

// Addr returns the full address in "http://host:port" format.
func (v *VaultContainer) Addr() string {
	return fmt.Sprintf("http://%s:%s", v.host, v.port)
}

// Token returns the root token.
func (v *VaultContainer) Token() string {
	return v.rootToken
}

// Close terminates the Vault container.
func (v *VaultContainer) Close(ctx context.Context) error {
	if v.Container == nil {
		return nil
	}
	return v.Container.Terminate(ctx)
}

// EnableKVEngine enables the KV v2 secrets engine at the specified path.
// Note: Vault dev mode has KV v2 enabled at "secret/" by default.
func (v *VaultContainer) EnableKVEngine(ctx context.Context, path string) error {
	// Use sh -c to set env vars before running vault command
	cmd := []string{
		"sh", "-c",
		fmt.Sprintf("VAULT_TOKEN=%s VAULT_ADDR=http://127.0.0.1:8200 vault secrets enable -path=%s -version=2 kv",
			v.rootToken, path),
	}
	exitCode, _, err := v.Container.Exec(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to enable KV engine: %w", err)
	}
	if exitCode != 0 {
		// May already be enabled, which is fine
		return nil
	}
	return nil
}

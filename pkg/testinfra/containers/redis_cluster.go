package containers

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// RedisClusterContainer wraps a Redis cluster container for integration testing.
// It uses the grokzen/redis-cluster image which provides a pre-configured cluster.
type RedisClusterContainer struct {
	testcontainers.Container
	host  string
	ports []string
	addrs []string
}

// RedisClusterConfig holds configuration for creating a Redis cluster container.
type RedisClusterConfig struct {
	// Version specifies the Redis cluster image tag. Defaults to "7.0.10".
	Version string
}

// DefaultRedisClusterConfig returns a default Redis cluster configuration.
func DefaultRedisClusterConfig() *RedisClusterConfig {
	return &RedisClusterConfig{
		Version: "7.0.10",
	}
}

// NewRedisClusterContainer creates and starts a Redis cluster container.
// This uses the grokzen/redis-cluster image which provides a 6-node cluster
// (3 masters + 3 replicas) with ports 7000-7005.
//
// IMPORTANT: Redis Cluster in Docker requires host network mode on Linux for the
// cluster to be accessible from outside the container. On macOS/Windows with Docker
// Desktop, host networking doesn't work the same way, so cluster tests may fail.
//
// For macOS/Windows, consider using standalone Redis tests instead.
func NewRedisClusterContainer(ctx context.Context, cfg *RedisClusterConfig) (*RedisClusterContainer, error) {
	if cfg == nil {
		cfg = DefaultRedisClusterConfig()
	}

	if cfg.Version == "" {
		cfg.Version = "7.0.10"
	}

	// Use grokzen/redis-cluster which provides a pre-configured 6-node cluster
	// Ports 7000-7005 are used for the cluster nodes
	//
	// IMPORTANT: We use fixed port mappings (7000:7000, etc.) so that the cluster's
	// internal addresses match what's accessible from the host. This requires ports
	// 7000-7005 to be available on the host machine.
	req := testcontainers.ContainerRequest{
		Image: fmt.Sprintf("grokzen/redis-cluster:%s", cfg.Version),
		ExposedPorts: []string{
			"7000/tcp",
			"7001/tcp",
			"7002/tcp",
			"7003/tcp",
			"7004/tcp",
			"7005/tcp",
		},
		Env: map[string]string{
			"IP": "0.0.0.0",
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("Cluster state changed: ok").WithStartupTimeout(120*time.Second),
			wait.ForListeningPort("7000/tcp"),
		),
		// Use fixed port mappings so cluster addresses match host addresses
		HostConfigModifier: func(hc *container.HostConfig) {
			hc.PortBindings = nat.PortMap{
				"7000/tcp": []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "7000"}},
				"7001/tcp": []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "7001"}},
				"7002/tcp": []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "7002"}},
				"7003/tcp": []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "7003"}},
				"7004/tcp": []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "7004"}},
				"7005/tcp": []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "7005"}},
			}
		},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start Redis cluster container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	// Get mapped ports for all cluster nodes
	var ports []string
	var addrs []string
	for i := 7000; i <= 7005; i++ {
		port, err := container.MappedPort(ctx, nat.Port(fmt.Sprintf("%d/tcp", i)))
		if err != nil {
			_ = container.Terminate(ctx)
			return nil, fmt.Errorf("failed to get mapped port %d: %w", i, err)
		}
		ports = append(ports, port.Port())
		addrs = append(addrs, fmt.Sprintf("%s:%s", host, port.Port()))
	}

	return &RedisClusterContainer{
		Container: container,
		host:      host,
		ports:     ports,
		addrs:     addrs,
	}, nil
}

// Host returns the container's host address.
func (c *RedisClusterContainer) Host() string {
	return c.host
}

// Ports returns all mapped ports for the Redis cluster nodes.
func (c *RedisClusterContainer) Ports() []string {
	return c.ports
}

// Addrs returns the cluster node addresses.
// Returns all 6 node addresses for the grokzen/redis-cluster setup.
func (c *RedisClusterContainer) Addrs() []string {
	return c.addrs
}

// PrimaryAddr returns the primary cluster address for initial connection.
func (c *RedisClusterContainer) PrimaryAddr() string {
	if len(c.addrs) > 0 {
		return c.addrs[0]
	}
	if len(c.ports) > 0 {
		return fmt.Sprintf("%s:%s", c.host, c.ports[0])
	}
	return ""
}

// Close terminates the Redis cluster container and releases resources.
func (c *RedisClusterContainer) Close(ctx context.Context) error {
	if c.Container == nil {
		return nil
	}
	return c.Container.Terminate(ctx)
}

// FlushAll clears all data from the Redis cluster.
func (c *RedisClusterContainer) FlushAll(ctx context.Context) error {
	// In cluster mode, FLUSHALL needs the -c flag and -p for the port
	// grokzen/redis-cluster uses ports 7000-7005
	cmd := []string{"redis-cli", "-c", "-p", "7000", "FLUSHALL"}

	exitCode, _, err := c.Container.Exec(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to execute FLUSHALL: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("FLUSHALL returned exit code %d", exitCode)
	}
	return nil
}

// Ping verifies the Redis cluster connection is working.
func (c *RedisClusterContainer) Ping(ctx context.Context) error {
	cmd := []string{"redis-cli", "-c", "-p", "7000", "PING"}

	exitCode, _, err := c.Container.Exec(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to execute PING: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("PING returned exit code %d", exitCode)
	}
	return nil
}

// ClusterInfo returns cluster information for debugging.
func (c *RedisClusterContainer) ClusterInfo(ctx context.Context) (string, error) {
	cmd := []string{"redis-cli", "-c", "-p", "7000", "CLUSTER", "INFO"}

	exitCode, output, err := c.Container.Exec(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to execute CLUSTER INFO: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("CLUSTER INFO returned exit code %d", exitCode)
	}

	// Read the output
	buf := make([]byte, 4096)
	n, _ := output.Read(buf)
	return string(buf[:n]), nil
}
